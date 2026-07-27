package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/chat"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectdebugger"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/gryph/omnidex/internal/scrumcardllm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

type stepSeed struct {
	action    string
	sortIndex int
}

const inferredMemoryCorrectionDistance = 0.08

var channelIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.:-]+`)

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return r.pool.Ping(ctx)
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, schemaSQL); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, v3SchemaSQL); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, telemetrySchemaSQL); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, projectsUISchemaSQL); err != nil {
		return err
	}
	if err := r.ApplyFileMigrations(ctx, ResolveMigrationsDir()); err != nil {
		return err
	}
	if err := r.BackfillMemoryCategories(ctx); err != nil {
		return err
	}
	if err := r.BackfillScrumBoardOrder(ctx); err != nil {
		return err
	}
	return nil
}

func (r *Repository) MigrateFresh(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = current_schema()
		ORDER BY tablename ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	names := make([]string, 0, 32)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range names {
		if _, err := r.pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s" CASCADE`, strings.ReplaceAll(name, `"`, `""`))); err != nil {
			return err
		}
	}

	return r.EnsureSchema(ctx)
}

func (r *Repository) EnqueueJob(ctx context.Context, instruction, pipeline string, metadataJSON []byte) (model.Job, error) {
	pipeline, err := validatePipeline(pipeline)
	if err != nil {
		return model.Job{}, err
	}
	if len(metadataJSON) == 0 {
		metadataJSON = []byte(`{}`)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	job, err := r.enqueueJobTx(ctx, tx, instruction, pipeline, metadataJSON)
	if err != nil {
		return model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func (r *Repository) enqueueJobTx(ctx context.Context, tx pgx.Tx, instruction, pipeline string, metadataJSON []byte) (model.Job, error) {
	projectID, err := resolveProjectID(ctx, tx, metadataJSON)
	if err != nil {
		return model.Job{}, err
	}

	var job model.Job
	var result, errText *string
	instruction = SanitizeUTF8Text(instruction)
	metadataJSON = SanitizeUTF8Bytes(metadataJSON)
	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata, project_id)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		RETURNING id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
	`, instruction, pipeline, model.JobStatusPending, string(metadataJSON), projectID).Scan(
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&result,
		&errText,
		&job.Metadata,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	)
	if err != nil {
		return model.Job{}, err
	}
	job.Result = stringOrEmpty(result)
	job.Error = stringOrEmpty(errText)

	telemetryRunID, err := createTelemetryRunForJob(ctx, tx, job, projectID)
	if err != nil {
		return model.Job{}, err
	}
	if telemetryRunID != "" {
		if err := tx.QueryRow(ctx, `
			UPDATE jobs
			SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{telemetry_run_id}', to_jsonb($2::text), true)
			WHERE id = $1
			RETURNING metadata
		`, job.ID, telemetryRunID).Scan(&job.Metadata); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, job.ID, "run_started", map[string]any{
			"job_id":   job.ID,
			"pipeline": job.Pipeline,
			"status":   job.Status,
		}); err != nil {
			return model.Job{}, err
		}
	}

	steps, err := stepsForJob(pipeline, instruction, metadataJSON)
	if err != nil {
		return model.Job{}, fmt.Errorf("resolve job execution steps: %w", err)
	}
	if len(steps) == 0 {
		return model.Job{}, fmt.Errorf("pipeline %q produced no executable steps", pipeline)
	}
	for _, step := range steps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status)
			VALUES ($1, $2, $3, $4)
		`, job.ID, step.action, step.sortIndex, model.StepStatusPending); err != nil {
			return model.Job{}, err
		}
	}
	return job, nil
}

