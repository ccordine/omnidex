package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func canonicalReplanStepsTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
) ([]stepSeed, error) {
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return nil, fmt.Errorf("replan channel authority: %w", err)
	}
	if !exists {
		if normalizePipeline(job.Pipeline) == model.PipelineChat {
			return nil, fmt.Errorf("replan chat job %d requires exact channel authority", job.ID)
		}
		return stepsForJob(job.Pipeline, job.Instruction, job.Metadata)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("decode replan channel authority: %w", err)
	}
	var valid bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM jobs AS job_row
			JOIN ai_channels AS channel ON channel.id=$2
			JOIN projects AS project ON project.id=channel.project_id
			JOIN ai_channel_messages AS message
			  ON message.id=$3 AND message.channel_id=channel.id
			WHERE job_row.id=$1 AND job_row.project_id=$4
			  AND channel.scope='user' AND channel.project_id=$4
			  AND channel.workspace_root=$5 AND project.location=$5
			  AND message.role='user' AND message.content=$6
		)
	`, job.ID, binding.ChannelID, binding.UserMessageID, binding.ProjectID,
		metadata.ClientCWD, job.Instruction).Scan(&valid)
	if err != nil {
		return nil, fmt.Errorf("validate replan channel authority: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("replan chat job %d has stale or mismatched channel authority", job.ID)
	}
	return conversationObjectiveSteps(), nil
}
