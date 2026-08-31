package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) enqueueScrumJobTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction string,
	projectID int64,
	metadata scrum.JobMetadata,
) (model.Job, error) {
	if projectID <= 0 {
		return model.Job{}, fmt.Errorf("Scrum job requires one authoritative project")
	}
	if err := metadata.Validate(); err != nil {
		return model.Job{}, err
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return model.Job{}, fmt.Errorf("encode typed Scrum job metadata: %w", err)
	}
	return r.enqueueJobWithStepsTx(
		ctx, tx, instruction, model.PipelineScrum, metadataJSON,
		[]stepSeed{{action: "v3_coding", sortIndex: 5}}, &projectID,
	)
}
