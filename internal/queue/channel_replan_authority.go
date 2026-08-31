package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
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
		switch job.Pipeline {
		case model.PipelineChat:
			return nil, fmt.Errorf("replan chat job %d requires exact channel authority", job.ID)
		case model.PipelineCoding:
			return stepsForJob(job.Metadata)
		case model.PipelineScrum:
			if _, err := scrum.DecodeStoredJobMetadata(job.Metadata); err != nil {
				return nil, err
			}
			return []stepSeed{{action: "v3_coding", sortIndex: 5}}, nil
		default:
			return nil, fmt.Errorf("replan job %d has no executable pipeline %q", job.ID, job.Pipeline)
		}
	}
	var valid bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM jobs AS job_row
			JOIN ai_channels AS channel ON channel.id=$2
			JOIN ai_channel_messages AS message
			  ON message.id=$3 AND message.channel_id=channel.id
			WHERE job_row.id=$1 AND channel.scope='user'
			  AND message.role='user' AND message.content=$4
		)
	`, job.ID, binding.ChannelID, binding.UserMessageID, job.Instruction).Scan(&valid)
	if err != nil {
		return nil, fmt.Errorf("validate replan channel authority: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("replan chat job %d has stale or mismatched channel authority", job.ID)
	}
	return conversationObjectiveSteps(), nil
}
