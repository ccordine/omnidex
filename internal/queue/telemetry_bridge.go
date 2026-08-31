package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Struggle and outcome event types persisted from the worker pipeline for metrics.
var telemetryStruggleEventTypes = []string{
	"step_error",
	"step_canceled",
	"run_failed",
	"run_cancelled",
	"coding_worker_rejected",
	"coding_worker_failed",
	"objective_worker_rejected",
	"objective_worker_failed",
}

var telemetryAcceptEventTypes = []string{
	"step_complete",
	"run_completed",
	"coding_stage_passed",
	"coding_worker_completed",
	"objective_worker_completed",
}

type TelemetryStruggleSummary struct {
	StruggleEvents     []TelemetryCountSummary `json:"struggle_events"`
	AcceptEvents       []TelemetryCountSummary `json:"accept_events"`
	RecentStruggleRuns int                     `json:"recent_struggle_runs"`
}

func shouldRecordTelemetryStepEvent(eventType, message string) bool {
	if shouldRecordTelemetrySignalEvent(eventType, message) {
		return true
	}
	return isTelemetryOpsEvent(eventType)
}

func shouldRecordTelemetrySignalEvent(eventType, message string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return false
	}
	_ = message
	for _, candidate := range telemetryStruggleEventTypes {
		if eventType == candidate {
			return true
		}
	}
	for _, candidate := range telemetryAcceptEventTypes {
		if eventType == candidate {
			return true
		}
	}
	return false
}

func isTelemetryOpsEvent(eventType string) bool {
	e := strings.ToLower(strings.TrimSpace(eventType))
	if e == "" {
		return false
	}
	markers := []string{
		"error", "failed", "rejected", "waiting_input", "blocked", "cancel",
		"unavailable", "deferred", "indeterminate",
	}
	for _, marker := range markers {
		if strings.Contains(e, marker) {
			return true
		}
	}
	switch e {
	case "step_complete", "run_completed", "coding_stage_passed",
		"coding_worker_completed", "objective_worker_completed",
		"coding_fragment_repair_guidance_started", "coding_fragment_correction_started":
		return true
	}
	return false
}

func (r *Repository) MarkTelemetryRunRunningForJob(ctx context.Context, jobID int64) error {
	if jobID <= 0 {
		return fmt.Errorf("mark telemetry run running requires a positive job ID")
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE omni_runs
		SET status = 'running', updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
		  AND status = 'pending'
	`, jobID)
	return err
}

func (r *Repository) RecordTelemetryStepEvent(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	eventType, message string,
) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("record telemetry step event requires an event type")
	}
	if !shouldRecordTelemetryStepEvent(eventType, message) {
		return nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := lockStepAttemptAuthorityTx(ctx, tx, authority); err != nil {
		return err
	}
	var runID *string
	err = tx.QueryRow(ctx, `
		SELECT NULLIF(metadata->>'telemetry_run_id', '') FROM jobs WHERE id=$1
	`, authority.JobID).Scan(&runID)
	if err != nil || runID == nil || strings.TrimSpace(*runID) == "" {
		return err
	}
	run := strings.TrimSpace(*runID)
	payload := map[string]any{
		"job_id": authority.JobID, "step_id": authority.StepID,
		"step_attempt": authority.Attempt, "worker_id": authority.WorkerID,
		"message": strings.TrimSpace(message),
	}
	payloadJSON, err := encodeTelemetryJSON("fenced step event payload", payload)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO omni_run_events (run_id,event_type,payload)
		VALUES ($1,$2,$3)
	`, run, strings.TrimSpace(eventType), payloadJSON); err != nil {
		return fmt.Errorf("insert fenced step telemetry event: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) RecordTelemetryJobEventNow(ctx context.Context, jobID int64, eventType string, payload any) error {
	if jobID <= 0 {
		return fmt.Errorf("record telemetry job event requires a positive job ID")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("record telemetry job event requires an event type")
	}
	payloadJSON, err := encodeTelemetryJSON("job event payload", payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, event_type, payload)
		SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid, $2, $3
		FROM jobs
		WHERE id = $1 AND NULLIF(metadata->>'telemetry_run_id', '') IS NOT NULL
	`, jobID, eventType, payloadJSON)
	return err
}

func (r *Repository) TelemetryStruggleSummary(ctx context.Context) (TelemetryStruggleSummary, error) {
	struggle, err := r.telemetryEventCounts(ctx, telemetryStruggleEventTypes, 12)
	if err != nil {
		return TelemetryStruggleSummary{}, err
	}
	accept, err := r.telemetryEventCounts(ctx, telemetryAcceptEventTypes, 8)
	if err != nil {
		return TelemetryStruggleSummary{}, err
	}
	var struggleRuns int
	_ = r.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT run_id)
		FROM omni_run_events
		WHERE created_at >= NOW() - INTERVAL '7 days'
		  AND event_type = ANY($1)
	`, telemetryStruggleEventTypes).Scan(&struggleRuns)
	return TelemetryStruggleSummary{
		StruggleEvents:     struggle,
		AcceptEvents:       accept,
		RecentStruggleRuns: struggleRuns,
	}, nil
}
