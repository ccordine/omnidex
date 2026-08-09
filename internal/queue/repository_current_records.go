package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) WriteArtifact(ctx context.Context, artifact artifacts.Envelope) error {
	if artifact.Kind == artifacts.KindIntent {
		return ErrIntentArtifactRequiresAcceptedWriter
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requireRunningCurrentStepTx(ctx, tx, artifact.JobID, artifact.StepID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		SELECT steps.job_id, steps.id, $3, $4, $5::jsonb
		FROM job_steps AS steps
		JOIN jobs ON jobs.id=steps.job_id
		WHERE steps.job_id=$1 AND steps.id=$2
		  AND steps.status=$6
		  AND steps.superseded_at_generation IS NULL
		  AND steps.generation=jobs.current_generation
	`, artifact.JobID, artifact.StepID, artifact.Kind, artifact.Version,
		string(artifact.Payload), model.StepStatusRunning)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: artifact step %d lost current authority", ErrStaleJobGeneration, artifact.StepID)
	}
	return tx.Commit(ctx)
}

func (r *Repository) WriteEvidence(ctx context.Context, record evidence.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requireRunningCurrentStepTx(ctx, tx, record.JobID, record.StepID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, source_type, source_ref, payload_json)
		SELECT steps.job_id, steps.id, $3, $4, $5, $6::jsonb
		FROM job_steps AS steps
		JOIN jobs ON jobs.id=steps.job_id
		WHERE steps.job_id=$1 AND steps.id=$2
		  AND steps.status=$7
		  AND steps.superseded_at_generation IS NULL
		  AND steps.generation=jobs.current_generation
	`, record.JobID, record.StepID, record.Kind, record.SourceType, record.SourceRef,
		string(payload), model.StepStatusRunning)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: evidence step %d lost current authority", ErrStaleJobGeneration, record.StepID)
	}
	return tx.Commit(ctx)
}
