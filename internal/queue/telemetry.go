package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type TelemetryRunSummary struct {
	ID                 string          `json:"id"`
	SessionID          string          `json:"session_id,omitempty"`
	WorkspaceID        string          `json:"workspace_id,omitempty"`
	TaskKind           string          `json:"task_kind,omitempty"`
	ProjectType        string          `json:"project_type,omitempty"`
	RecipeID           string          `json:"recipe_id,omitempty"`
	PlaybookID         string          `json:"playbook_id,omitempty"`
	Status             string          `json:"status"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
	DurationMS         *int64          `json:"duration_ms,omitempty"`
	LocalOnly          bool            `json:"local_only"`
	ExternalAgentsUsed []string        `json:"external_agents_used,omitempty"`
	Summary            json.RawMessage `json:"summary,omitempty"`
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
	LiveRuns       []TelemetryRunSummary   `json:"live_runs"`
	RecentRuns     []TelemetryRunSummary   `json:"recent_runs,omitempty"`
	StatusCounts   map[string]int          `json:"status_counts,omitempty"`
	CommonBlockers []TelemetryCountSummary `json:"common_blockers,omitempty"`
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
	Malformed     int     `json:"malformed"`
	Repaired      int     `json:"repaired"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	EstimatedCost string  `json:"estimated_cost_usd,omitempty"`
}

type TelemetryPlaybookSummary struct {
	PlaybookID string `json:"playbook_id"`
	Uses       int    `json:"uses"`
	Reused     int    `json:"reused"`
	Successes  int    `json:"successes"`
	Failures   int    `json:"failures"`
}

type TelemetryBenchmarkSummary struct {
	BenchmarkID string  `json:"benchmark_id"`
	SuiteID     string  `json:"suite_id,omitempty"`
	Runs        int     `json:"runs"`
	Successes   int     `json:"successes"`
	Failures    int     `json:"failures"`
	AvgDuration float64 `json:"avg_duration_ms"`
}

type TelemetryRunRecord struct {
	ID                 string
	SessionID          string
	WorkspaceID        string
	TaskKind           string
	PromptHash         string
	PromptSummary      string
	ProjectType        string
	RecipeID           string
	PlaybookID         string
	Status             string
	StartedAt          time.Time
	FinishedAt         *time.Time
	DurationMS         *int64
	LocalOnly          bool
	ExternalAgentsUsed []string
	ModelRoles         any
	CompletionEvidence any
	Summary            any
}

type TelemetryEventRecord struct {
	RunID     string
	Step      *int
	EventType string
	CreatedAt time.Time
	Payload   any
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
	Malformed        bool
	Repaired         bool
	Success          *bool
	Metadata         any
}

type TelemetryToolCallRecord struct {
	RunID      string
	ToolKind   string
	ToolName   string
	StartedAt  *time.Time
	FinishedAt *time.Time
	LatencyMS  *int64
	Success    *bool
	Metadata   any
}

type TelemetryCommandObservationRecord struct {
	RunID       string
	CommandID   string
	Step        *int
	Attempt     *int
	Command     string
	CWD         string
	ExitCode    *int
	Stdout      string
	Stderr      string
	ObjectiveID string
	WorkItemID  string
	Source      string
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Metadata    any
}

type TelemetryObjectiveRecord struct {
	RunID            string
	ObjectiveID      string
	Status           string
	Kind             string
	Required         bool
	RequiredEvidence any
	Evidence         any
	CompletedAt      *time.Time
}

type TelemetryRecoveryRecord struct {
	RunID           string
	RecoveryKind    string
	TriggerEvent    string
	Strategy        string
	Success         *bool
	StepsToSuccess  *int
	StuckDurationMS *int64
	Evidence        any
}

type TelemetryPlaybookUsageRecord struct {
	RunID               string
	PlaybookID          string
	Version             string
	UsageType           string
	Reused              bool
	Success             *bool
	ImprovementDetected bool
	SupersededBy        string
	Evidence            any
}

type TelemetryBenchmarkRecord struct {
	RunID       string
	BenchmarkID string
	SuiteID     string
	Status      string
	DurationMS  *int64
	LocalOnly   bool
	Models      any
	Metrics     any
	Evidence    any
}

