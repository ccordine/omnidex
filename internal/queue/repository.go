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
	pipeline = normalizePipeline(pipeline)
	if len(metadataJSON) == 0 {
		metadataJSON = []byte(`{}`)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

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

	steps := stepsForJob(pipeline, instruction, metadataJSON)
	for _, step := range steps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status)
			VALUES ($1, $2, $3, $4)
		`, job.ID, step.action, step.sortIndex, model.StepStatusPending); err != nil {
			return model.Job{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func resolveProjectID(ctx context.Context, tx pgx.Tx, metadataJSON []byte) (*int64, error) {
	location := projectLocationFromMetadata(metadataJSON)
	if location == "" {
		return nil, nil
	}
	name := projectNameFromLocation(location)

	var projectID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO projects (location, name, last_seen_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (location) DO UPDATE
		SET last_seen_at = NOW(),
		    updated_at = NOW()
		RETURNING id
	`, location, name).Scan(&projectID)
	if err != nil {
		return nil, err
	}
	return &projectID, nil
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
		return text[:max] + "...[redacted]"
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

func stepsForJob(pipeline, instruction string, metadataJSON []byte) []stepSeed {
	if datasource.IsExploreJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineDataExplore {
		return []stepSeed{{action: "data_source_explore", sortIndex: 1}}
	}
	if projectdebugger.IsJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineProjectDebugger {
		return []stepSeed{{action: "project_debugger", sortIndex: 1}}
	}
	if scrumcardllm.IsJobMetadata(metadataJSON) || normalizePipeline(pipeline) == model.PipelineScrumCardLLM {
		return []stepSeed{{action: "scrum_card_llm", sortIndex: 1}}
	}
	if isDataSourceQueryJob(metadataJSON) || normalizePipeline(pipeline) == model.PipelineDataQuery {
		return []stepSeed{{action: "data_source_query", sortIndex: 1}}
	}
	if agentconfig.FromJobMetadata(metadataJSON).IsExternal() {
		return []stepSeed{{action: "external_agent_execute", sortIndex: 1}}
	}
	if scrum.IsScrumJob(metadataJSON) {
		return []stepSeed{
			{action: "v3_intent_parse", sortIndex: 5},
			{action: "v3_workspace_research", sortIndex: 20},
			{action: "v3_planning", sortIndex: 40},
			{action: "v3_response_draft", sortIndex: 90},
			{action: "v3_finalize", sortIndex: 120},
		}
	}
	if normalizePipeline(pipeline) == model.PipelineCoding {
		return stepsForPipeline(model.PipelineCoding)
	}
	if usesV3NativeSteps(metadataJSON) || strings.EqualFold(strings.TrimSpace(pipeline), "v3") {
		if chat.IsLowSignal(instruction, pipeline) {
			return []stepSeed{{action: "v3_chat_fastpath", sortIndex: 1}}
		}
		return []stepSeed{
			{action: "v3_intent_parse", sortIndex: 5},
			{action: "v3_capability_audit", sortIndex: 10},
			{action: "v3_workspace_research", sortIndex: 20},
			{action: "v3_memory_retrieval", sortIndex: 30},
			{action: "v3_planning", sortIndex: 40},
			{action: "v3_external_research", sortIndex: 50},
			{action: "v3_analysis", sortIndex: 80},
			{action: "v3_response_draft", sortIndex: 90},
			{action: "v3_verification", sortIndex: 100},
			{action: "v3_memory_review", sortIndex: 110},
			{action: "v3_finalize", sortIndex: 120},
		}
	}
	return stepsForPipeline(pipeline)
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

func (r *Repository) ListEvidenceByJob(ctx context.Context, jobID int64) ([]evidence.Record, error) {
	if jobID <= 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, payload_json
		FROM evidence
		WHERE job_id = $1
		ORDER BY id ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]evidence.Record, 0, 8)
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

func (r *Repository) ListClaimSupportByJob(ctx context.Context, jobID int64, limit int) ([]model.ClaimSupportDetail, error) {
	if jobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			cs.id,
			cs.claim_id,
			c.text,
			c.status,
			cs.evidence_id,
			COALESCE(e.kind, ''),
			COALESCE(e.source_ref, ''),
			cs.support_score,
			COALESCE(cs.rationale, ''),
			cs.created_at
		FROM claim_support cs
		JOIN claims c ON c.id = cs.claim_id
		LEFT JOIN evidence e ON e.id = cs.evidence_id
		WHERE c.job_id = $1
		ORDER BY cs.id ASC
		LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ClaimSupportDetail, 0, limit)
	for rows.Next() {
		var item model.ClaimSupportDetail
		if err := rows.Scan(&item.ID, &item.ClaimID, &item.ClaimText, &item.ClaimStatus, &item.EvidenceID, &item.EvidenceKind, &item.EvidenceSourceRef, &item.SupportScore, &item.Rationale, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) WriteMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) (int64, error) {
	if strings.TrimSpace(candidate.CandidateKind) == "" || strings.TrimSpace(candidate.Content) == "" {
		return 0, errors.New("memory candidate kind and content are required")
	}
	provenance := strings.TrimSpace(string(candidate.Provenance))
	if provenance == "" {
		provenance = `{}`
	}
	var id int64
	err := r.pool.QueryRow(ctx, `
        INSERT INTO memory_candidates (job_id, source_memory_id, candidate_kind, content, provenance, confidence, status)
        VALUES ($1, $2, $3, $4, $5::jsonb, $6, COALESCE(NULLIF($7, ''), 'candidate'))
        RETURNING id
    `, candidate.JobID, candidate.SourceMemoryID, candidate.CandidateKind, candidate.Content, provenance, candidate.Confidence, candidate.Status).Scan(&id)
	return id, err
}

