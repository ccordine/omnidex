package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// JobRequiresLifecycleWorkspaceAuthority reports whether one persisted job is
// bound to an exact client-attested workspace. The mutation transaction still
// rechecks the complete binding under its job lock.
func (r *Repository) JobRequiresLifecycleWorkspaceAuthority(
	ctx context.Context,
	jobID int64,
) (bool, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID <= 0 {
		return false, fmt.Errorf(
			"lifecycle workspace authority lookup requires PostgreSQL, context, and a positive job ID",
		)
	}
	job := model.Job{ID: jobID}
	if err := r.pool.QueryRow(ctx, `
		SELECT pipeline,metadata
		FROM jobs
		WHERE id=$1
	`, jobID).Scan(&job.Pipeline, &job.Metadata); err != nil {
		if err == pgx.ErrNoRows {
			return false, err
		}
		return false, fmt.Errorf("read job %d lifecycle workspace authority: %w", jobID, err)
	}
	required, _, _, err := lifecycleJobWorkspaceRequirement(job)
	return required, err
}

// requireLifecycleWorkspaceAuthority binds every lifecycle mutation of a
// workspace-bound job to the exact immutable workspace recorded when the job
// was created. Jobs which have no workspace authority must omit both values.
func requireLifecycleWorkspaceAuthority(
	job model.Job,
	workspaceRoot string,
	workspaceIdentity string,
) error {
	required, expectedRoot, expectedIdentity, err := lifecycleJobWorkspaceRequirement(job)
	if err != nil {
		return err
	}
	if !required {
		if workspaceRoot == "" && workspaceIdentity == "" {
			return nil
		}
		return fmt.Errorf(
			"%w: job %d has no lifecycle workspace authority",
			ErrChannelSessionWorkspace,
			job.ID,
		)
	}
	if err := validateRequiredLifecycleWorkspaceBinding(
		workspaceRoot,
		workspaceIdentity,
	); err != nil {
		return err
	}
	if expectedRoot != workspaceRoot || expectedIdentity != workspaceIdentity {
		return fmt.Errorf(
			"%w: job %d differs from the exact workspace authority",
			ErrChannelSessionWorkspace,
			job.ID,
		)
	}
	return nil
}

func requireCodingPlanWorkspaceAuthority(
	job model.Job,
	workspaceRoot string,
	workspaceIdentity string,
) error {
	if err := validateRequiredLifecycleWorkspaceBinding(workspaceRoot, workspaceIdentity); err != nil {
		return err
	}
	return requireLifecycleWorkspaceAuthority(job, workspaceRoot, workspaceIdentity)
}

func lifecycleJobWorkspaceBinding(job model.Job) (string, string, error) {
	required, root, identity, err := lifecycleJobWorkspaceRequirement(job)
	if err != nil {
		return "", "", err
	}
	if !required {
		return "", "", fmt.Errorf(
			"%w: job %d pipeline %q has no workspace authority",
			ErrChannelSessionWorkspace,
			job.ID,
			job.Pipeline,
		)
	}
	return root, identity, nil
}

func lifecycleJobWorkspaceRequirement(job model.Job) (bool, string, string, error) {
	switch job.Pipeline {
	case model.PipelineChat:
		return channelJobWorkspaceRequirement(job)
	case model.PipelineCoding:
		root, identity, err := codingJobWorkspaceBinding(job)
		return true, root, identity, err
	case model.PipelineScrum:
		return false, "", "", nil
	default:
		return false, "", "", fmt.Errorf(
			"%w: job %d pipeline %q has no workspace authority",
			ErrChannelSessionWorkspace,
			job.ID,
			job.Pipeline,
		)
	}
}

func channelJobWorkspaceRequirement(job model.Job) (bool, string, string, error) {
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return false, "", "", fmt.Errorf("decode job %d lifecycle workspace authority: %w", job.ID, err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return false, "", "", fmt.Errorf("job %d lifecycle workspace authority: %w", job.ID, err)
	}
	if binding.ClientWorkspaceIdentity == "" {
		return false, "", "", nil
	}
	if binding.ChannelMode != model.ChannelModeAssistant {
		return false, "", "", fmt.Errorf(
			"%w: job %d has workspace identity outside assistant mode",
			ErrChannelSessionWorkspace,
			job.ID,
		)
	}
	return true, binding.ClientCWD, binding.ClientWorkspaceIdentity, nil
}

func codingJobWorkspaceBinding(job model.Job) (string, string, error) {
	var binding struct {
		ClientCWD               string `json:"client_cwd"`
		ClientWorkspaceIdentity string `json:"client_workspace_identity"`
	}
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return "", "", fmt.Errorf("decode job %d coding workspace authority: %w", job.ID, err)
	}
	if err := validateRequiredLifecycleWorkspaceBinding(
		binding.ClientCWD,
		binding.ClientWorkspaceIdentity,
	); err != nil {
		return "", "", fmt.Errorf("job %d coding workspace authority: %w", job.ID, err)
	}
	return binding.ClientCWD, binding.ClientWorkspaceIdentity, nil
}
