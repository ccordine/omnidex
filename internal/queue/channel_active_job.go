package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func activeChannelJobTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
) (model.Job, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata,
		       current_generation, created_at, updated_at, completed_at
		FROM jobs
		WHERE metadata->>'channel_id'=$1
		  AND status IN ('pending','running','waiting_input')
		ORDER BY id DESC
		LIMIT 2
	`, channelID)
	if err != nil {
		return model.Job{}, false, fmt.Errorf(
			"read channel %q active job: %w",
			channelID,
			err,
		)
	}
	defer rows.Close()
	jobs := make([]model.Job, 0, 2)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return model.Job{}, false, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return model.Job{}, false, err
	}
	if len(jobs) > 1 {
		return model.Job{}, false, fmt.Errorf(
			"channel %q has multiple active jobs %d and %d",
			channelID,
			jobs[0].ID,
			jobs[1].ID,
		)
	}
	if len(jobs) == 0 {
		return model.Job{}, false, nil
	}
	return jobs[0], true, nil
}

func validateChannelSessionJobAuthority(
	channelID model.ChannelID,
	authority lockedChannelTurnAuthority,
	expectedWorkspaceIdentity string,
	job model.Job,
) error {
	if job.ID < 1 || job.Pipeline != model.PipelineChat || job.CurrentGeneration < 1 {
		return fmt.Errorf("channel %q active job has invalid identity or pipeline", channelID)
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return fmt.Errorf(
			"channel %q job %d has unregistered status %q",
			channelID,
			job.ID,
			job.Status,
		)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return fmt.Errorf("decode channel %q job %d binding: %w", channelID, job.ID, err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return fmt.Errorf("channel %q job %d binding: %w", channelID, job.ID, err)
	}
	if binding.ClientWorkspaceIdentity != expectedWorkspaceIdentity {
		return fmt.Errorf(
			"%w: channel %q active job %d differs from the exact CLI workspace identity",
			ErrChannelSessionWorkspace,
			channelID,
			job.ID,
		)
	}
	if binding.ChannelID != channelID || binding.ClientCWD != authority.WorkspaceRoot ||
		binding.ChannelMode != authority.Mode ||
		binding.DataSourceID != modelDataSourceID(authority.DataSourceID) ||
		binding.RoleplayViewpointCharacterID != modelRoleplayCharacterID(authority.RoleplayViewpointID) {
		return fmt.Errorf(
			"channel %q active job %d differs from immutable channel authority",
			channelID,
			job.ID,
		)
	}
	return nil
}