func (r *Repository) ListMemoryCandidates(ctx context.Context, jobID int64, status string, limit int) ([]model.MemoryCandidate, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
        SELECT id, job_id, source_memory_id, candidate_kind, content, provenance, confidence, status, created_at, updated_at
        FROM memory_candidates
        WHERE ($1 = 0 OR job_id = $1)
          AND ($2 = '' OR status = $2)
        ORDER BY confidence DESC, id ASC
        LIMIT $3
    `, jobID, strings.TrimSpace(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.MemoryCandidate, 0, limit)
	for rows.Next() {
		var item model.MemoryCandidate
		if err := rows.Scan(&item.ID, &item.JobID, &item.SourceMemoryID, &item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetMemoryCandidate(ctx context.Context, id int64) (model.MemoryCandidate, error) {
	if id <= 0 {
		return model.MemoryCandidate{}, pgx.ErrNoRows
	}
	var item model.MemoryCandidate
	err := r.pool.QueryRow(ctx, `
        SELECT id, job_id, source_memory_id, candidate_kind, content, provenance, confidence, status, created_at, updated_at
        FROM memory_candidates
        WHERE id = $1
    `, id).Scan(&item.ID, &item.JobID, &item.SourceMemoryID, &item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *Repository) GetJobInspection(ctx context.Context, jobID int64, limit int) (model.JobInspection, error) {
	if jobID <= 0 {
		return model.JobInspection{}, nil
	}
	details, err := r.GetJobDetails(ctx, jobID)
	if err != nil {
		return model.JobInspection{}, err
	}
	artifactsList, err := r.ListArtifactsByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	evidenceList, err := r.ListEvidenceByJob(ctx, jobID)
	if err != nil {
		return model.JobInspection{}, err
	}
	claims, err := r.ListClaimsByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	support, err := r.ListClaimSupportByJob(ctx, jobID, limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	memoryCandidates, err := r.ListMemoryCandidates(ctx, jobID, "", limit)
	if err != nil {
		return model.JobInspection{}, err
	}
	return model.JobInspection{
		Job:              details.Job,
		JobID:            jobID,
		Artifacts:        artifactsList,
		Evidence:         evidenceList,
		Claims:           claims,
		ClaimSupport:     support,
		MemoryCandidates: memoryCandidates,
	}, nil
}

func (r *Repository) UpdateMemoryCandidateStatus(ctx context.Context, id int64, status string) error {
	if id <= 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
        UPDATE memory_candidates
        SET status = $2, updated_at = NOW()
        WHERE id = $1
    `, id, strings.TrimSpace(status))
	return err
}

func (r *Repository) DeleteMemoryChunk(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid memory id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_chunks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteMemoryCandidate(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid candidate id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_candidates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) MindStats(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	queries := map[string]string{
		"memory_chunks":      `SELECT COUNT(*) FROM memory_chunks`,
		"memory_candidates":  `SELECT COUNT(*) FROM memory_candidates`,
		"candidate_pending":  `SELECT COUNT(*) FROM memory_candidates WHERE status = 'candidate'`,
		"jobs":               `SELECT COUNT(*) FROM jobs`,
		"telemetry_events":   `SELECT COUNT(*) FROM omni_run_events`,
	}
	for key, query := range queries {
		var count int64
		if err := r.pool.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, nil
}

func (r *Repository) CountStepsByAction(ctx context.Context, jobID int64, action string) (int, error) {
	if jobID <= 0 || strings.TrimSpace(action) == "" {
		return 0, nil
	}
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND action = $2
	`, jobID, strings.TrimSpace(action)).Scan(&count)
	return count, err
}

func (r *Repository) ExpandDelegatedSubtasks(ctx context.Context, jobID int64, anchorStepID int64, subtasks []artifacts.Subtask) ([]model.Step, error) {
	if jobID <= 0 || anchorStepID <= 0 || len(subtasks) == 0 {
		return nil, nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var anchorSort int
	if err := tx.QueryRow(ctx, `SELECT sort_index FROM job_steps WHERE id = $1 AND job_id = $2 FOR UPDATE`, anchorStepID, jobID).Scan(&anchorSort); err != nil {
		return nil, err
	}
	spacing := 5
	shift := len(subtasks) * spacing
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET sort_index = sort_index + $3, updated_at = NOW()
		WHERE job_id = $1 AND sort_index > $2
	`, jobID, anchorSort, shift); err != nil {
		return nil, err
	}
	created := make([]model.Step, 0, len(subtasks))
	for idx, subtask := range subtasks {
		sortIndex := anchorSort + ((idx + 1) * spacing)
		row := tx.QueryRow(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id, job_id, action, sort_index, status, worker_id, output, error, started_at, finished_at, created_at, updated_at
		`, jobID, "v3_subtask", sortIndex, model.StepStatusPending)
		step, err := scanStep(row)
		if err != nil {
			return nil, err
		}
		contexts := map[string]string{
			"subtask_id":        strings.TrimSpace(subtask.ID),
			"subtask_kind":      strings.TrimSpace(subtask.Kind),
			"subtask_objective": strings.TrimSpace(subtask.Objective),
			"subtask_inputs":    strings.Join(subtask.Inputs, ", "),
			"subtask_outputs":   strings.Join(subtask.Outputs, ", "),
			"subtask_success":   strings.Join(subtask.SuccessCriteria, " | "),
		}
		for key, value := range contexts {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO step_contexts (step_id, key, value)
				VALUES ($1, $2, $3)
			`, step.ID, key, value); err != nil {
				return nil, err
			}
		}
		created = append(created, step)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *Repository) WriteClaims(ctx context.Context, claims []model.ClaimRecord) ([]model.ClaimRecord, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	saved := make([]model.ClaimRecord, 0, len(claims))
	for _, claim := range claims {
		var created model.ClaimRecord
		err := r.pool.QueryRow(ctx, `
            INSERT INTO claims (job_id, step_id, text, normalized_text, status, confidence)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id, created_at
        `, claim.JobID, claim.StepID, claim.Text, claim.NormalizedText, claim.Status, claim.Confidence).Scan(&created.ID, &created.CreatedAt)
		if err != nil {
			return nil, err
		}
		claim.ID = created.ID
		claim.CreatedAt = created.CreatedAt
		saved = append(saved, claim)
	}
	return saved, nil
}

func (r *Repository) WriteClaimSupports(ctx context.Context, supports []model.ClaimSupportRecord) error {
	if len(supports) == 0 {
		return nil
	}
	for _, support := range supports {
		if support.ClaimID <= 0 || support.EvidenceID <= 0 {
			continue
		}
		if _, err := r.pool.Exec(ctx, `
            INSERT INTO claim_support (claim_id, evidence_id, support_score, rationale)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (claim_id, evidence_id) DO UPDATE
            SET support_score = EXCLUDED.support_score,
                rationale = EXCLUDED.rationale
        `, support.ClaimID, support.EvidenceID, support.SupportScore, support.Rationale); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListJobs(ctx context.Context, status string, limit, offset int) ([]model.Job, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	query := `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
	`

	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]model.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *Repository) GetJobDetails(ctx context.Context, jobID int64) (model.JobDetails, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.JobDetails{}, err
	}

	stepsRows, err := r.pool.Query(ctx, `
		SELECT id, job_id, action, sort_index, status, worker_id, output, error, started_at, finished_at, created_at, updated_at
		FROM job_steps
		WHERE job_id = $1
		ORDER BY sort_index ASC, id ASC
	`, jobID)
	if err != nil {
		return model.JobDetails{}, err
	}
	defer stepsRows.Close()

	steps := []model.Step{}
	for stepsRows.Next() {
		step, err := scanStep(stepsRows)
		if err != nil {
			return model.JobDetails{}, err
		}
		steps = append(steps, step)
	}
	if err := stepsRows.Err(); err != nil {
		return model.JobDetails{}, err
	}

	ctxRows, err := r.pool.Query(ctx, `
		SELECT c.id, c.step_id, c.key, c.value, c.created_at
		FROM step_contexts c
		JOIN job_steps s ON s.id = c.step_id
		WHERE s.job_id = $1
		ORDER BY c.id ASC
	`, jobID)
	if err != nil {
		return model.JobDetails{}, err
	}
	defer ctxRows.Close()

	contexts := []model.StepContext{}
	for ctxRows.Next() {
		ctxValue, err := scanStepContext(ctxRows)
		if err != nil {
			return model.JobDetails{}, err
		}
		contexts = append(contexts, ctxValue)
	}
	if err := ctxRows.Err(); err != nil {
		return model.JobDetails{}, err
	}

	return model.JobDetails{Job: job, Steps: steps, Contexts: contexts}, nil
}

func (r *Repository) JobProjectID(ctx context.Context, jobID int64) (int64, error) {
	if jobID <= 0 {
		return 0, nil
	}
	var projectID *int64
	err := r.pool.QueryRow(ctx, `SELECT project_id FROM jobs WHERE id = $1`, jobID).Scan(&projectID)
	if err != nil {
		return 0, err
	}
	if projectID == nil || *projectID <= 0 {
		return 0, nil
	}
	return *projectID, nil
}

func (r *Repository) JobIDForStep(ctx context.Context, stepID int64) (int64, error) {
	if stepID <= 0 {
		return 0, nil
	}
	var jobID int64
	err := r.pool.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID)
	return jobID, err
}

