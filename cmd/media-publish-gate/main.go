package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infrai-examples/media-publish-gate/internal/mediaflow"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	pipeline := mediaflow.NewInfraiPipeline(key)
	gate := mediaflow.NewGate(pipeline, pipeline)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /submissions", submissionHandler(gate))
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("media publish gate listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func submissionHandler(gate *mediaflow.Gate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart submission"})
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
			return
		}
		defer file.Close()
		image, err := io.ReadAll(io.LimitReader(file, 10<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read image"})
			return
		}
		contentType := header.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "image content type required"})
			return
		}

		result, err := gate.Process(r.Context(), mediaflow.Submission{
			ID: r.FormValue("submission_id"), Caption: r.FormValue("caption"),
			ContentType: contentType, Image: image,
		})
		if err != nil {
			var upstream interface{ HTTPStatus() int }
			if errors.As(err, &upstream) {
				status := upstream.HTTPStatus()
				if status < 400 || status >= 500 {
					status = http.StatusBadGateway
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