func resolveProjectID(ctx context.Context, tx pgx.Tx, metadataJSON []byte) (*int64, error) {
	ref, err := projectReferenceFromMetadata(metadataJSON)
	if err != nil {
		return nil, err
	}
	if ref.HasProjectID {
		var location string
		err := tx.QueryRow(ctx, `
			UPDATE projects
			SET last_seen_at = NOW(), updated_at = NOW()
			WHERE id = $1
			RETURNING location
		`, ref.ProjectID).Scan(&location)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("project_id %d does not exist", ref.ProjectID)
		}
		if err != nil {
			return nil, err
		}
		if ref.Location != "" && filepath.Clean(location) != ref.Location {
			return nil, fmt.Errorf(
				"job metadata project mismatch: project_id %d owns %q, received %q",
				ref.ProjectID,
				filepath.Clean(location),
				ref.Location,
			)
		}
		projectID := ref.ProjectID
		return &projectID, nil
	}
	if ref.Location == "" {
		return nil, nil
	}
	name := projectNameFromLocation(ref.Location)

	var projectID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO projects (location, name, last_seen_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (location) DO UPDATE
		SET last_seen_at = NOW(),
		    updated_at = NOW()
		RETURNING id
	`, ref.Location, name).Scan(&projectID)
	if err != nil {
		return nil, err
	}
	return &projectID, nil
}

type metadataProjectReference struct {
	ProjectID    int64
	HasProjectID bool
	Location     string
}

func projectReferenceFromMetadata(metadataJSON []byte) (metadataProjectReference, error) {
	if len(metadataJSON) == 0 {
		return metadataProjectReference{}, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return metadataProjectReference{}, fmt.Errorf("parse job metadata: %w", err)
	}
	ref := metadataProjectReference{}
	if raw, ok := payload["project_id"]; ok {
		if err := json.Unmarshal(raw, &ref.ProjectID); err != nil {
			return metadataProjectReference{}, fmt.Errorf("project_id must be a positive integer: %w", err)
		}
		if ref.ProjectID <= 0 {
			return metadataProjectReference{}, fmt.Errorf("project_id must be a positive integer")
		}
		ref.HasProjectID = true
	}
	for _, key := range []string{"client_cwd", "host_env_cwd"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var location string
		if err := json.Unmarshal(raw, &location); err != nil {
			return metadataProjectReference{}, fmt.Errorf("%s must be a string: %w", key, err)
		}
		if location = strings.TrimSpace(location); location != "" {
			ref.Location = filepath.Clean(location)
			break
		}
	}
	return ref, nil
}

func createTelemetryRunForJob(ctx context.Context, tx pgx.Tx, job model.Job, projectID *int64) (string, error) {
	if job.ID <= 0 {
		return "", nil
	}
	metadata := decodeMetadataObject(job.Metadata)
	workspaceID := strings.TrimSpace(firstMetadataString(metadata, "workspace_id", "workspace", "workspace_root", "project_location"))
	if workspaceID == "" {
		workspaceID = projectLocationFromMetadata(job.Metadata)
	}
	projectType := strings.TrimSpace(firstMetadataString(metadata, "project_type", "framework", "stack"))
	taskKind := strings.TrimSpace(firstMetadataString(metadata, "task_kind", "kind"))
	if taskKind == "" {
		taskKind = inferTelemetryTaskKind(job.Pipeline, job.Instruction, metadata)
	}
	promptHash := telemetryPromptHash(job.Instruction)
	promptSummary := telemetryPromptSummary(job.Instruction, 240)
	summary := map[string]any{
		"job_id":         job.ID,
		"pipeline":       job.Pipeline,
		"project_id":     projectID,
		"prompt_summary": promptSummary,
	}
	externalAgents := pgTextArray(metadataStringSlice(metadata, "external_agents_used"))
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO omni_runs (session_id, workspace_id, task_kind, prompt_hash, prompt_summary, project_type, recipe_id, playbook_id, status, started_at, local_only, external_agents_used, model_roles, summary)
		VALUES (NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), $9, $10, $11, $12, $13, $14)
		RETURNING id::text
	`, firstMetadataString(metadata, "session_id"), workspaceID, taskKind, promptHash, promptSummary, projectType, firstMetadataString(metadata, "recipe_id"), firstMetadataString(metadata, "playbook_id"), "pending", job.CreatedAt, len(externalAgents) == 0, externalAgents, jsonParam(metadataValue(metadata, "model_roles")), jsonParam(summary)).Scan(&id)
	return id, err
}

func completeTelemetryRunForJob(ctx context.Context, tx pgx.Tx, jobID int64, status string, summary any, completionEvidence any) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	_, err := tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = $2,
		    finished_at = NOW(),
		    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint),
		    summary = $3,
		    completion_evidence = $4,
		    updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
	`, jobID, status, jsonParam(summary), jsonParam(completionEvidence))
	return err
}

func recordTelemetryJobEvent(ctx context.Context, tx pgx.Tx, jobID int64, eventType string, payload any) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO omni_run_events (run_id, event_type, payload)
		SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid, $2, $3
		FROM jobs
		WHERE id = $1 AND NULLIF(metadata->>'telemetry_run_id', '') IS NOT NULL
	`, jobID, eventType, jsonParam(payload))
	return err
}

