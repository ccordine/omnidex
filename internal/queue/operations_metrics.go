package queue

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

var telemetryFailureEventTypes = []string{
	"step_error",
	"step_interrupted",
	"step_canceled",
	"llm_error",
	"llm_retry_model",
	"llm_retry_same_model",
	"verify_test_fail",
	"verify_auto_replan",
	"verify_replan",
	"verify_hallucination_retry",
	"verify_hallucination_loop",
	"verify_ollama_restart_failed",
	"verification_retry",
	"verify_grounding_retry",
	"tool_call_rejected",
	"external_agent_failed",
	"progression_gate_failed",
	"progression_gate_rejected_false_done",
	"structured_loop_exhausted",
	"structured_command_rejected",
	"artifact_validation_failed",
	"plan_candidate_fallback",
	"plan_waiting_input",
	"analyze_waiting_input",
	"response_waiting_input",
	"tooling_waiting_input",
	"web_search_waiting_input",
	"web_search_degraded",
	"retrieve_embedding_error",
	"workspace_scan_waiting_input",
}

var telemetryLoopEventTypes = []string{
	"verification_retry",
	"verify_hallucination_retry",
	"verify_grounding_retry",
	"verify_auto_replan",
	"verify_replan",
	"verify_hallucination_loop",
	"llm_retry_model",
	"llm_retry_same_model",
	"structured_loop_exhausted",
}

type OperationsLoopStat struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	AvgPerRun     float64 `json:"avg_per_run"`
	MaxPerRun     int     `json:"max_per_run"`
	TotalEvents   int     `json:"total_events"`
	RunsAffected  int     `json:"runs_affected"`
	PriorAvgPerRun float64 `json:"prior_avg_per_run"`
	DeltaPct      float64 `json:"delta_pct"`
}

type OperationsFailureEvent struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id,omitempty"`
	EventType string          `json:"event_type"`
	Message   string          `json:"message,omitempty"`
	JobID     int64           `json:"job_id,omitempty"`
	StepID    int64           `json:"step_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type OperationsRunDiagnostic struct {
	RunID        string          `json:"run_id"`
	Status       string          `json:"status"`
	TaskKind     string          `json:"task_kind,omitempty"`
	DurationMS   *int64          `json:"duration_ms,omitempty"`
	LoopEvents   int             `json:"loop_events"`
	FailureEvents int            `json:"failure_events"`
	LLMCalls     int             `json:"llm_calls"`
	MaxPromptChars int           `json:"max_prompt_chars"`
	Summary      json.RawMessage `json:"summary,omitempty"`
	StartedAt    time.Time       `json:"started_at"`
}

type OperationsContextFlood struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	Scope        string    `json:"scope,omitempty"`
	Model        string    `json:"model,omitempty"`
	RunID        string    `json:"run_id,omitempty"`
	SentChars    int       `json:"sent_chars"`
	DeltaChars   int       `json:"delta_chars"`
	Utilization  float64   `json:"utilization_pct"`
	Success      bool      `json:"success"`
	ErrorClass   string    `json:"error_class,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type OperationsMetricsResponse struct {
	FailureCounts   []TelemetryCountSummary  `json:"failure_counts"`
	RecentFailures  []OperationsFailureEvent `json:"recent_failures"`
	LoopStats       []OperationsLoopStat     `json:"loop_stats"`
	ContextFloods   []OperationsContextFlood `json:"context_floods"`
	RunDiagnostics  []OperationsRunDiagnostic `json:"run_diagnostics"`
	LLMFailures     int                      `json:"llm_failures"`
	LLMFailureRate  float64                  `json:"llm_failure_rate_pct"`
	AvgContextDelta float64                  `json:"avg_context_delta_chars"`
}

func (r *Repository) TelemetryJobContextForStep(ctx context.Context, stepID int64) (runID string, jobID int64, err error) {
	if stepID <= 0 {
		return "", 0, nil
	}
	var runPtr *string
	err = r.pool.QueryRow(ctx, `
		SELECT j.id, NULLIF(j.metadata->>'telemetry_run_id', '')
		FROM job_steps s
		JOIN jobs j ON j.id = s.job_id
		WHERE s.id = $1
	`, stepID).Scan(&jobID, &runPtr)
	if err != nil || runPtr == nil {
		return "", jobID, err
	}
	return strings.TrimSpace(*runPtr), jobID, nil
}

