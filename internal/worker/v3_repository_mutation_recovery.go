package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (runtime *nativeRuntimeV3) reconcileCurrentRepositoryMutation(root string) error {
	if runtime == nil || runtime.svc == nil || runtime.svc.repo == nil || runtime.claim == nil {
		return fmt.Errorf("repository mutation recovery requires one active queue claim")
	}
	job := runtime.claim.Job
	step := runtime.claim.Step
	if job.ID <= 0 || job.CurrentGeneration <= 0 || step.Generation != job.CurrentGeneration {
		return fmt.Errorf("repository mutation recovery requires current generation authority")
	}
	command, err := runtime.svc.repo.UnresolvedRepositoryMutation(
		runtime.ctx, job.ID, job.CurrentGeneration,
	)
	if err != nil {
		return err
	}
	if command == nil {
		return nil
	}
	if err := requireCurrentRepositoryMutationRecoveryClaim(runtime.claim, *command); err != nil {
		return err
	}
	projectID, err := runtime.svc.repo.JobProjectID(runtime.ctx, job.ID)
	if err != nil {
		return fmt.Errorf("resolve repository mutation recovery project: %w", err)
	}
	source, err := runtime.svc.repo.RepositorySnapshot(
		runtime.ctx, projectID, command.SourceSnapshotID,
	)
	if err != nil {
		return fmt.Errorf("load repository mutation recovery source: %w", err)
	}
	runtime.svc.emitStepEvent(
		runtime.claim.Authority, "repository_mutation_recovery_started",
		fmt.Sprintf("stage=%s snapshot=%s", command.StageID, source.ID),
	)
	var applyResult omni.PatchApplyResult
	err = runtime.svc.repo.ApplyRepositoryMutation(
		runtime.ctx, runtime.claim.Authority, *command,
		exactRepositoryMutationClassifier(root, source),
		func(ctx context.Context) error {
			var applyErr error
			applyResult, applyErr = omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
				Context: ctx, Workspace: root, Patch: command.Patch,
			})
			if applyErr != nil {
				return applyErr
			}
			return validateRecoveredRepositoryPatchResult(source, *command, applyResult.Files)
		},
	)
	if err != nil {
		return fmt.Errorf("reconcile durable repository mutation: %w", err)
	}
	runtime.svc.emitStepEvent(
		runtime.claim.Authority, "repository_mutation_recovered",
		fmt.Sprintf("stage=%s snapshot=%s", command.StageID, source.ID),
	)
	return nil
}

func requireCurrentRepositoryMutationRecoveryClaim(
	claim *model.ClaimedStep,
	command queue.RepositoryMutationCommand,
) error {
	if claim == nil {
		return fmt.Errorf("repository mutation recovery requires one active queue claim")
	}
	if claim.Job.ID <= 0 || claim.Job.CurrentGeneration <= 0 ||
		claim.Step.ID <= 0 || claim.Step.WorkerID == "" ||
		claim.Authority.JobID != claim.Job.ID ||
		claim.Authority.Generation != claim.Job.CurrentGeneration ||
		claim.Authority.StepID != claim.Step.ID || claim.Authority.Attempt <= 0 ||
		claim.Authority.WorkerID != claim.Step.WorkerID ||
		claim.Step.JobID != claim.Job.ID ||
		claim.Step.Generation != claim.Job.CurrentGeneration {
		return fmt.Errorf("repository mutation recovery active queue claim is incomplete or stale")
	}
	if command.JobID != claim.Job.ID ||
		command.Generation != claim.Job.CurrentGeneration ||
		command.StepID != claim.Step.ID {
		return fmt.Errorf(
			"repository mutation recovery command does not match the active queue claim",
		)
	}
	return nil
}

func validateRecoveredRepositoryPatchResult(
	source repositoryfacts.Snapshot,
	command queue.RepositoryMutationCommand,
	files []omni.PatchFileResult,
) error {
	expected, err := repositoryMutationExpectedStates(source, command.ChangedFiles)
	if err != nil {
		return err
	}
	return validateRepositoryFileStatePatchResult(source, expected, files)
}
