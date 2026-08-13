package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

const (
	maxStationGapPromptBytes   = 64 * 1024
	maxStationGapSchemaBytes   = 32 * 1024
	maxStationGapEnvelopeBytes = 128 * 1024
	maxStationGapResponseBytes = 64 * 1024
	maxStationGapErrorBytes    = 8 * 1024
)

type StationGapStatus string

const (
	StationGapResolved StationGapStatus = "resolved"
	StationGapFailed   StationGapStatus = "failed"
)

type StationGapOpenRecord struct {
	Authority       model.StepAttemptAuthority
	Job             assemblyline.PortableJob
	Station         station.ID
	ContextTokens   int
	MaxOutputTokens int
}

type StationGapOpening struct {
	ID                     int64           `json:"id"`
	JobID                  int64           `json:"job_id"`
	Generation             int64           `json:"generation"`
	StepID                 int64           `json:"step_id"`
	StepAttempt            int64           `json:"step_attempt"`
	WorkerID               string          `json:"worker_id"`
	GapID                  string          `json:"gap_id"`
	Station                station.ID      `json:"station"`
	Scope                  string          `json:"scope"`
	PortableSchema         string          `json:"portable_schema"`
	WorkID                 string          `json:"work_id"`
	WorkKind               string          `json:"work_kind"`
	PortablePayload        string          `json:"portable_payload"`
	PortablePayloadSHA256  string          `json:"portable_payload_sha256"`
	PortableEnvelope       string          `json:"portable_envelope"`
	PortableEnvelopeSHA256 string          `json:"portable_envelope_sha256"`
	RendererVersion        string          `json:"renderer_version"`
	Prompt                 string          `json:"prompt"`
	ResponseSchema         json.RawMessage `json:"response_schema"`
	ProjectionEnvelope     string          `json:"projection_envelope"`
	ProjectionSHA256       string          `json:"projection_sha256"`
	ContextTokens          int             `json:"context_tokens"`
	MaxOutputTokens        int             `json:"max_output_tokens"`
	CreatedAt              time.Time       `json:"created_at"`
}

type StationGapTerminalRecord struct {
	Authority model.StepAttemptAuthority
	OpeningID int64
	GapID     string
	Status    StationGapStatus
	Response  string
	Error     string
}

type StationGapOutcome struct {
	ID             int64            `json:"id"`
	OpeningID      int64            `json:"opening_id"`
	JobID          int64            `json:"job_id"`
	Generation     int64            `json:"generation"`
	StepID         int64            `json:"step_id"`
	StepAttempt    int64            `json:"step_attempt"`
	WorkerID       string           `json:"worker_id"`
	GapID          string           `json:"gap_id"`
	Status         StationGapStatus `json:"status"`
	Response       string           `json:"response,omitempty"`
	ResponseSHA256 string           `json:"response_sha256,omitempty"`
	Error          string           `json:"error,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

func stationGapSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func canonicalStationGapSchema(schema map[string]any) ([]byte, error) {
	if schema == nil {
		return exactjson.Canonical(nil)
	}
	return exactjson.Canonical(schema)
}

func validateStationGapToken(subject, value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 {
		return fmt.Errorf("station gap %s must be exact non-empty text within 128 bytes", subject)
	}
	return nil
}
