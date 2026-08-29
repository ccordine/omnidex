package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/jackc/pgx/v5"
)

// EnqueueScrumJob is the only public boundary that may derive the Scrum
// pipeline. Callers provide typed, code-owned metadata rather than pipeline or
// source strings.
func (r *Repository) EnqueueScrumJob(
	ctx context.Context,
	instruction string,
	metadata scrum.JobMetadata,
) (model.Job, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return model.Job{}, fmt.Errorf("PostgreSQL and context are required to enqueue Scrum work")
	}
	if err := metadata.Validate(); err != nil {
		return model.Job{}, err
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return model.Job{}, fmt.Errorf("encode typed Scrum job metadata: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- commit closes the transaction.
	job, err := r.enqueueScrumJobTx(ctx, tx, instruction, metadataJSON)
	if err != nil {
		return model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func (r *Repository) enqueueScrumJobTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction string,
	metadataJSON []byte,
) (model.Job, error) {
	if _, err := scrum.DecodeJobMetadata(metadataJSON); err != nil {
		return model.Job{}, err
	}
	return r.enqueueJobWithStepsTx(
		ctx, tx, instruction, model.PipelineScrum, metadataJSON,
		[]stepSeed{{action: "v3_coding", sortIndex: 5}},
	)
}
