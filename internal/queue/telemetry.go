package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type TelemetryRunSummary struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	TaskKind    string          `json:"task_kind,omitempty"`
	ProjectType string          `json:"project_type,omitempty"`
	Status      string          `json:"status"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	DurationMS  *int64          `json:"duration_ms,omitempty"`
	LocalOnly   bool            `json:"local_only"`
	Summary     json.RawMessage `json:"summary,omitempty"`
}

type TelemetryEventSummary struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Step      *int            `json:"step,omitempty"`
	EventType string          `json:"event_type"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

type TelemetryDashboardSummary struct {
	LiveRuns       []TelemetryRunSummary    `json:"live_runs"`
	RecentRuns     []TelemetryRunSummary    `json:"recent_runs,omitempty"`
	StatusCounts   map[string]int           `json:"status_counts,omitempty"`
	CommonBlockers []TelemetryCountSummary  `json:"common_blockers,omitempty"`
	Struggle       TelemetryStruggleSummary `json:"struggle,omitempty"`
}

type TelemetryCountSummary struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type TelemetryModelSummary struct {
	Role          string  `json:"role"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	Calls         int     `json:"calls"`
	Successes     int     `json:"successes"`
	Failures      int     `json:"failures"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	EstimatedCost string  `json:"estimated_cost_usd,omitempty"`
}

type TelemetryModelCallRecord struct {
	RunID            string
	Role             string
	Provider         string
	Model            string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	LatencyMS        *int64
	InputTokens      *int
	OutputTokens     *int
	EstimatedCostUSD *string
	Success          *bool
	Metadata         any
}

func (r *Repository) RecordTelemetryModelCall(ctx context.Context, record TelemetryModelCallRecord) error {
	metadata, err := encodeTelemetryJSON("model-call metadata", record.Metadata)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO omni_model_calls (run_id, role, provider, model, started_at, finished_at, latency_ms, input_tokens, output_tokens, estimated_cost_usd, success, metadata)
		VALUES ($1, NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), $5, $6, $7, $8, $9, NULLIF($10,'')::numeric, $11, $12)
	`, strings.TrimSpace(record.RunID), record.Role, record.Provider, record.Model, record.StartedAt, record.FinishedAt, record.LatencyMS, record.InputTokens, record.OutputTokens, valueString(record.EstimatedCostUSD), record.Success, metadata)
	return err
}

// encodeTelemetryJSON defines nil telemetry payloads as one explicit empty JSON
// object. Every non-nil value must encode as exact canonical, non-null JSON.
func encodeTelemetryJSON(label string, value any) ([]byte, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("telemetry JSON label is required")
	}
	if value == nil {
		return []byte(`{}`), nil
	}
	var blob []byte
	rawInput := false
	if raw, ok := value.(json.RawMessage); ok {
		blob = append([]byte(nil), raw...)
		rawInput = true
	} else if raw, ok := value.([]byte); ok {
		blob = append([]byte(nil), raw...)
		rawInput = true
	} else {
		var err error
		blob, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode telemetry %s: %w", label, err)
		}
	}
	if len(blob) == 0 {
		return nil, fmt.Errorf("encode telemetry %s: empty JSON is forbidden", label)
	}
	if !json.Valid(blob) {
		return nil, fmt.Errorf("encode telemetry %s: invalid JSON", label)
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("encode telemetry %s: decode canonical JSON: %w", label, err)
	}
	if decoded == nil {
		return nil, fmt.Errorf("encode telemetry %s: JSON null is forbidden", label)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("encode telemetry %s: canonicalize JSON: %w", label, err)
	}
	if rawInput && !bytes.Equal(blob, canonical) {
		return nil, fmt.Errorf("encode telemetry %s: raw JSON must use canonical encoding", label)
	}
	return canonical, nil
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
