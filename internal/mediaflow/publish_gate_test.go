package mediaflow

import (
	"context"
	"errors"
	"testing"
)

type fixedModerator struct {
	flagged bool
	err     error
}

func (m fixedModerator) Flagged(context.Context, Submission) (bool, error) {
	return m.flagged, m.err
}

type countingTransformer struct {
	calls int
}

func (t *countingTransformer) Resize(context.Context, Submission) (any, error) {
	t.calls++
	return map[string]any{"id": "asset-ready"}, nil
}

func TestGateProcess(t *testing.T) {
	tests := []struct {
		name          string
		flagged       bool
		moderationErr error
		wantState     string
		wantCalls     int
		wantErr       bool
	}{
		{name: "safe submission is transformed", wantState: "ready", wantCalls: 1},
		{name: "flagged submission is quarantined", flagged: true, wantState: "quarantined"},
		{name: "moderation error stops delivery", moderationErr: errors.New("decision unavailable"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := &countingTransformer{}
			gate := NewGate(fixedModerator{flagged: tt.flagged, err: tt.moderationErr}, transformer)
			got, err := gate.Process(context.Background(), Submission{
				ID: "upload-42", Caption: "Morning trail", ContentType: "image/jpeg", Image: []byte("image"),
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got.State != tt.wantState {
				t.Errorf("state = %q, want %q", got.State, tt.wantState)
			}
			if transformer.calls != tt.wantCalls {
				t.Errorf("resize calls = %d, want %d", transformer.calls, tt.wantCalls)
			}
		})
	}
}
