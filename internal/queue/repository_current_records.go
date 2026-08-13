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

func (r *Repository) WriteArtifact(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	artifact artifacts.Envelope,
) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	if artifact.JobID != authority.JobID || artifact.StepID != authority.StepID {
		return fmt.Errorf("%w: artifact owner disagrees with step attempt", ErrStaleStepAttempt)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return err
	} else if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return fmt.Errorf("%w: artifact attempt is not running", ErrStaleStepAttempt)
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		SELECT steps.job_id, steps.id, $3, $4, $5::jsonb
		FROM job_steps AS steps
		JOIN jobs ON jobs.id=steps.job_id
		WHERE steps.job_id=$1 AND steps.id=$2
		  AND steps.status=$6
		  AND steps.generation=$7 AND steps.current_attempt=$8 AND steps.worker_id=$9
		  AND steps.superseded_at_generation IS NULL
		  AND steps.generation=jobs.current_generation
	`, artifact.JobID, artifact.StepID, artifact.Kind, artifact.Version,
		string(artifact.Payload), model.StepStatusRunning, authority.Generation,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "artifact target lost current authority", nil)
	}
	return tx.Commit(ctx)
}

func (r *Repository) WriteEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	record evidence.Record,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	if record.Kind == evidence.KindObjectiveCitation {
		return fmt.Errorf("objective citations require atomic completion evidence authority")
	}
	if record.JobID != authority.JobID || record.StepID != authority.StepID {
		return fmt.Errorf("%w: evidence owner disagrees with step attempt", ErrStaleStepAttempt)
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
	if jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return err
	} else if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return fmt.Errorf("%w: evidence attempt is not running", ErrStaleStepAttempt)
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, source_type, source_ref, payload_json)
		SELECT steps.job_id, steps.id, $3, $4, $5, $6::jsonb
		FROM job_steps AS steps
		JOIN jobs ON jobs.id=steps.job_id
		WHERE steps.job_id=$1 AND steps.id=$2
		  AND steps.status=$7
		  AND steps.generation=$8 AND steps.current_attempt=$9 AND steps.worker_id=$10
		  AND steps.superseded_at_generation IS NULL
		  AND steps.generation=jobs.current_generation
	`, record.JobID, record.StepID, record.Kind, record.SourceType, record.SourceRef,
		string(payload), model.StepStatusRunning, authority.Generation,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "evidence target lost current authority", nil)
	}
	return tx.Commit(ctx)
}
