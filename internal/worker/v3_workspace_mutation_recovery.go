package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (runtime *nativeRuntimeV3) reconcileCurrentWorkspaceMutation(
	root string,
	request directCodingRequest,
) (summary string, handled bool, resultErr error) {
	if runtime == nil || runtime.svc == nil || runtime.svc.repo == nil || runtime.claim == nil {
		return "", true, fmt.Errorf("workspace mutation recovery requires one active queue claim")
	}
	job := runtime.claim.Job
	step := runtime.claim.Step
	if job.ID <= 0 || job.CurrentGeneration <= 0 || step.Generation != job.CurrentGeneration {
		return "", true, fmt.Errorf("workspace mutation recovery requires current generation authority")
	}
	snapshot, err := runtime.svc.repo.CurrentWorkspaceMutation(
		runtime.ctx, job.ID, job.CurrentGeneration,
	)
	if err != nil || snapshot == nil {
		return "", err != nil, err
	}
	command := snapshot.Command
	if err := requireCurrentWorkspaceMutationRecoveryClaim(runtime.claim, command); err != nil {
		return "", true, err
	}
	projectID, err := runtime.svc.repo.JobProjectID(runtime.ctx, job.ID)
	if err != nil {
		return "", true, fmt.Errorf("resolve workspace mutation recovery project: %w", err)
	}
	if projectID != command.ProjectID || root != command.Plan.WorkspaceRoot {
		return "", true, fmt.Errorf("workspace mutation recovery command differs from current project root authority")
	}
	commands, err := workspaceVerificationCommandsFromPlan(command.Verification)
	if err != nil {
		return "", true, err
	}
	if snapshot.Terminal != nil {
		return runtime.recoverTerminalWorkspaceMutation(root, request, snapshot, commands)
	}
	runtime.svc.emitStepEvent(
		runtime.claim.Authority, "workspace_mutation_recovery_started",
		fmt.Sprintf("stage=%s source=%s", command.Plan.ID, command.Plan.SourceStateID),
	)
	var stage *workspacefacts.StagedMutation
	defer func() {
		if stage != nil {
			resultErr = errors.Join(resultErr, stage.Cleanup())
		}
	}()
	session := &directCodingSession{runtime: runtime, request: request, root: root}
	_, err = runtime.svc.repo.ExecuteWorkspaceMutation(
		runtime.ctx,
		runtime.claim.Authority,
		command,
		queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(ctx context.Context, exact queue.WorkspaceMutationCommand) error {
				source, captureErr := workspacefacts.Capture(ctx, exact.Plan.WorkspaceRoot)
				if captureErr != nil {
					return captureErr
				}
				stage, captureErr = workspacefacts.StageMutation(ctx, source, exact.Plan)
				if captureErr != nil {
					return captureErr
				}
				_, applyErr := stage.ApplyVerified(ctx)
				return applyErr
			},
			Verify: func(
				ctx context.Context,
				exact queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				if exact.Plan.GitSourceSnapshotID != "" {
					if err := validateRepositoryGoVerificationPlan(
						repositoryVerificationAuthoritative, commands,
					); err != nil {
						return queue.WorkspaceMutationVerificationResult{}, err
					}
					return session.verifyAppliedRepositoryWorkspaceMutation(ctx, exact, commands)
				}
				return session.verifyRecoveredPlainWorkspaceMutation(ctx, exact, commands)
			},
		},
	)
	if err != nil {
		return "", true, fmt.Errorf("reconcile durable workspace mutation: %w", err)
	}
	terminal, err := runtime.svc.repo.CurrentWorkspaceMutation(
		runtime.ctx, job.ID, job.CurrentGeneration,
	)
	if err != nil {
		return "", true, fmt.Errorf("reload recovered workspace mutation terminal: %w", err)
	}
	return runtime.recoverTerminalWorkspaceMutation(root, request, terminal, commands)
}

func requireCurrentWorkspaceMutationRecoveryClaim(
	claim *model.ClaimedStep,
	command queue.WorkspaceMutationCommand,
) error {
	if claim == nil {
		return fmt.Errorf("workspace mutation recovery requires one active queue claim")
	}
	if claim.Job.ID <= 0 || claim.Job.CurrentGeneration <= 0 ||
		claim.Step.ID <= 0 || claim.Step.WorkerID == "" ||
		claim.Authority.JobID != claim.Job.ID ||
		claim.Authority.Generation != claim.Job.CurrentGeneration ||
		claim.Authority.StepID != claim.Step.ID || claim.Authority.Attempt <= 0 ||
		claim.Authority.WorkerID != claim.Step.WorkerID ||
		claim.Step.JobID != claim.Job.ID ||
		claim.Step.Generation != claim.Job.CurrentGeneration {
		return fmt.Errorf("workspace mutation recovery active queue claim is incomplete or stale")
	}
	if command.JobID != claim.Job.ID ||
		command.Generation != claim.Job.CurrentGeneration ||
		command.StepID != claim.Step.ID {
		return fmt.Errorf("workspace mutation recovery command does not match the active queue claim")
	}
	return nil
}
