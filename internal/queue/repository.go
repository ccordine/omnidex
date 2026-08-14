package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
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

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return r.pool.Ping(ctx)
}

func (r *Repository) EnsureSchema(ctx context.Context, bundle MigrationBundle) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("ensure schema requires PostgreSQL and context")
	}
	if err := bundle.validate(); err != nil {
		return err
	}
	if err := r.applyMigrationBundle(ctx, bundle); err != nil {
		return err
	}
	return nil
}

// ValidateRuntimeAuthority checks post-migration invariants that must hold
// before the production API or worker loops are allowed to start.
func (r *Repository) ValidateRuntimeAuthority(ctx context.Context) error {
	return r.validateExecutablePipelineState(ctx)
}

func (r *Repository) EnqueueJob(ctx context.Context, instruction, pipeline string, metadataJSON []byte) (model.Job, error) {
	pipeline, err := validatePublicEnqueuePipeline(pipeline)
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
	steps, err := stepsForJob(pipeline, instruction, metadataJSON)
	if err != nil {
		return model.Job{}, fmt.Errorf("resolve job execution steps: %w", err)
	}
	return r.enqueueJobWithStepsTx(ctx, tx, instruction, pipeline, metadataJSON, steps)
}

func (r *Repository) enqueueJobWithStepsTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction, pipeline string,
	metadataJSON []byte,
	steps []stepSeed,
) (model.Job, error) {
	normalizedPipeline, err := validatePipeline(pipeline)
	if err != nil {
		return model.Job{}, err
	}
	pipeline = normalizedPipeline
	if err := validateJobInstruction(instruction); err != nil {
		return model.Job{}, err
	}
	if len(steps) == 0 {
		return model.Job{}, fmt.Errorf("pipeline %q produced no executable steps", pipeline)
	}
	projectID, err := resolveProjectID(ctx, tx, metadataJSON)
	if err != nil {
		return model.Job{}, err
	}

	var job model.Job
	var result, errText *string
	metadataJSON = SanitizeUTF8Bytes(metadataJSON)
	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata, project_id)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		RETURNING id, instruction, pipeline, status, result, error, metadata,
		          current_generation, created_at, updated_at, completed_at
	`, instruction, pipeline, model.JobStatusPending, string(metadataJSON), projectID).Scan(
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&result,
		&errText,
		&job.Metadata,
		&job.CurrentGeneration,
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
	if telemetryRunID == "" {
		return model.Job{}, fmt.Errorf("create telemetry run for job %d returned an empty identity", job.ID)
	}
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
	if err := createTaskLedgerTx(ctx, tx, job.ID, telemetryRunID); err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose)
		VALUES ($1, 1, $2)
	`, job.ID, jobGenerationPurposeInitial); err != nil {
		return model.Job{}, fmt.Errorf("create initial generation for job %d: %w", job.ID, err)
	}
	if err := seedInitialTaskAuthorityTx(ctx, tx, job); err != nil {
		return model.Job{}, err
	}

	for _, step := range steps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status, generation)
			VALUES ($1, $2, $3, $4, 1)
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

func inferTelemetryTaskKind(pipeline string, metadata map[string]any) string {
	if kind := strings.TrimSpace(firstMetadataString(metadata, "research_topic")); kind != "" {
		return "research"
	}
	pipeline = strings.ToLower(strings.TrimSpace(pipeline))
	switch pipeline {
	case model.PipelineCoding:
		return "coding"
	case model.PipelineChat:
		return "chat"
	}
	return pipeline
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

func (r *Repository) CurrentArtifact(ctx context.Context, jobID int64, kind string) (artifacts.Envelope, bool, error) {
	kind = strings.TrimSpace(kind)
	if jobID <= 0 || kind == "" {
		return artifacts.Envelope{}, false, fmt.Errorf("current artifact requires a positive job ID and exact kind")
	}
	var env artifacts.Envelope
	var raw []byte
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT artifact.id, artifact.job_id, artifact.step_id, artifact.kind,
		       artifact.version, artifact.payload_json, artifact.created_at
		FROM artifacts AS artifact
		JOIN job_steps AS steps
		  ON steps.job_id=artifact.job_id AND steps.id=artifact.step_id
		WHERE artifact.job_id=$1 AND artifact.kind=$2
		  AND steps.superseded_at_generation IS NULL
		ORDER BY artifact.id DESC
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

func (r *Repository) ListCurrentEvidenceByJob(ctx context.Context, jobID int64, limit int) ([]evidence.Record, error) {
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
		SELECT evidence.id, evidence.payload_json
		FROM evidence
		JOIN job_steps AS steps
		  ON steps.job_id=evidence.job_id AND steps.id=evidence.step_id
		WHERE evidence.job_id=$1
		  AND steps.superseded_at_generation IS NULL
		ORDER BY evidence.id ASC
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
