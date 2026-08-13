package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Envelope is the generic immutable persistence boundary for historical
// artifacts. Artifact semantics belong to their producing typed workflow.
type Envelope struct {
	ID        string          `json:"id,omitempty"`
	JobID     int64           `json:"job_id,omitempty"`
	StepID    int64           `json:"step_id,omitempty"`
	Kind      string          `json:"kind"`
	Version   string          `json:"version"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

func (e Envelope) Validate() error {
	if e.Kind == "" || e.Kind != strings.TrimSpace(e.Kind) {
		return errors.New("artifact kind is required as one exact value")
	}
	if e.Version == "" || e.Version != strings.TrimSpace(e.Version) {
		return errors.New("artifact version is required as one exact value")
	}
	if len(e.Payload) == 0 || string(e.Payload) == "null" {
		return errors.New("artifact payload is required")
	}
	if !json.Valid(e.Payload) {
		return fmt.Errorf("artifact payload for %s is not valid JSON", e.Kind)
	}
	return nil
}
