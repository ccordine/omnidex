package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/jackc/pgx/v5"
)

func requireCodingAgentStepTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64) error {
	var metadata []byte
	var action string
	if err := tx.QueryRow(ctx, `SELECT j.metadata,s.action FROM jobs j JOIN job_steps s ON s.job_id=j.id WHERE j.id=$1 AND s.id=$2 AND s.generation=$3`, jobID, stepID, generation).Scan(&metadata, &action); err != nil {
		return err
	}
	configuration, err := codingagent.ConfigurationFromMetadata(metadata)
	if err != nil {
		return err
	}
	if (action != "v3_coding_plan" && action != "v3_coding") || configuration.Mode != codingagent.Agents {
		return fmt.Errorf("coding agent evidence requires the selected Agents planning or execution step")
	}
	return nil
}