func (r *Repository) RecordTelemetryRun(ctx context.Context, record TelemetryRunRecord) (string, error) {
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = "running"
	}
	started := record.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	localOnly := record.LocalOnly
	if len(record.ExternalAgentsUsed) > 0 {
		localOnly = false
	} else if !record.LocalOnly {
		localOnly = true
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO omni_runs (id, session_id, workspace_id, task_kind, prompt_hash, prompt_summary, project_type, recipe_id, playbook_id, status, started_at, finished_at, duration_ms, local_only, external_agents_used, model_roles, completion_evidence, summary)
		VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			finished_at = EXCLUDED.finished_at,
			duration_ms = EXCLUDED.duration_ms,
			external_agents_used = EXCLUDED.external_agents_used,
			model_roles = EXCLUDED.model_roles,
			completion_evidence = EXCLUDED.completion_evidence,
			summary = EXCLUDED.summary,
			updated_at = NOW()
		RETURNING id::text
	`, record.ID, record.SessionID, record.WorkspaceID, record.TaskKind, record.PromptHash, record.PromptSummary, record.ProjectType, record.RecipeID, record.PlaybookID, status, started, record.FinishedAt, record.DurationMS, localOnly, record.ExternalAgentsUsed, jsonParam(record.ModelRoles), jsonParam(record.CompletionEvidence), jsonParam(record.Summary)).Scan(&id)
	return id, err
}

func (r *Repository) CompleteTelemetryRun(ctx context.Context, runID, status string, summary any, completionEvidence any) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE omni_runs
		SET status = $2, finished_at = NOW(), duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint), summary = $3, completion_evidence = $4, updated_at = NOW()
		WHERE id = $1
	`, strings.TrimSpace(runID), status, jsonParam(summary), jsonParam(completionEvidence))
	return err
}

func (r *Repository) RecordTelemetryEvent(ctx context.Context, record TelemetryEventRecord) error {
	eventType := strings.TrimSpace(record.EventType)
	if eventType == "" {
		return fmt.Errorf("event type is required")
	}
	created := record.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, step, event_type, created_at, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, strings.TrimSpace(record.RunID), record.Step, eventType, created, jsonParam(record.Payload))
	return err
}

func (r *Repository) RecordTelemetryModelCall(ctx context.Context, record TelemetryModelCallRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_model_calls (run_id, role, provider, model, started_at, finished_at, latency_ms, input_tokens, output_tokens, estimated_cost_usd, malformed, repaired, success, metadata)
		VALUES ($1, NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), $5, $6, $7, $8, $9, NULLIF($10,'')::numeric, $11, $12, $13, $14)
	`, strings.TrimSpace(record.RunID), record.Role, record.Provider, record.Model, record.StartedAt, record.FinishedAt, record.LatencyMS, record.InputTokens, record.OutputTokens, valueString(record.EstimatedCostUSD), record.Malformed, record.Repaired, record.Success, jsonParam(record.Metadata))
	return err
}

func (r *Repository) RecordTelemetryToolCall(ctx context.Context, record TelemetryToolCallRecord) error {
	toolKind := strings.TrimSpace(record.ToolKind)
	if toolKind == "" {
		return fmt.Errorf("tool kind is required")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_tool_calls (run_id, tool_kind, tool_name, started_at, finished_at, latency_ms, success, metadata)
		VALUES ($1, $2, NULLIF($3,''), $4, $5, $6, $7, $8)
	`, strings.TrimSpace(record.RunID), toolKind, record.ToolName, record.StartedAt, record.FinishedAt, record.LatencyMS, record.Success, jsonParam(record.Metadata))
	return err
}

