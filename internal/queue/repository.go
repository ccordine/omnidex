package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool           *pgxpool.Pool
	modelAuthority modelconfig.Authority
}

type stepSeed struct {
	action    string
	sortIndex int
}

func New(pool *pgxpool.Pool, modelAuthority modelconfig.Authority) *Repository {
	return &Repository{pool: pool, modelAuthority: modelAuthority}
}

func (r *Repository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return r.pool.Ping(ctx)
}

func (r *Repository) EnqueueCodingJob(ctx context.Context, instruction string, metadataJSON []byte) (model.Job, error) {
	if len(metadataJSON) == 0 {
		return model.Job{}, fmt.Errorf("enqueue job requires exact metadata JSON")
	}
	pipeline := model.PipelineCoding

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
	steps, err := stepsForJob(metadataJSON)
	if err != nil {
		return model.Job{}, fmt.Errorf("resolve job execution steps: %w", err)
	}
	return r.enqueueJobWithStepsTx(ctx, tx, instruction, pipeline, metadataJSON, steps, nil)
}

func (r *Repository) enqueueJobWithStepsTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction, pipeline string,
	metadataJSON []byte,
	steps []stepSeed,
	projectID *int64,
) (model.Job, error) {
	if err := validateJobInstruction(instruction); err != nil {
		return model.Job{}, err
	}
	if len(steps) == 0 {
		return model.Job{}, fmt.Errorf("pipeline %q produced no executable steps", pipeline)
	}
	var job model.Job
	var result, errText *string
	err := tx.QueryRow(ctx, `
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

	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose)
		VALUES ($1, 1, $2)
	`, job.ID, jobGenerationPurposeInitial); err != nil {
		return model.Job{}, fmt.Errorf("create initial generation for job %d: %w", job.ID, err)
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

func decodeMetadataObject(raw json.RawMessage) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return nil, fmt.Errorf("job metadata must be an exact JSON object")
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode exact job metadata object: %w", err)
	}
	if out == nil {
		return nil, fmt.Errorf("job metadata must be a JSON object")
	}
	return out, nil
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
