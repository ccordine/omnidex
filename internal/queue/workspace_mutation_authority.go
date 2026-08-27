package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockWorkspaceMutationAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	requireCreator bool,
) error {
	if err := validateWorkspaceMutationExecutionAuthority(authority, command); err != nil {
		return err
	}
	if requireCreator && authority != command.creatorAuthority() {
		return staleStepAttemptError(authority, "new workspace mutation creator disagrees with executor", nil)
	}
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "workspace mutation executor is not running", nil)
	}
	return nil
}

func observeWorkspaceMutation(
	ctx context.Context,
	callbacks WorkspaceMutationCallbacks,
	command WorkspaceMutationCommand,
) (WorkspaceMutationObservation, error) {
	observation, err := callbacks.Observe(ctx, command)
	if err != nil {
		return "", fmt.Errorf("observe complete workspace mutation state: %w", err)
	}
	switch observation {
	case WorkspaceMutationSource, WorkspaceMutationPost, WorkspaceMutationIndeterminate:
		return observation, nil
	default:
		return "", fmt.Errorf("workspace mutation observer returned invalid state %q", observation)
	}
}