func (r *Repository) ListRecentSessionJobs(ctx context.Context, pipeline, sessionID string, beforeJobID int64, limit int) ([]model.Job, error) {
	pipeline = normalizePipeline(pipeline)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || beforeJobID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 6
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE pipeline = $1
		  AND COALESCE(metadata->>'session_id', '') = $2
		  AND id < $3
		ORDER BY id DESC
		LIMIT $4
	`, pipeline, sessionID, beforeJobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]model.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(jobs)-1; i < j; i, j = i+1, j-1 {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	}
	return jobs, nil
}

func (r *Repository) GetStepRuntimeState(ctx context.Context, jobID, stepID int64) (string, string, error) {
	var jobStatus string
	var stepStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT j.status, s.status
		FROM jobs j
		JOIN job_steps s ON s.job_id = j.id
		WHERE j.id = $1 AND s.id = $2
	`, jobID, stepID).Scan(&jobStatus, &stepStatus)
	if err != nil {
		return "", "", err
	}
	return jobStatus, stepStatus, nil
}

func (r *Repository) AddStepContext(ctx context.Context, stepID int64, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("step context key is required")
	}
	value = SanitizeUTF8Text(value)
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, key, value); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE job_steps
		SET updated_at = NOW()
		WHERE id = $1
	`, stepID); err != nil {
		return err
	}
	return nil
}

func (r *Repository) ClaimNextStep(ctx context.Context, workerID string) (*model.ClaimedStep, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT
			s.id, s.job_id, s.action, s.sort_index, s.status, s.worker_id, s.output, s.error,
			s.started_at, s.finished_at, s.created_at, s.updated_at,
			j.id, j.instruction, j.pipeline, j.status, j.result, j.error, j.metadata, j.created_at, j.updated_at, j.completed_at
		FROM job_steps s
		JOIN jobs j ON j.id = s.job_id
		WHERE s.status = $1
		  AND j.status IN ($2, $3)
		  AND NOT EXISTS (
		      SELECT 1
		      FROM job_steps prev
		      WHERE prev.job_id = s.job_id
		        AND prev.sort_index < s.sort_index
		        AND prev.status <> $4
		  )
		ORDER BY s.sort_index ASC, s.id ASC
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1
	`, model.StepStatusPending, model.JobStatusPending, model.JobStatusRunning, model.StepStatusCompleted)

	step, job, err := scanClaim(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, worker_id = $3, started_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, step.ID, model.StepStatusRunning, workerID); err != nil {
		return nil, err
	}
	step.Status = model.StepStatusRunning
	step.WorkerID = workerID

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, job.ID, model.JobStatusRunning, model.JobStatusPending); err != nil {
		return nil, err
	}
	job.Status = model.JobStatusRunning

	if err := markTelemetryRunRunningForJob(ctx, tx, job.ID); err != nil {
		return nil, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, job.ID, "run_running", map[string]any{
		"job_id":  job.ID,
		"step_id": step.ID,
		"action":  step.Action,
	}); err != nil {
		return nil, err
	}

	ctxRows, err := tx.Query(ctx, `
		SELECT c.id, c.step_id, c.key, c.value, c.created_at
		FROM step_contexts c
		JOIN job_steps s ON s.id = c.step_id
		WHERE s.job_id = $1
		  AND (s.status = $2 OR s.id = $3)
		ORDER BY c.id ASC
	`, job.ID, model.StepStatusCompleted, step.ID)
	if err != nil {
		return nil, err
	}
	defer ctxRows.Close()

	contexts := make([]model.StepContext, 0, 8)
	for ctxRows.Next() {
		ctxValue, err := scanStepContext(ctxRows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, ctxValue)
	}
	if err := ctxRows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &model.ClaimedStep{Job: job, Step: step, Contexts: contexts}, nil
}

// AppendStepOutput appends text to a running step's output (used for live external-agent streaming).
func (r *Repository) AppendStepOutput(ctx context.Context, stepID int64, delta string) error {
	delta = SanitizeUTF8Text(delta)
	if delta == "" {
		return nil
	}
	if !strings.HasSuffix(delta, "\n") {
		delta += "\n"
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE job_steps
		SET output = COALESCE(output, '') || $2,
		    updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, stepID, delta, model.StepStatusRunning)
	return err
}

