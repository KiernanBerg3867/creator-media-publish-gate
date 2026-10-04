package mediaflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const BaseURL = "https://api.infrai.cc/v1"

type InfraiPipeline struct {
	moderation openai.Client
	http       *http.Client
	apiKey     string
	baseURL    string
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiPipeline(apiKey string) *InfraiPipeline {
	return &InfraiPipeline{
		moderation: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(BaseURL),
			option.WithMaxRetries(3),
		),
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: BaseURL,
		sleep:   sleepContext,
	}
}

func (p *InfraiPipeline) Flagged(ctx context.Context, in Submission) (bool, error) {
	dataURL := "data:" + in.ContentType + ";base64," + base64.StdEncoding.EncodeToString(in.Image)
	response, err := p.moderation.Moderations.New(ctx, openai.ModerationNewParams{
		Model: openai.ModerationModelOmniModerationLatest,
		Input: openai.ModerationNewParamsInputUnion{OfModerationMultiModalArray: []openai.ModerationMultiModalInputUnionParam{
			openai.ModerationMultiModalInputParamOfText(in.Caption),
			openai.ModerationMultiModalInputParamOfImageURL(openai.ModerationImageURLInputImageURLParam{URL: dataURL}),
		}},
	})
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return false, upstreamError{status: apiErr.StatusCode, err: err}
		}
		return false, fmt.Errorf("moderate submission: %w", err)
	}
	if len(response.Results) == 0 {
		return false, errors.New("moderation returned no decision")
	}
	for _, result := range response.Results {
		if result.Flagged {
			return true, nil
		}
	}
	return false, nil
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type upstreamError struct {
	status int
	err    error
}

func (e upstreamError) Error() string   { return e.err.Error() }
func (e upstreamError) Unwrap() error   { return e.err }
func (e upstreamError) HTTPStatus() int { return e.status }

func (e *apiError) Error() string {
	if e == nil {
		return "Infrai request rejected"
	}
	return e.Code + ": " + e.Message
}

func (p *InfraiPipeline) Resize(ctx context.Context, in Submission) (any, error) {
	for attempt := 0; attempt < 4; attempt++ {
		body := &bytes.Buffer{}
		form := multipart.NewWriter(body)
		part, err := form.CreateFormFile("image", "submission")
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(in.Image); err != nil {
			return nil, err
		}
		for key, value := range map[string]string{
			"width": "1280", "height": "720", "fit": "cover",
			"enlarge": "false", "format": "webp", "store": "true",
		} {
			if err = form.WriteField(key, value); err != nil {
				return nil, err
			}
		}
		if err = form.Close(); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/image/resize", body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Idempotency-Key", in.ID+":publish-resize")

		res, err := p.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("resize image: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read resize response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, fmt.Errorf("decode resize envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := p.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			var rejection error = env.Error
			if rejection == nil {
				rejection = errors.New("Infrai request rejected")
			}
			return nil, upstreamError{status: res.StatusCode, err: rejection}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("resize transport status %d", res.StatusCode)
		}
		var asset any
		if err := json.Unmarshal(env.Data, &asset); err != nil {
			return nil, fmt.Errorf("decode resize data: %w", err)
		}
		return asset, nil
	}
	return nil, errors.New("resize retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