func (r *Repository) RecordTelemetryCommandObservation(ctx context.Context, record TelemetryCommandObservationRecord) error {
	commandID := strings.TrimSpace(record.CommandID)
	if commandID == "" {
		return fmt.Errorf("command id is required")
	}
	command := strings.TrimSpace(record.Command)
	if command == "" {
		return fmt.Errorf("command is required")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_command_observations (run_id, command_id, step, attempt, command, cwd, exit_code, stdout, stderr, objective_id, work_item_id, source, started_at, finished_at, metadata)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), $7, $8, $9, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), $13, $14, $15)
		ON CONFLICT (run_id, command_id) DO UPDATE SET
			step = EXCLUDED.step,
			attempt = EXCLUDED.attempt,
			command = EXCLUDED.command,
			cwd = EXCLUDED.cwd,
			exit_code = EXCLUDED.exit_code,
			stdout = EXCLUDED.stdout,
			stderr = EXCLUDED.stderr,
			objective_id = EXCLUDED.objective_id,
			work_item_id = EXCLUDED.work_item_id,
			source = EXCLUDED.source,
			started_at = EXCLUDED.started_at,
			finished_at = EXCLUDED.finished_at,
			metadata = EXCLUDED.metadata
	`, strings.TrimSpace(record.RunID), commandID, record.Step, record.Attempt, command, record.CWD, record.ExitCode, record.Stdout, record.Stderr, record.ObjectiveID, record.WorkItemID, record.Source, record.StartedAt, record.FinishedAt, jsonParam(record.Metadata))
	return err
}

func (r *Repository) RecordTelemetryObjective(ctx context.Context, record TelemetryObjectiveRecord) error {
	if strings.TrimSpace(record.ObjectiveID) == "" {
		return fmt.Errorf("objective id is required")
	}
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = "pending"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_objective_metrics (run_id, objective_id, status, kind, required, required_evidence, evidence, completed_at)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7, $8)
	`, strings.TrimSpace(record.RunID), strings.TrimSpace(record.ObjectiveID), status, record.Kind, record.Required, jsonArrayParam(record.RequiredEvidence), jsonParam(record.Evidence), record.CompletedAt)
	return err
}

func (r *Repository) RecordTelemetryRecovery(ctx context.Context, record TelemetryRecoveryRecord) error {
	if strings.TrimSpace(record.RecoveryKind) == "" {
		return fmt.Errorf("recovery kind is required")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_recovery_metrics (run_id, recovery_kind, trigger_event, strategy, success, steps_to_success, stuck_duration_ms, evidence)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7, $8)
	`, strings.TrimSpace(record.RunID), strings.TrimSpace(record.RecoveryKind), record.TriggerEvent, record.Strategy, record.Success, record.StepsToSuccess, record.StuckDurationMS, jsonParam(record.Evidence))
	return err
}

func (r *Repository) RecordTelemetryPlaybookUsage(ctx context.Context, record TelemetryPlaybookUsageRecord) error {
	if strings.TrimSpace(record.PlaybookID) == "" {
		return fmt.Errorf("playbook id is required")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_playbook_usage (run_id, playbook_id, version, usage_type, reused, success, improvement_detected, superseded_by, evidence)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7, NULLIF($8,''), $9)
	`, strings.TrimSpace(record.RunID), strings.TrimSpace(record.PlaybookID), record.Version, record.UsageType, record.Reused, record.Success, record.ImprovementDetected, record.SupersededBy, jsonParam(record.Evidence))
	return err
}

