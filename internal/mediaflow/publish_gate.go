package mediaflow

import (
	"context"
	"errors"
)

type Submission struct {
	ID          string
	Caption     string
	ContentType string
	Image       []byte
}

type Result struct {
	SubmissionID string `json:"submission_id"`
	State        string `json:"state"`
	Asset        any    `json:"asset,omitempty"`
}

type Moderator interface {
	Flagged(context.Context, Submission) (bool, error)
}

type Transformer interface {
	Resize(context.Context, Submission) (any, error)
}

type Gate struct {
	moderator   Moderator
	transformer Transformer
}

func NewGate(m Moderator, t Transformer) *Gate {
	return &Gate{moderator: m, transformer: t}
}

func (g *Gate) Process(ctx context.Context, in Submission) (Result, error) {
	if in.ID == "" || in.Caption == "" || len(in.Image) == 0 || in.ContentType == "" {
		return Result{}, errors.New("submission_id, caption, and image are required")
	}

	flagged, err := g.moderator.Flagged(ctx, in)
	if err != nil {
		return Result{}, err
	}
	if flagged {
		return Result{SubmissionID: in.ID, State: "quarantined"}, nil
	}

	asset, err := g.transformer.Resize(ctx, in)
	if err != nil {
		return Result{}, err
	}
	return Result{SubmissionID: in.ID, State: "ready", Asset: asset}, nil
}