func markTelemetryRunRunningForJob(ctx context.Context, tx pgx.Tx, jobID int64) error {
	if jobID <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE omni_runs
		SET status = 'running', updated_at = NOW()
		WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid
		  AND status = 'pending'
	`, jobID)
	return err
}

func decodeMetadataObject(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func firstMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(metadata[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func metadataValue(metadata map[string]any, key string) any {
	if value, ok := metadata[key]; ok && value != nil {
		return value
	}
	return map[string]any{}
}

func metadataStringSlice(metadata map[string]any, key string) []string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		return pgTextArray(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case string:
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		return []string{}
	}
}

// pgTextArray ensures pgx sends an empty Postgres text[] instead of NULL.
func pgTextArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func inferTelemetryTaskKind(pipeline, instruction string, metadata map[string]any) string {
	if kind := strings.TrimSpace(firstMetadataString(metadata, "research_topic")); kind != "" {
		return "research"
	}
	pipeline = strings.ToLower(strings.TrimSpace(pipeline))
	switch pipeline {
	case model.PipelineCoding:
		return "coding"
	case model.PipelineStory:
		return "story"
	case model.PipelineChat:
		return "chat"
	}
	lower := strings.ToLower(instruction)
	switch {
	case strings.Contains(lower, "research"), strings.Contains(lower, "look up"), strings.Contains(lower, "search"):
		return "research"
	case strings.Contains(lower, "build"), strings.Contains(lower, "code"), strings.Contains(lower, "test"), strings.Contains(lower, "fix"):
		return "coding"
	default:
		return pipeline
	}
}

func telemetryPromptHash(instruction string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(instruction)))
	return fmt.Sprintf("%x", sum[:])
}

func telemetryPromptSummary(instruction string, max int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(instruction)), " ")
	if max > 0 && len(text) > max {
		return TruncateUTF8Text(text, max, "...[redacted]")
	}
	return text
}

func projectLocationFromMetadata(metadataJSON []byte) string {
	if len(metadataJSON) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"client_cwd", "host_env_cwd"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		return filepath.Clean(text)
	}
	return ""
}

func projectNameFromLocation(location string) string {
	location = strings.TrimSpace(filepath.Clean(location))
	if location == "" || location == "." {
		return "workspace"
	}
	base := strings.TrimSpace(filepath.Base(location))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return location
	}
	return base
}

func usesV3NativeSteps(metadataJSON []byte) bool {
	if len(metadataJSON) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return false
	}
	for _, key := range []string{"runtime", "engine", "execution_mode"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", raw)))
		if text == "v3" || text == "native_v3" || text == "native-v3" {
			return true
		}
	}
	if raw, ok := payload["v3_enabled"]; ok {
		switch typed := raw.(type) {
		case bool:
			return typed
		case string:
			text := strings.ToLower(strings.TrimSpace(typed))
			return text == "true" || text == "1" || text == "yes" || text == "on"
		}
	}
	return false
}

func stepsForJob(pipeline, instruction string, metadataJSON []byte) ([]stepSeed, error) {
	agentCfg, err := agentconfig.FromJobMetadata(metadataJSON)
	if err != nil {
		return nil, err
	}
	if datasource.IsExploreJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineDataExplore {
		return []stepSeed{{action: "data_source_explore", sortIndex: 1}}, nil
	}
	if projectdebugger.IsJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineProjectDebugger {
		return []stepSeed{{action: "project_debugger", sortIndex: 1}}, nil
	}
	if scrumcardllm.IsJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineScrumCardLLM {
		return []stepSeed{{action: "scrum_card_llm", sortIndex: 1}}, nil
	}
	if isDataSourceQueryJob(metadataJSON) || normalizePipeline(pipeline) == model.PipelineDataQuery {
		return []stepSeed{{action: "data_source_query", sortIndex: 1}}, nil
	}
	if agentCfg.IsExternal() {
		return []stepSeed{{action: "external_agent_execute", sortIndex: 1}}, nil
	}
	if scrum.IsScrumJob(metadataJSON) {
		return []stepSeed{
			{action: "v3_intent_parse", sortIndex: 5},
			{action: "v3_capability_audit", sortIndex: 10},
			{action: "v3_workspace_research", sortIndex: 20},
			{action: "v3_memory_retrieval", sortIndex: 30},
			{action: "v3_external_research", sortIndex: 35},
			{action: "v3_planning", sortIndex: 40},
			{action: "v3_analysis", sortIndex: 80},
			{action: "v3_response_draft", sortIndex: 90},
			{action: "v3_verification", sortIndex: 100},
			{action: "v3_memory_review", sortIndex: 110},
			{action: "v3_finalize", sortIndex: 120},
		}, nil
	}
	if normalizePipeline(pipeline) == model.PipelineCoding {
		return stepsForPipeline(model.PipelineCoding), nil
	}
	if usesV3NativeSteps(metadataJSON) || strings.EqualFold(strings.TrimSpace(pipeline), "v3") {
		authorityDirectives, err := v3AuthorityDirectivesFromMetadata(metadataJSON)
		if err != nil {
			return nil, err
		}
		if chat.IsLowSignal(instruction, pipeline) && len(authorityDirectives) == 0 {
			return []stepSeed{{action: "v3_chat_fastpath", sortIndex: 1}}, nil
		}
		return []stepSeed{
			{action: "v3_intent_parse", sortIndex: 5},
			{action: "v3_capability_audit", sortIndex: 10},
			{action: "v3_workspace_research", sortIndex: 20},
			{action: "v3_memory_retrieval", sortIndex: 30},
			{action: "v3_external_research", sortIndex: 35},
			{action: "v3_planning", sortIndex: 40},
			{action: "v3_analysis", sortIndex: 80},
			{action: "v3_response_draft", sortIndex: 90},
			{action: "v3_verification", sortIndex: 100},
			{action: "v3_memory_review", sortIndex: 110},
			{action: "v3_finalize", sortIndex: 120},
		}, nil
	}
	return stepsForPipeline(pipeline), nil
}

func (r *Repository) WriteArtifact(ctx context.Context, artifact artifacts.Envelope) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, artifact.JobID, artifact.StepID, artifact.Kind, artifact.Version, string(artifact.Payload))
	return err
}

func (r *Repository) LatestArtifact(ctx context.Context, jobID int64, kind string) (artifacts.Envelope, bool, error) {
	kind = strings.TrimSpace(kind)
	if jobID <= 0 || kind == "" {
		return artifacts.Envelope{}, false, nil
	}
	var env artifacts.Envelope
	var raw []byte
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT id, job_id, step_id, kind, version, payload_json, created_at
		FROM artifacts
		WHERE job_id = $1 AND kind = $2
		ORDER BY id DESC
		LIMIT 1
	`, jobID, kind).Scan(&id, &env.JobID, &env.StepID, &env.Kind, &env.Version, &raw, &env.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return artifacts.Envelope{}, false, nil
		}
		return artifacts.Envelope{}, false, err
	}
	env.ID = fmt.Sprintf("%d", id)
	env.Payload = append([]byte(nil), raw...)
	return env, true, nil
}