func (r *Repository) OperationsMetrics(ctx context.Context) (OperationsMetricsResponse, error) {
	out := OperationsMetricsResponse{}

	failureCounts, err := r.telemetryEventCounts(ctx, telemetryFailureEventTypes, 24)
	if err != nil {
		return OperationsMetricsResponse{}, err
	}
	out.FailureCounts = failureCounts

	recentRows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(run_id::text,''), event_type,
			COALESCE(payload->>'message',''), COALESCE((payload->>'job_id')::bigint, 0),
			COALESCE((payload->>'step_id')::bigint, 0), payload, created_at
		FROM omni_run_events
		WHERE created_at >= NOW() - INTERVAL '7 days'
		  AND (
		    event_type = ANY($1)
		    OR event_type LIKE '%error%'
		    OR event_type LIKE '%fail%'
		    OR event_type LIKE '%retry%'
		    OR event_type LIKE '%replan%'
		    OR event_type LIKE '%loop%'
		    OR event_type LIKE '%reject%'
		    OR event_type LIKE '%waiting%'
		  )
		ORDER BY created_at DESC
		LIMIT 40
	`, telemetryFailureEventTypes)
	if err != nil {
		return OperationsMetricsResponse{}, err
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var item OperationsFailureEvent
		if err := recentRows.Scan(&item.ID, &item.RunID, &item.EventType, &item.Message, &item.JobID, &item.StepID, &item.Payload, &item.CreatedAt); err != nil {
			return OperationsMetricsResponse{}, err
		}
		out.RecentFailures = append(out.RecentFailures, item)
	}
	if err := recentRows.Err(); err != nil {
		return OperationsMetricsResponse{}, err
	}

	out.LoopStats, err = r.operationsLoopStats(ctx)
	if err != nil {
		return OperationsMetricsResponse{}, err
	}

	out.ContextFloods, err = r.operationsContextFloods(ctx, 20)
	if err != nil {
		return OperationsMetricsResponse{}, err
	}

	out.RunDiagnostics, err = r.operationsRunDiagnostics(ctx, 12)
	if err != nil {
		return OperationsMetricsResponse{}, err
	}

	_ = r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE NOT success),
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE NOT success)::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END,
			COALESCE(AVG(NULLIF(delta_chars, 0)), 0)
		FROM omni_llm_context_usage
		WHERE created_at >= NOW() - INTERVAL '7 days'
	`).Scan(&out.LLMFailures, &out.LLMFailureRate, &out.AvgContextDelta)
	out.AvgContextDelta = math.Round(out.AvgContextDelta*10) / 10

	return out, nil
}

func (r *Repository) operationsLoopStats(ctx context.Context) ([]OperationsLoopStat, error) {
	labels := map[string]string{
		"verification_retry":       "Verification retries",
		"verify_hallucination_retry": "Hallucination retries",
		"verify_grounding_retry":   "Grounding retries",
		"verify_auto_replan":       "Auto replans",
		"verify_replan":            "Manual replans",
		"verify_hallucination_loop": "Hallucination loops",
		"llm_retry_model":          "LLM model fallbacks",
		"llm_retry_same_model":     "LLM same-model retries",
		"structured_loop_exhausted": "Structured loop exhaustions",
	}
	stats := make([]OperationsLoopStat, 0, len(telemetryLoopEventTypes))
	for _, eventType := range telemetryLoopEventTypes {
		item, err := r.operationsLoopStatForEvent(ctx, eventType, labels[eventType])
		if err != nil {
			return nil, err
		}
		stats = append(stats, item)
	}
	return stats, nil
}