func (r *Repository) RecordTelemetryBenchmarkResult(ctx context.Context, record TelemetryBenchmarkRecord) error {
	if strings.TrimSpace(record.BenchmarkID) == "" {
		return fmt.Errorf("benchmark id is required")
	}
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = "unknown"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_benchmark_results (run_id, benchmark_id, suite_id, status, duration_ms, local_only, models, metrics, evidence)
		VALUES (NULLIF($1,'')::uuid, $2, NULLIF($3,''), $4, $5, $6, $7, $8, $9)
	`, strings.TrimSpace(record.RunID), strings.TrimSpace(record.BenchmarkID), record.SuiteID, status, record.DurationMS, record.LocalOnly, jsonParam(record.Models), jsonParam(record.Metrics), jsonParam(record.Evidence))
	return err
}

func (r *Repository) ListTelemetryRuns(ctx context.Context, limit int) ([]TelemetryRunSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), COALESCE(recipe_id,''), COALESCE(playbook_id,''), status, started_at, finished_at, duration_ms, local_only, external_agents_used, summary
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
		if err := rows.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.TaskKind, &item.ProjectType, &item.RecipeID, &item.PlaybookID, &item.Status, &item.StartedAt, &item.FinishedAt, &item.DurationMS, &item.LocalOnly, &item.ExternalAgentsUsed, &item.Summary); err != nil {
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
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), COALESCE(recipe_id,''), COALESCE(playbook_id,''), status, started_at, finished_at, duration_ms, local_only, external_agents_used, summary
		FROM omni_runs
		WHERE id = $1
	`, id).Scan(&run.ID, &run.SessionID, &run.WorkspaceID, &run.TaskKind, &run.ProjectType, &run.RecipeID, &run.PlaybookID, &run.Status, &run.StartedAt, &run.FinishedAt, &run.DurationMS, &run.LocalOnly, &run.ExternalAgentsUsed, &run.Summary)
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
	blockers, err := r.telemetryEventCounts(ctx, append(telemetryStruggleEventTypes, []string{
		"structured_payload_rejected_mixed_ask_command",
		"structured_user_input_cancelled",
		"pathfinder_strategy_selected",
	}...), 12)
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
		SELECT COALESCE(role,''), COALESCE(provider,''), COALESCE(model,''), COUNT(*), COUNT(*) FILTER (WHERE success IS TRUE), COUNT(*) FILTER (WHERE success IS FALSE), COUNT(*) FILTER (WHERE malformed), COUNT(*) FILTER (WHERE repaired), COALESCE(AVG(latency_ms),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(estimated_cost_usd),0)::text
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
		if err := rows.Scan(&item.Role, &item.Provider, &item.Model, &item.Calls, &item.Successes, &item.Failures, &item.Malformed, &item.Repaired, &item.AvgLatencyMS, &item.InputTokens, &item.OutputTokens, &item.EstimatedCost); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) TelemetryPlaybookSummaries(ctx context.Context) ([]TelemetryPlaybookSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT playbook_id, COUNT(*), COUNT(*) FILTER (WHERE reused), COUNT(*) FILTER (WHERE success IS TRUE), COUNT(*) FILTER (WHERE success IS FALSE)
		FROM omni_playbook_usage
		GROUP BY playbook_id
		ORDER BY COUNT(*) DESC, playbook_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryPlaybookSummary{}
	for rows.Next() {
		var item TelemetryPlaybookSummary
		if err := rows.Scan(&item.PlaybookID, &item.Uses, &item.Reused, &item.Successes, &item.Failures); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) TelemetryBenchmarkSummaries(ctx context.Context) ([]TelemetryBenchmarkSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT benchmark_id, COALESCE(suite_id,''), COUNT(*), COUNT(*) FILTER (WHERE status = 'success'), COUNT(*) FILTER (WHERE status <> 'success'), COALESCE(AVG(duration_ms),0)
		FROM omni_benchmark_results
		GROUP BY benchmark_id, suite_id
		ORDER BY benchmark_id ASC, suite_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryBenchmarkSummary{}
	for rows.Next() {
		var item TelemetryBenchmarkSummary
		if err := rows.Scan(&item.BenchmarkID, &item.SuiteID, &item.Runs, &item.Successes, &item.Failures, &item.AvgDuration); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) telemetryRunsByStatus(ctx context.Context, statuses []string, limit int) ([]TelemetryRunSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(session_id,''), COALESCE(workspace_id,''), COALESCE(task_kind,''), COALESCE(project_type,''), COALESCE(recipe_id,''), COALESCE(playbook_id,''), status, started_at, finished_at, duration_ms, local_only, external_agents_used, summary
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
		if err := rows.Scan(&item.ID, &item.SessionID, &item.WorkspaceID, &item.TaskKind, &item.ProjectType, &item.RecipeID, &item.PlaybookID, &item.Status, &item.StartedAt, &item.FinishedAt, &item.DurationMS, &item.LocalOnly, &item.ExternalAgentsUsed, &item.Summary); err != nil {
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

func jsonParam(value any) []byte {
	if value == nil {
		return []byte(`{}`)
	}
	if raw, ok := value.(json.RawMessage); ok {
		if len(raw) == 0 {
			return []byte(`{}`)
		}
		return raw
	}
	if raw, ok := value.([]byte); ok {
		if len(raw) == 0 {
			return []byte(`{}`)
		}
		return raw
	}
	blob, err := json.Marshal(value)
	if err != nil || len(blob) == 0 || string(blob) == "null" {
		return []byte(`{}`)
	}
	return blob
}

func jsonArrayParam(value any) []byte {
	if value == nil {
		return []byte(`[]`)
	}
	if raw, ok := value.(json.RawMessage); ok {
		if len(raw) == 0 {
			return []byte(`[]`)
		}
		return raw
	}
	blob, err := json.Marshal(value)
	if err != nil || len(blob) == 0 || string(blob) == "null" {
		return []byte(`[]`)
	}
	return blob
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