func (r *Repository) ListArtifactsByJob(ctx context.Context, jobID int64, limit int) ([]artifacts.Envelope, error) {
	if jobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, step_id, kind, version, payload_json, created_at
		FROM artifacts
		WHERE job_id = $1
		ORDER BY id ASC
		LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]artifacts.Envelope, 0, limit)
	for rows.Next() {
		var item artifacts.Envelope
		var raw []byte
		var id int64
		if err := rows.Scan(&id, &item.JobID, &item.StepID, &item.Kind, &item.Version, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = fmt.Sprintf("%d", id)
		item.Payload = append([]byte(nil), raw...)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) WriteEvidence(ctx context.Context, record evidence.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, source_type, source_ref, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, record.JobID, record.StepID, record.Kind, record.SourceType, record.SourceRef, string(payload))
	return err
}

func (r *Repository) ListEvidenceByJob(ctx context.Context, jobID int64, limit int) ([]evidence.Record, error) {
	if jobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, payload_json
		FROM evidence
		WHERE job_id = $1
		ORDER BY id ASC
		LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]evidence.Record, 0, min(limit, 32))
	for rows.Next() {
		var raw []byte
		var id int64
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var item evidence.Record
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		item.ID = id
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListClaimsByJob(ctx context.Context, jobID int64, limit int) ([]model.ClaimRecord, error) {
	if jobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, step_id, text, normalized_text, status, confidence, created_at
		FROM claims
		WHERE job_id = $1
		ORDER BY id ASC
		LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ClaimRecord, 0, limit)
	for rows.Next() {
		var item model.ClaimRecord
		if err := rows.Scan(&item.ID, &item.JobID, &item.StepID, &item.Text, &item.NormalizedText, &item.Status, &item.Confidence, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