func (r *Repository) CompleteStep(ctx context.Context, stepID int64, output, contextKey, contextValue string) error {
	output = SanitizeUTF8Text(output)
	contextKey = SanitizeUTF8Text(strings.TrimSpace(contextKey))
	contextValue = SanitizeUTF8Text(contextValue)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobID int64
	if err := tx.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID); err != nil {
		return err
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusCanceled {
		return tx.Commit(ctx)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, stepID, model.StepStatusCompleted, output, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if contextKey != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, contextKey, contextValue); err != nil {
			return err
		}
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return err
	}

	if openSteps == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, output, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": output}, map[string]any{"terminal_step_id": stepID, "context_key": contextKey}); err != nil {
			return err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID}); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET updated_at = NOW()
			WHERE id = $1 AND status <> $2
		`, jobID, model.JobStatusCanceled); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) FailStep(ctx context.Context, stepID int64, errMsg string) error {
	errMsg = SanitizeUTF8Text(errMsg)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobID int64
	if err := tx.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID); err != nil {
		return err
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusCanceled {
		return tx.Commit(ctx)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, stepID, model.StepStatusFailed, errMsg, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5, $6)
	`, jobID, model.JobStatusFailed, errMsg, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
		return err
	}
	if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusFailed, map[string]any{"job_id": jobID, "error": errMsg}, map[string]any{"failed_step_id": stepID}); err != nil {
		return err
	}
	if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_failed", map[string]any{"job_id": jobID, "step_id": stepID, "error": errMsg}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) PauseStepForInput(ctx context.Context, stepID int64, stepOutput string, question string, extraContexts map[string]string) error {
	stepOutput = SanitizeUTF8Text(stepOutput)
	question = SanitizeUTF8Text(question)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobID int64
	if err := tx.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID); err != nil {
		return err
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusCanceled {
		return tx.Commit(ctx)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, updated_at = NOW()
		WHERE id = $1 AND status = $4
	`, stepID, model.StepStatusWaiting, stepOutput, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if strings.TrimSpace(question) != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, "input_question", strings.TrimSpace(question)); err != nil {
			return err
		}
	}

	for key, value := range extraContexts {
		k := SanitizeUTF8Text(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, k, SanitizeUTF8Text(value)); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, updated_at = NOW(), error = NULL
		WHERE id = $1 AND status IN ($3, $4)
	`, jobID, model.JobStatusWaiting, model.JobStatusPending, model.JobStatusRunning); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) SubmitJobFeedback(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	feedback = SanitizeUTF8Text(strings.TrimSpace(feedback))
	if feedback == "" {
		return model.Job{}, fmt.Errorf("feedback is required")
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return model.Job{}, err
	}
	if jobStatus == model.JobStatusCanceled || jobStatus == model.JobStatusCompleted || jobStatus == model.JobStatusFailed {
		return model.Job{}, fmt.Errorf("job is already %s", jobStatus)
	}

	var stepID int64
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusWaiting).Scan(&stepID)
	if err != nil {
		return model.Job{}, err
	}

	var approvalRequiredRaw string
	approvalCtxErr := tx.QueryRow(ctx, `
		SELECT value
		FROM step_contexts
		WHERE step_id = $1 AND key = $2
		ORDER BY id DESC
		LIMIT 1
	`, stepID, "approval_required").Scan(&approvalRequiredRaw)
	if approvalCtxErr != nil && !errors.Is(approvalCtxErr, pgx.ErrNoRows) {
		return model.Job{}, approvalCtxErr
	}
	if approvalCtxErr == nil && strings.EqualFold(strings.TrimSpace(approvalRequiredRaw), "true") {
		if !isExplicitApprovalFeedback(feedback) {
			return model.Job{}, fmt.Errorf("explicit approval required: reply with APPROVE: <notes> to continue")
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, stepID, model.StepStatusCompleted, feedback); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return model.Job{}, err
	}

	if openSteps == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, feedback, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": feedback}, map[string]any{"user_feedback_step_id": stepID}); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID, "source": "user_feedback"}); err != nil {
			return model.Job{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($3, $4, $5)
		`, jobID, model.JobStatusRunning, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func (r *Repository) ReplanJob(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return model.Job{}, fmt.Errorf("feedback is required")
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.Job{}, err
	}
	if job.Status == model.JobStatusCanceled || job.Status == model.JobStatusCompleted || job.Status == model.JobStatusFailed {
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	}

	var planStepID int64
	var planSortIndex int
	if err := tx.QueryRow(ctx, `
		SELECT id, sort_index
		FROM job_steps
		WHERE job_id = $1 AND action IN ('plan', 'v3_planning')
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID).Scan(&planStepID, &planSortIndex); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, planStepID, "replan_feedback", feedback); err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, planStepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2,
		    worker_id = NULL,
		    output = NULL,
		    error = NULL,
		    started_at = NULL,
		    finished_at = NULL,
		    updated_at = NOW()
		WHERE job_id = $1
		  AND sort_index >= $3
	`, jobID, model.StepStatusPending, planSortIndex); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2,
		    result = NULL,
		    error = NULL,
		    completed_at = NULL,
		    metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{replan_feedback}', to_jsonb($3::text), true),
		    updated_at = NOW()
		WHERE id = $1
	`, jobID, model.JobStatusRunning, feedback); err != nil {
		return model.Job{}, err
	}

	row = tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func (r *Repository) InterruptJob(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	feedback = SanitizeUTF8Text(strings.TrimSpace(feedback))
	if feedback == "" {
		return model.Job{}, fmt.Errorf("feedback is required")
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return model.Job{}, err
	}
	if jobStatus == model.JobStatusCanceled || jobStatus == model.JobStatusCompleted || jobStatus == model.JobStatusFailed {
		return model.Job{}, fmt.Errorf("job is already %s", jobStatus)
	}

	stepID, stepStatus, err := findInterruptStep(ctx, tx, jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Job{}, fmt.Errorf("job has no available step for interruption")
		}
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	switch stepStatus {
	case model.StepStatusRunning:
		if _, err := tx.Exec(ctx, `
			UPDATE job_steps
			SET status = $2, worker_id = NULL, output = NULL, started_at = NULL, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status = $3
		`, stepID, model.StepStatusPending, model.StepStatusRunning); err != nil {
			return model.Job{}, err
		}
	case model.StepStatusWaiting:
		if _, err := tx.Exec(ctx, `
			UPDATE job_steps
			SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status = $4
		`, stepID, model.StepStatusCompleted, feedback, model.StepStatusWaiting); err != nil {
			return model.Job{}, err
		}
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return model.Job{}, err
	}

	if openSteps == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, feedback, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": feedback}, map[string]any{"interrupt_step_id": stepID}); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID, "source": "interrupt"}); err != nil {
			return model.Job{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($3, $4, $5)
		`, jobID, model.JobStatusRunning, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func (r *Repository) CancelJob(ctx context.Context, jobID int64, reason string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	reason = SanitizeUTF8Text(strings.TrimSpace(reason))
	if reason == "" {
		reason = "canceled by user"
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	switch job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	case model.JobStatusCanceled:
		return job, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2,
		    error = COALESCE(NULLIF(error, ''), $3),
		    finished_at = COALESCE(finished_at, NOW()),
		    updated_at = NOW()
		WHERE job_id = $1
		  AND status IN ($4, $5, $6)
	`, jobID, model.StepStatusCanceled, reason, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, model.JobStatusCanceled, reason); err != nil {
		return model.Job{}, err
	}
	if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCanceled, map[string]any{"job_id": jobID, "error": reason}, map[string]any{"cancel_reason": reason}); err != nil {
		return model.Job{}, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_cancelled", map[string]any{"job_id": jobID, "reason": reason}); err != nil {
		return model.Job{}, err
	}

	row = tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func findInterruptStep(ctx context.Context, tx pgx.Tx, jobID int64) (int64, string, error) {
	var stepID int64
	var stepStatus string
	err := tx.QueryRow(ctx, `
		SELECT id, status
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusRunning).Scan(&stepID, &stepStatus)
	if err == nil {
		return stepID, stepStatus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusWaiting).Scan(&stepID, &stepStatus)
	if err == nil {
		return stepID, stepStatus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index DESC, id DESC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusCompleted).Scan(&stepID, &stepStatus)
	if err == nil {
		return stepID, stepStatus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusPending).Scan(&stepID, &stepStatus)
	if err == nil {
		return stepID, stepStatus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	return 0, "", pgx.ErrNoRows
}

func isExplicitApprovalFeedback(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "approve") ||
		strings.HasPrefix(normalized, "approved") ||
		strings.HasPrefix(normalized, "yes, proceed") ||
		strings.HasPrefix(normalized, "yes proceed") ||
		strings.Contains(normalized, " i approve")
}

func (r *Repository) AddMemoryChunk(ctx context.Context, source, kind, content string, tags []string, embedding []float64) (model.MemoryChunk, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.MemoryChunk{}, err
	}
	defer tx.Rollback(ctx)

	if source == "" {
		source = "manual"
	}
	kind = normalizeMemoryKind(kind)
	content = strings.TrimSpace(content)
	if content == "" {
		return model.MemoryChunk{}, fmt.Errorf("memory content is required")
	}

	var chunk model.MemoryChunk
	err = tx.QueryRow(ctx, `
		SELECT id, source, kind, content, created_at
		FROM memory_chunks
		WHERE kind = $1 AND content = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, kind, content).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.MemoryChunk{}, err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		if memoryKindAllowsSemanticCorrection(kind) && len(embedding) > 0 {
			var existingID int64
			var distance float64
			correctionErr := tx.QueryRow(ctx, `
				SELECT id, COALESCE(embedding <=> $2::vector, 10.0) AS distance
				FROM memory_chunks
				WHERE kind = $1
				  AND embedding IS NOT NULL
				ORDER BY embedding <=> $2::vector ASC
				LIMIT 1
			`, kind, vectorLiteral(embedding)).Scan(&existingID, &distance)
			if correctionErr != nil && !errors.Is(correctionErr, pgx.ErrNoRows) {
				return model.MemoryChunk{}, correctionErr
			}

			if correctionErr == nil && distance <= inferredMemoryCorrectionDistance {
				err = tx.QueryRow(ctx, `
					UPDATE memory_chunks
					SET source = $2, content = $3, embedding = $4::vector
					WHERE id = $1
					RETURNING id, source, kind, content, created_at
				`, existingID, source, content, vectorLiteral(embedding)).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
				if err != nil {
					return model.MemoryChunk{}, err
				}
			}
		}

		if chunk.ID == 0 {
			if len(embedding) > 0 {
				err = tx.QueryRow(ctx, `
				INSERT INTO memory_chunks (source, kind, content, embedding)
			VALUES ($1, $2, $3, $4::vector)
			RETURNING id, source, kind, content, created_at
		`, source, kind, content, vectorLiteral(embedding)).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
			} else {
				err = tx.QueryRow(ctx, `
			INSERT INTO memory_chunks (source, kind, content)
			VALUES ($1, $2, $3)
			RETURNING id, source, kind, content, created_at
		`, source, kind, content).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
			}
			if err != nil {
				return model.MemoryChunk{}, err
			}
		}
	}

	categories := inferMemoryCategories(kind, content, tags)
	cleaned := decorateMemoryTags(source, appendCleanTags(tags, memoryCategoryTags(categories)...))
	for _, tag := range cleaned {
		var tagID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tags(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tag).Scan(&tagID)
		if err != nil {
			return model.MemoryChunk{}, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_tags (memory_chunk_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, tag_id) DO NOTHING
		`, chunk.ID, tagID); err != nil {
			return model.MemoryChunk{}, err
		}
	}

	for _, category := range categories {
		var categoryID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO memory_categories(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, category).Scan(&categoryID)
		if err != nil {
			return model.MemoryChunk{}, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_categories (memory_chunk_id, category_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, category_id) DO NOTHING
		`, chunk.ID, categoryID); err != nil {
			return model.MemoryChunk{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.MemoryChunk{}, err
	}

	return chunk, nil
}

func memoryKindAllowsSemanticCorrection(kind string) bool {
	switch normalizeMemoryKind(kind) {
	case model.MemoryKindEpisodic, model.MemoryKindReference:
		return false
	default:
		return true
	}
}

func (r *Repository) FindRelevantMemory(ctx context.Context, embedding []float64, tags []string, limit int) ([]model.MemoryMatch, error) {
	if limit <= 0 {
		limit = 8
	}
	tags = cleanTags(tags)
	categoryFilters := memoryCategoryFilters(tags)
	trustOrder := fmt.Sprintf(`
				COALESCE(MAX(CASE WHEN t.name = '%s' THEN 1 ELSE 0 END), 0) DESC,
				COALESCE(MAX(CASE WHEN t.name = '%s' THEN 1 ELSE 0 END), 0) DESC,
				CASE
					WHEN mc.source = 'manual' THEN 0
					WHEN mc.source LIKE 'job:%%:reviewed:durable' THEN 1
					WHEN mc.source LIKE 'job:%%:reviewed:approved' THEN 2
					WHEN mc.source LIKE 'job:%%:inferred:approved' THEN 3
					ELSE 4
				END ASC,`, model.MemoryTrustTagDurable, model.MemoryTrustTagApproved)

	var rows pgx.Rows
	var err error

	if len(embedding) > 0 {
		query := fmt.Sprintf(`
			SELECT
				mc.id,
				mc.kind,
				mc.content,
				mc.created_at,
				COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
				COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories,
				COALESCE(1 - (mc.embedding <=> $1::vector), 0) AS score
			FROM memory_chunks mc
			LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
			LEFT JOIN tags t ON t.id = mct.tag_id
			LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id = mc.id
			LEFT JOIN memory_categories c ON c.id = mcc.category_id
			WHERE (
				$2::text[] IS NULL
				OR cardinality($2::text[]) = 0
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_tags fmct
					JOIN tags ft ON ft.id = fmct.tag_id
					WHERE fmct.memory_chunk_id = mc.id
					  AND ft.name = ANY($2)
				)
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_categories fmcc
					JOIN memory_categories fc ON fc.id = fmcc.category_id
					WHERE fmcc.memory_chunk_id = mc.id
					  AND fc.name = ANY($4)
				)
			)
			GROUP BY mc.id
			ORDER BY
%s
				CASE mc.kind
					WHEN 'instruction' THEN 0
					WHEN 'procedural' THEN 1
					WHEN 'reference' THEN 2
					WHEN 'preference' THEN 3
					ELSE 4
				END ASC,
				CASE WHEN mc.embedding IS NULL THEN 1 ELSE 0 END,
				mc.embedding <=> $1::vector ASC,
				mc.created_at DESC,
				mc.id DESC
			LIMIT $3
		`, trustOrder)
		rows, err = r.pool.Query(ctx, query, vectorLiteral(embedding), tags, limit, categoryFilters)
	} else {
		query := fmt.Sprintf(`
			SELECT
				mc.id,
				mc.kind,
				mc.content,
				mc.created_at,
				COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
				COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories,
				0.0 AS score
			FROM memory_chunks mc
			LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
			LEFT JOIN tags t ON t.id = mct.tag_id
			LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id = mc.id
			LEFT JOIN memory_categories c ON c.id = mcc.category_id
			WHERE (
				$1::text[] IS NULL
				OR cardinality($1::text[]) = 0
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_tags fmct
					JOIN tags ft ON ft.id = fmct.tag_id
					WHERE fmct.memory_chunk_id = mc.id
					  AND ft.name = ANY($1)
				)
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_categories fmcc
					JOIN memory_categories fc ON fc.id = fmcc.category_id
					WHERE fmcc.memory_chunk_id = mc.id
					  AND fc.name = ANY($3)
				)
			)
			GROUP BY mc.id
			ORDER BY
%s
				CASE mc.kind
					WHEN 'instruction' THEN 0
					WHEN 'procedural' THEN 1
					WHEN 'reference' THEN 2
					WHEN 'preference' THEN 3
					ELSE 4
				END ASC
				,
				mc.created_at DESC,
				mc.id DESC
			LIMIT $2
		`, trustOrder)
		rows, err = r.pool.Query(ctx, query, tags, limit, categoryFilters)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := make([]model.MemoryMatch, 0, limit)
	for rows.Next() {
		var m model.MemoryMatch
		if err := rows.Scan(&m.ID, &m.Kind, &m.Content, &m.CreatedAt, &m.Tags, &m.Categories, &m.Score); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

func (r *Repository) ListMemoryCategories(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.name, COUNT(mcc.memory_chunk_id)::bigint AS count
		FROM memory_categories c
		JOIN memory_chunk_categories mcc ON mcc.category_id = c.id
		GROUP BY c.id, c.name
		ORDER BY count DESC, c.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func (r *Repository) ListMemoryTags(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.name, COUNT(mct.memory_chunk_id)::bigint AS count
		FROM tags t
		JOIN memory_chunk_tags mct ON mct.tag_id = t.id
		GROUP BY t.id, t.name
		ORDER BY count DESC, t.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func (r *Repository) BackfillMemoryCategories(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT
			mc.id,
			mc.kind,
			mc.content,
			COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags
		FROM memory_chunks mc
		LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
		LEFT JOIN tags t ON t.id = mct.tag_id
		WHERE NOT EXISTS (
			SELECT 1
			FROM memory_chunk_categories existing
			WHERE existing.memory_chunk_id = mc.id
		)
		GROUP BY mc.id
		ORDER BY mc.id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pendingMemoryCategory struct {
		id         int64
		categories []string
	}
	pending := []pendingMemoryCategory{}
	for rows.Next() {
		var id int64
		var kind, content string
		var tags []string
		if err := rows.Scan(&id, &kind, &content, &tags); err != nil {
			return err
		}
		pending = append(pending, pendingMemoryCategory{id: id, categories: inferMemoryCategories(kind, content, tags)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, item := range pending {
		for _, category := range item.categories {
			var categoryID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO memory_categories(name) VALUES ($1)
				ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
				RETURNING id
			`, category).Scan(&categoryID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO memory_chunk_categories (memory_chunk_id, category_id)
				VALUES ($1, $2)
				ON CONFLICT(memory_chunk_id, category_id) DO NOTHING
			`, item.id, categoryID); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func scanMemoryFacets(rows pgx.Rows) ([]model.MemoryFacet, error) {
	facets := []model.MemoryFacet{}
	for rows.Next() {
		var facet model.MemoryFacet
		if err := rows.Scan(&facet.Name, &facet.Count); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return facets, nil
}

func (r *Repository) CreateChannel(ctx context.Context, channel model.Channel) (model.Channel, error) {
	channel.ID = normalizeChannelID(channel.ID, channel.Name)
	if channel.ID == "" {
		return model.Channel{}, fmt.Errorf("channel id is required")
	}
	channel.Persona = normalizeChannelPersona(channel.Persona)
	channel.Tags = cleanTags(channel.Tags)
	if len(channel.Context) == 0 || !json.Valid(channel.Context) {
		channel.Context = json.RawMessage(`{}`)
	}
	var out model.Channel
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_channels (id, name, persona, system, provider, model, context, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			persona = EXCLUDED.persona,
			system = EXCLUDED.system,
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			context = EXCLUDED.context,
			tags = EXCLUDED.tags,
			updated_at = NOW()
		RETURNING id, name, persona, system, provider, model, context, tags, created_at, updated_at
	`, channel.ID, strings.TrimSpace(channel.Name), channel.Persona, strings.TrimSpace(channel.System), strings.TrimSpace(channel.Provider), strings.TrimSpace(channel.Model), string(channel.Context), channel.Tags).Scan(
		&out.ID,
		&out.Name,
		&out.Persona,
		&out.System,
		&out.Provider,
		&out.Model,
		&out.Context,
		&out.Tags,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return model.Channel{}, err
	}
	return out, nil
}

func (r *Repository) GetChannel(ctx context.Context, id string) (model.Channel, error) {
	var out model.Channel
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, persona, system, provider, model, context, tags, created_at, updated_at
		FROM ai_channels
		WHERE id = $1
	`, strings.TrimSpace(id)).Scan(
		&out.ID,
		&out.Name,
		&out.Persona,
		&out.System,
		&out.Provider,
		&out.Model,
		&out.Context,
		&out.Tags,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return model.Channel{}, err
	}
	return out, nil
}

func (r *Repository) ListChannels(ctx context.Context, limit, offset int) ([]model.Channel, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, persona, system, provider, model, context, tags, created_at, updated_at
		FROM ai_channels
		ORDER BY updated_at DESC, id ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []model.Channel{}
	for rows.Next() {
		var ch model.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Persona, &ch.System, &ch.Provider, &ch.Model, &ch.Context, &ch.Tags, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *Repository) AddChannelMessage(ctx context.Context, channelID, role, content string) (model.ChannelMessage, error) {
	role = normalizeChannelMessageRole(role)
	content = SanitizeUTF8Text(strings.TrimSpace(content))
	if strings.TrimSpace(channelID) == "" {
		return model.ChannelMessage{}, fmt.Errorf("channel id is required")
	}
	if content == "" {
		return model.ChannelMessage{}, fmt.Errorf("message content is required")
	}
	var msg model.ChannelMessage
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_channel_messages (channel_id, role, content)
		VALUES ($1, $2, $3)
		RETURNING id, channel_id, role, content, created_at
	`, channelID, role, content).Scan(&msg.ID, &msg.ChannelID, &msg.Role, &msg.Content, &msg.CreatedAt)
	if err != nil {
		return model.ChannelMessage{}, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE ai_channels SET updated_at = NOW() WHERE id = $1`, channelID)
	return msg, nil
}

func (r *Repository) ListChannelMessages(ctx context.Context, channelID string, limit int) ([]model.ChannelMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM (
			SELECT id, channel_id, role, content, created_at
			FROM ai_channel_messages
			WHERE channel_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		) recent
		ORDER BY created_at ASC, id ASC
	`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.ChannelMessage{}
	for rows.Next() {
		var msg model.ChannelMessage
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func normalizeChannelID(id, fallback string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = strings.TrimSpace(fallback)
	}
	id = strings.ToLower(id)
	id = channelIDSanitizer.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-_.:")
	if len(id) > 96 {
		id = strings.Trim(id[:96], "-_.:")
	}
	return id
}

func normalizeChannelPersona(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "instruct", "assistant", "chat", "":
		return "assistant"
	case "roleplay", "rp":
		return "roleplay"
	case "narrate", "story":
		return "narrate"
	default:
		return strings.ToLower(strings.TrimSpace(persona))
	}
}

func normalizeChannelMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "system", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func decorateMemoryTags(source string, tags []string) []string {
	out := append([]string(nil), tags...)
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case source == "", source == "manual":
		out = append(out, model.MemoryTrustTagDurable, "provenance:user")
	case strings.Contains(source, ":reviewed:durable"):
		out = append(out, model.MemoryTrustTagDurable, "provenance:reviewed")
	case strings.Contains(source, ":reviewed:approved"):
		out = append(out, model.MemoryTrustTagApproved, "provenance:reviewed")
	case strings.Contains(source, ":inferred:approved"):
		out = append(out, model.MemoryTrustTagApproved, "provenance:inferred")
	case strings.Contains(source, ":prompt"), strings.Contains(source, ":response"):
		out = append(out, "scope:session")
	}
	return cleanTags(out)
}

func inferMemoryCategories(kind, content string, tags []string) []string {
	out := []string{}
	add := func(category string) {
		category = normalizeMemoryCategory(category)
		if category == "" {
			return
		}
		for _, existing := range out {
			if existing == category {
				return
			}
		}
		out = append(out, category)
	}

	switch normalizeMemoryKind(kind) {
	case model.MemoryKindProcedural:
		add("strategy")
	case model.MemoryKindReference:
		add("reference")
	case model.MemoryKindPreference:
		add("preference")
		add("personal")
	case model.MemoryKindInstruction:
		add("instruction")
	}

	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			add(strings.TrimPrefix(tag, "category:"))
			continue
		}
		switch {
		case strings.HasPrefix(tag, "project:"):
			add("project")
		case strings.HasPrefix(tag, "session:"), strings.HasPrefix(tag, "channel:"):
			add("personal")
		case strings.HasPrefix(tag, "provider:"):
			add("integration")
		case strings.HasPrefix(tag, "query:"), tag == "research", tag == "web_search":
			add("research")
		case tag == "success-playbook", tag == "learned-skill":
			add("strategy")
		case isLanguageMemoryMarker(tag):
			add("language")
		case isDatabaseMemoryMarker(tag):
			add("database")
		case isInfrastructureMemoryMarker(tag):
			add("infrastructure")
		case isFrontendMemoryMarker(tag):
			add("frontend")
		}
	}

	text := strings.ToLower(content)
	if containsAnyText(text, "user prefers", "i prefer", "my name", "remember that i", "personal") {
		add("personal")
	}
	if containsAnyText(text, "project", "repository", "workspace", "schema", "codebase") {
		add("project")
	}
	if containsAnyText(text, "rust", "golang", "go lang", "javascript", "typescript", "php", "python", "zig", "react", "node", "vite") {
		add("language")
	}
	if containsAnyText(text, "postgres", "postgresql", "pgsql", "sql", "table", "column", "migration") {
		add("database")
	}
	if containsAnyText(text, "docker", "container", "compose", "kubernetes", "ollama", "gpu", "vulkan") {
		add("infrastructure")
	}
	if containsAnyText(text, "react", "vite", "frontend", "ui", "component", "css") {
		add("frontend")
	}
	if containsAnyText(text, "api", "endpoint", "http", "openai", "anthropic", "google", "hugging face") {
		add("integration")
	}
	if containsAnyText(text, "test", "verified", "verification", "build passed", "go test", "cargo test", "npm test") {
		add("verification")
	}
	if containsAnyText(text, "error", "failed", "blocker", "recovery", "fix", "troubleshoot") {
		add("troubleshooting")
	}
	if len(out) == 0 {
		add("general")
	}
	return out
}

func memoryCategoryTags(categories []string) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		if normalized := normalizeMemoryCategory(category); normalized != "" {
			out = append(out, "category:"+normalized)
		}
	}
	return out
}

func memoryCategoryFilters(tags []string) []string {
	out := []string{}
	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			if category := normalizeMemoryCategory(strings.TrimPrefix(tag, "category:")); category != "" {
				out = appendCleanTags(out, category)
			}
		}
	}
	return out
}

func normalizeMemoryCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.TrimPrefix(category, "category:")
	category = strings.ReplaceAll(category, "_", "-")
	category = strings.ReplaceAll(category, " ", "-")
	switch category {
	case "personal", "person", "user":
		return "personal"
	case "project", "codebase", "workspace", "repo", "repository":
		return "project"
	case "language", "languages", "programming-language":
		return "language"
	case "database", "db", "sql", "pgsql", "postgres", "postgresql":
		return "database"
	case "infrastructure", "infra", "docker", "container", "deployment", "devops":
		return "infrastructure"
	case "frontend", "ui", "react", "vite":
		return "frontend"
	case "integration", "api", "provider", "model-provider":
		return "integration"
	case "strategy", "procedural", "playbook", "skill":
		return "strategy"
	case "reference", "research", "documentation", "docs":
		return "research"
	case "preference", "instruction", "verification", "troubleshooting", "security", "general":
		return category
	default:
		if category == "" || len(category) > 40 {
			return ""
		}
		return category
	}
}

func appendCleanTags(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := map[string]struct{}{}
	for _, existing := range cleanTags(out) {
		seen[existing] = struct{}{}
	}
	for _, value := range cleanTags(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isLanguageMemoryMarker(tag string) bool {
	switch tag {
	case "go", "golang", "rust", "javascript", "typescript", "php", "python", "zig", "node", "nodejs", "react", "vite":
		return true
	default:
		return false
	}
}

func isDatabaseMemoryMarker(tag string) bool {
	switch tag {
	case "postgres", "postgresql", "pgsql", "sql", "database", "db", "schema", "migration":
		return true
	default:
		return false
	}
}

func isInfrastructureMemoryMarker(tag string) bool {
	switch tag {
	case "docker", "compose", "container", "containers", "kubernetes", "ollama", "vulkan", "gpu", "deployment":
		return true
	default:
		return false
	}
}

func isFrontendMemoryMarker(tag string) bool {
	switch tag {
	case "react", "vite", "frontend", "ui", "css", "component", "components":
		return true
	default:
		return false
	}
}

func normalizePipeline(pipeline string) string {
	switch strings.ToLower(strings.TrimSpace(pipeline)) {
	case model.PipelineAssistant:
		return model.PipelineAssistant
	case model.PipelineChat:
		return model.PipelineChat
	case model.PipelineCoding:
		return model.PipelineCoding
	case model.PipelineStory:
		return model.PipelineStory
	case model.PipelineDataQuery:
		return model.PipelineDataQuery
	case model.PipelineDataExplore:
		return model.PipelineDataExplore
	default:
		return model.PipelineAssistant
	}
}

func isDataSourceQueryJob(metadataJSON []byte) bool {
	if len(metadataJSON) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromMetadata(payload["source"])) == "omni-data-source"
}

func stringFromMetadata(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func stepsForPipeline(pipeline string) []stepSeed {
	switch normalizePipeline(pipeline) {
	case model.PipelineCoding:
		return []stepSeed{
			{action: "coding_workflow", sortIndex: 5},
		}
	case model.PipelineChat:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "roleplay", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	case model.PipelineDataQuery:
		return []stepSeed{
			{action: "data_source_query", sortIndex: 1},
		}
	case model.PipelineDataExplore:
		return []stepSeed{
			{action: "data_source_explore", sortIndex: 1},
		}
	case model.PipelineStory:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "narrate", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	default:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "assist", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	}
}

func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func normalizeMemoryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.MemoryKindProcedural:
		return model.MemoryKindProcedural
	case model.MemoryKindInstruction:
		return model.MemoryKindInstruction
	case model.MemoryKindPreference:
		return model.MemoryKindPreference
	case model.MemoryKindReference:
		return model.MemoryKindReference
	default:
		return model.MemoryKindEpisodic
	}
}

func vectorLiteral(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%f", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func scanJob(row pgx.Row) (model.Job, error) {
	var job model.Job
	var result, errText *string
	if err := row.Scan(
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
	); err != nil {
		return model.Job{}, err
	}
	job.Result = stringOrEmpty(result)
	job.Error = stringOrEmpty(errText)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return job, nil
}

func scanStep(row pgx.Row) (model.Step, error) {
	var step model.Step
	var workerID, output, errText *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&workerID,
		&output,
		&errText,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
	); err != nil {
		return model.Step{}, err
	}
	step.WorkerID = stringOrEmpty(workerID)
	step.Output = stringOrEmpty(output)
	step.Error = stringOrEmpty(errText)
	return step, nil
}

func scanStepContext(row pgx.Row) (model.StepContext, error) {
	var ctxValue model.StepContext
	if err := row.Scan(
		&ctxValue.ID,
		&ctxValue.StepID,
		&ctxValue.Key,
		&ctxValue.Value,
		&ctxValue.CreatedAt,
	); err != nil {
		return model.StepContext{}, err
	}
	return ctxValue, nil
}

func scanClaim(row pgx.Row) (model.Step, model.Job, error) {
	var step model.Step
	var job model.Job
	var stepWorker, stepOutput, stepError *string
	var jobResult, jobError *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&stepWorker,
		&stepOutput,
		&stepError,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&jobResult,
		&jobError,
		&job.Metadata,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	); err != nil {
		return model.Step{}, model.Job{}, err
	}
	step.WorkerID = stringOrEmpty(stepWorker)
	step.Output = stringOrEmpty(stepOutput)
	step.Error = stringOrEmpty(stepError)
	job.Result = stringOrEmpty(jobResult)
	job.Error = stringOrEmpty(jobError)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return step, job, nil
}
