package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (session *directCodingSession) executeQueuedRepositoryWorkspaceMutation(
	ctx context.Context,
	ownerID string,
	commands []testCommand,
	source repositoryfacts.Snapshot,
	prepared *verifiedRepositoryChangeStage,
) (result queue.WorkspaceMutationResult, resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil {
		return queue.WorkspaceMutationResult{}, fmt.Errorf("queued workspace mutation requires one active coding session")
	}
	stage, err := stageWorkspaceMutationFromRepositoryChange(
		ctx, source, ownerID, commands, prepared,
	)
	if err != nil {
		return queue.WorkspaceMutationResult{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, stage.Cleanup())
	}()
	command, err := workspaceMutationCommandForStage(session.runtime, commands, stage)
	if err != nil {
		return queue.WorkspaceMutationResult{}, err
	}
	result, err = session.runtime.svc.repo.ExecuteWorkspaceMutation(
		ctx,
		session.runtime.claim.Authority,
		command,
		queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(applyCtx context.Context, _ queue.WorkspaceMutationCommand) error {
				_, applyErr := stage.ApplyVerified(applyCtx)
				return applyErr
			},
			Verify: func(
				verifyCtx context.Context,
				exact queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				return session.verifyAppliedRepositoryWorkspaceMutation(
					verifyCtx, exact, commands,
				)
			},
		},
	)
	if err != nil {
		return queue.WorkspaceMutationResult{}, err
	}
	if !result.VerificationSucceeded {
		return queue.WorkspaceMutationResult{}, fmt.Errorf("workspace mutation authoritative verification failed")
	}
	return result, nil
}