func (r *Repository) operationsLoopStatForEvent(ctx context.Context, eventType, label string) (OperationsLoopStat, error) {
	item := OperationsLoopStat{Key: eventType, Label: label}
	if label == "" {
		item.Label = eventType
	}
	err := r.pool.QueryRow(ctx, `
		WITH current AS (
			SELECT run_id, COUNT(*) AS cnt
			FROM omni_run_events
			WHERE event_type = $1 AND created_at >= NOW() - INTERVAL '7 days' AND run_id IS NOT NULL
			GROUP BY run_id
		),
		prior AS (
			SELECT run_id, COUNT(*) AS cnt
			FROM omni_run_events
			WHERE event_type = $1
			  AND created_at >= NOW() - INTERVAL '14 days'
			  AND created_at < NOW() - INTERVAL '7 days'
			  AND run_id IS NOT NULL
			GROUP BY run_id
		)
		SELECT COALESCE((SELECT COUNT(*) FROM omni_run_events WHERE event_type = $1 AND created_at >= NOW() - INTERVAL '7 days'), 0),
			COALESCE((SELECT COUNT(DISTINCT run_id) FROM current), 0),
			COALESCE((SELECT AVG(cnt) FROM current), 0),
			COALESCE((SELECT MAX(cnt) FROM current), 0),
			COALESCE((SELECT AVG(cnt) FROM prior), 0)
	`, eventType).Scan(&item.TotalEvents, &item.RunsAffected, &item.AvgPerRun, &item.MaxPerRun, &item.PriorAvgPerRun)
	if err != nil {
		return OperationsLoopStat{}, err
	}
	item.AvgPerRun = math.Round(item.AvgPerRun*100) / 100
	item.PriorAvgPerRun = math.Round(item.PriorAvgPerRun*100) / 100
	if item.PriorAvgPerRun > 0 {
		item.DeltaPct = math.Round((item.AvgPerRun-item.PriorAvgPerRun)/item.PriorAvgPerRun*1000) / 10
	}
	return item, nil
}

func (r *Repository) operationsContextFloods(ctx context.Context, limit int) ([]OperationsContextFlood, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, source, COALESCE(scope,''), COALESCE(model,''), COALESCE(run_id::text,''),
			sent_chars, delta_chars, utilization_pct, success, COALESCE(error_class,''), created_at
		FROM omni_llm_context_usage
		WHERE created_at >= NOW() - INTERVAL '7 days'
		  AND (delta_chars >= 2000 OR overloaded OR NOT success)
		ORDER BY delta_chars DESC, sent_chars DESC, created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OperationsContextFlood, 0, limit)
	for rows.Next() {
		var item OperationsContextFlood
		if err := rows.Scan(&item.ID, &item.Source, &item.Scope, &item.Model, &item.RunID, &item.SentChars, &item.DeltaChars, &item.Utilization, &item.Success, &item.ErrorClass, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) operationsRunDiagnostics(ctx context.Context, limit int) ([]OperationsRunDiagnostic, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.status, COALESCE(r.task_kind,''), r.duration_ms, r.summary, r.started_at,
			COALESCE((SELECT COUNT(*) FROM omni_run_events e WHERE e.run_id = r.id AND e.event_type = ANY($2)), 0),
			COALESCE((SELECT COUNT(*) FROM omni_run_events e WHERE e.run_id = r.id AND (
				e.event_type LIKE '%fail%' OR e.event_type LIKE '%error%' OR e.event_type LIKE '%reject%'
			)), 0),
			COALESCE((SELECT COUNT(*) FROM omni_llm_context_usage u WHERE u.run_id = r.id), 0),
			COALESCE((SELECT MAX(sent_chars) FROM omni_llm_context_usage u WHERE u.run_id = r.id), 0)
		FROM omni_runs r
		WHERE r.started_at >= NOW() - INTERVAL '7 days'
		ORDER BY r.started_at DESC
		LIMIT $1
	`, limit, telemetryLoopEventTypes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OperationsRunDiagnostic, 0, limit)
	for rows.Next() {
		var item OperationsRunDiagnostic
		if err := rows.Scan(&item.RunID, &item.Status, &item.TaskKind, &item.DurationMS, &item.Summary, &item.StartedAt, &item.LoopEvents, &item.FailureEvents, &item.LLMCalls, &item.MaxPromptChars); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
