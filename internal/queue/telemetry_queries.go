package queue

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) ListTelemetryRuns(ctx context.Context, limit int) ([]TelemetryRunSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), status, started_at, finished_at, duration_ms, local_only, summary
		FROM omni_runs
		ORDER BY started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryRunSummary{}
	for rows.Next() {
		var item TelemetryRunSummary
		if err := rows.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.TaskKind, &item.ProjectType, &item.Status, &item.StartedAt, &item.FinishedAt, &item.DurationMS, &item.LocalOnly, &item.Summary); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) GetTelemetryRun(ctx context.Context, id string) (TelemetryRunSummary, []TelemetryEventSummary, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TelemetryRunSummary{}, nil, fmt.Errorf("run id is required")
	}
	var run TelemetryRunSummary
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), status, started_at, finished_at, duration_ms, local_only, summary
		FROM omni_runs
		WHERE id = $1
	`, id).Scan(&run.ID, &run.SessionID, &run.WorkspaceID, &run.TaskKind, &run.ProjectType, &run.Status, &run.StartedAt, &run.FinishedAt, &run.DurationMS, &run.LocalOnly, &run.Summary)
	if err != nil {
		return TelemetryRunSummary{}, nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, run_id::text, step, event_type, created_at, payload
		FROM omni_run_events
		WHERE run_id = $1
		ORDER BY created_at ASC, id ASC
	`, id)
	if err != nil {
		return TelemetryRunSummary{}, nil, err
	}
	defer rows.Close()
	events := []TelemetryEventSummary{}
	for rows.Next() {
		var event TelemetryEventSummary
		if err := rows.Scan(&event.ID, &event.RunID, &event.Step, &event.EventType, &event.CreatedAt, &event.Payload); err != nil {
			return TelemetryRunSummary{}, nil, err
		}
		events = append(events, event)
	}
	return run, events, rows.Err()
}

func (r *Repository) TelemetryLive(ctx context.Context) (TelemetryDashboardSummary, error) {
	live, err := r.telemetryRunsByStatus(ctx, []string{"running", "pending"}, 20)
	if err != nil {
		return TelemetryDashboardSummary{}, err
	}
	recent, err := r.ListTelemetryRuns(ctx, 20)
	if err != nil {
		return TelemetryDashboardSummary{}, err
	}
	counts, err := r.telemetryStatusCounts(ctx)
	if err != nil {
		return TelemetryDashboardSummary{}, err
	}
	blockers, err := r.telemetryEventCounts(ctx, telemetryStruggleEventTypes, 12)
	if err != nil {
		return TelemetryDashboardSummary{}, err
	}
	struggle, err := r.TelemetryStruggleSummary(ctx)
	if err != nil {
		return TelemetryDashboardSummary{}, err
	}
	return TelemetryDashboardSummary{LiveRuns: live, RecentRuns: recent, StatusCounts: counts, CommonBlockers: blockers, Struggle: struggle}, nil
}

func (r *Repository) TelemetryModelSummaries(ctx context.Context) ([]TelemetryModelSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(role,''), COALESCE(provider,''), COALESCE(model,''), COUNT(*), COUNT(*) FILTER (WHERE success IS TRUE), COUNT(*) FILTER (WHERE success IS FALSE), COALESCE(AVG(latency_ms),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(estimated_cost_usd),0)::text
		FROM omni_model_calls
		GROUP BY role, provider, model
		ORDER BY COUNT(*) DESC, role ASC, model ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryModelSummary{}
	for rows.Next() {
		var item TelemetryModelSummary
		if err := rows.Scan(&item.Role, &item.Provider, &item.Model, &item.Calls, &item.Successes, &item.Failures, &item.AvgLatencyMS, &item.InputTokens, &item.OutputTokens, &item.EstimatedCost); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) telemetryRunsByStatus(ctx context.Context, statuses []string, limit int) ([]TelemetryRunSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), status, started_at, finished_at, duration_ms, local_only, summary
		FROM omni_runs
		WHERE status = ANY($1)
		ORDER BY started_at DESC
		LIMIT $2
	`, statuses, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryRunSummary{}
	for rows.Next() {
		var item TelemetryRunSummary
		if err := rows.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.TaskKind, &item.ProjectType, &item.Status, &item.StartedAt, &item.FinishedAt, &item.DurationMS, &item.LocalOnly, &item.Summary); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) telemetryStatusCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `SELECT status, COUNT(*) FROM omni_runs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func (r *Repository) telemetryEventCounts(ctx context.Context, eventTypes []string, limit int) ([]TelemetryCountSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT event_type, COUNT(*)
		FROM omni_run_events
		WHERE event_type = ANY($1)
		GROUP BY event_type
		ORDER BY COUNT(*) DESC, event_type ASC
		LIMIT $2
	`, eventTypes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryCountSummary{}
	for rows.Next() {
		var item TelemetryCountSummary
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
