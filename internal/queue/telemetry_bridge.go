package queue

import (
	"context"
	"strings"
)

// Struggle and outcome event types persisted from the worker pipeline for metrics.
var telemetryStruggleEventTypes = []string{
	"step_error",
	"step_interrupted",
	"verify_auto_replan",
	"verify_replan",
	"verify_hallucination_retry",
	"verify_hallucination_loop",
	"verify_test_fail",
	"tool_call_rejected",
	"plan_waiting_input",
	"analyze_waiting_input",
	"response_waiting_input",
	"tooling_waiting_input",
	"web_search_waiting_input",
	"llm_error",
	"external_agent_failed",
	"progression_gate_failed",
	"progression_gate_rejected_false_done",
	"structured_loop_exhausted",
	"structured_command_rejected",
	"pathfinder_started",
	"artifact_validation_failed",
}

var telemetryAcceptEventTypes = []string{
	"verify_test_pass",
	"run_completed",
	"solution_accepted",
	"structured_evaluator_repair_accepted",
	"completion_check_accepted",
}

var telemetryRecoveryTriggers = map[string]string{
	"verify_auto_replan":        "auto_replan",
	"verify_replan":             "replan",
	"verify_hallucination_loop": "hallucination_loop",
	"pathfinder_started":        "pathfinder",
	"progression_gate_failed":   "progression_gate",
}

type TelemetryStruggleSummary struct {
	StruggleEvents    []TelemetryCountSummary `json:"struggle_events"`
	AcceptEvents      []TelemetryCountSummary `json:"accept_events"`
	RecoveryAttempts  int                     `json:"recovery_attempts"`
	RecoverySuccesses int                     `json:"recovery_successes"`
	RecentStruggleRuns int                    `json:"recent_struggle_runs"`
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
	switch eventType {
	case "verify_ready":
		msg := strings.ToLower(message)
		return strings.Contains(msg, "status=blocked") ||
			strings.Contains(msg, "status=retry") ||
			strings.Contains(msg, "failed=") && !strings.Contains(msg, "failed=0")
	case "verify_complete", "verify_consensus":
		msg := strings.ToLower(message)
		return strings.Contains(msg, "blocked") || strings.Contains(msg, "fail")
	}
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
		"error", "fail", "failed", "retry", "replan", "loop", "reject",
		"exhaust", "waiting", "degraded", "blocked", "interrupt", "cancel",
		"unavailable", "skipped",
	}
	for _, marker := range markers {
		if strings.Contains(e, marker) {
			return true
		}
	}
	switch e {
	case "llm_prompt", "llm_response", "llm_model_prepared", "verification_retry",
		"verify_consensus", "verify_test_start", "verify_test_pass", "verify_test_fail",
		"step_complete", "run_completed", "tool_call_begin", "tool_call_complete",
		"plan_candidate_ready", "plan_selected", "external_agent_started", "external_agent_completed":
		return true
	}
	return false
}

func shouldRecordTelemetryRecovery(eventType string) bool {
	_, ok := telemetryRecoveryTriggers[eventType]
	return ok
}

func (r *Repository) MarkTelemetryRunRunningForJob(ctx context.Context, jobID int64) error {
	if jobID <= 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE omni_runs
		SET status = 'running', updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
		  AND status = 'pending'
	`, jobID)
	return err
}

func (r *Repository) RecordTelemetryStepEvent(ctx context.Context, stepID int64, eventType, message string) error {
	if !shouldRecordTelemetryStepEvent(eventType, message) {
		return nil
	}
	var runID *string
	var jobID int64
	err := r.pool.QueryRow(ctx, `
		SELECT j.id, NULLIF(j.metadata->>'telemetry_run_id', '')
		FROM job_steps s
		JOIN jobs j ON j.id = s.job_id
		WHERE s.id = $1
	`, stepID).Scan(&jobID, &runID)
	if err != nil || runID == nil || strings.TrimSpace(*runID) == "" {
		return err
	}
	run := strings.TrimSpace(*runID)
	payload := map[string]any{
		"job_id":  jobID,
		"step_id": stepID,
		"message": strings.TrimSpace(message),
	}
	if err := r.RecordTelemetryEvent(ctx, TelemetryEventRecord{
		RunID:     run,
		EventType: strings.TrimSpace(eventType),
		Payload:   payload,
	}); err != nil {
		return err
	}
	if strategy, ok := telemetryRecoveryTriggers[eventType]; ok && shouldRecordTelemetryRecovery(eventType) {
		success := false
		_ = r.RecordTelemetryRecovery(ctx, TelemetryRecoveryRecord{
			RunID:        run,
			RecoveryKind: "worker",
			TriggerEvent: eventType,
			Strategy:     strategy,
			Success:      &success,
			Evidence:     payload,
		})
	}
	return nil
}

func (r *Repository) RecordTelemetryJobEventNow(ctx context.Context, jobID int64, eventType string, payload any) error {
	if jobID <= 0 || strings.TrimSpace(eventType) == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, event_type, payload)
		SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid, $2, $3
		FROM jobs
		WHERE id = $1 AND NULLIF(metadata->>'telemetry_run_id', '') IS NOT NULL
	`, jobID, strings.TrimSpace(eventType), jsonParam(payload))
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
	var attempts, successes int
	_ = r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE success IS TRUE)
		FROM omni_recovery_metrics
		WHERE created_at >= NOW() - INTERVAL '7 days'
	`).Scan(&attempts, &successes)
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
		RecoveryAttempts:   attempts,
		RecoverySuccesses:  successes,
		RecentStruggleRuns: struggleRuns,
	}, nil
}
