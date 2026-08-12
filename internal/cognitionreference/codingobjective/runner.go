package codingobjective

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type operations struct {
	apply func(context.Context, *changeapply.StagedChange) (omni.PatchApplyResult, error)
}

func Run(
	ctx context.Context,
	objective Objective,
	station DeclarationStation,
) (Result, error) {
	return runWithOperations(ctx, objective, station, operations{})
}

func runWithOperations(
	ctx context.Context,
	objective Objective,
	station DeclarationStation,
	configured operations,
) (result Result, resultErr error) {
	result.Steps = []Step{}
	result.ChangedFileIDs = []string{}
	result.ExpectedFiles = []ExpectedFile{}
	result.CommitOutcome = CommitNotAttempted
	if ctx == nil {
		return result, fmt.Errorf("%w: context is required", ErrInvalidObjective)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	objective.Acceptance = append([]AcceptancePredicate(nil), objective.Acceptance...)
	if err := objective.validate(); err != nil {
		return result, err
	}
	result.ObjectiveID = objective.ID
	result.Acceptance = append([]AcceptancePredicate(nil), objective.Acceptance...)
	if station == nil {
		return result, fmt.Errorf("%w: declaration station is required", ErrInvalidObjective)
	}
	repository, err := inspectRepository(ctx, objective, &result)
	if err != nil {
		return result, err
	}
	if err := verifyBaseline(ctx, repository); err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, StepBaselineUnsatisfied)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	candidate, err := generateDeclaration(ctx, station, repository, &result)
	if err != nil {
		return result, err
	}
	stage, err := stageAndVerify(ctx, repository, candidate)
	if err != nil {
		return result, err
	}
	result.Satisfied = true
	cleanupPending := true
	defer func() {
		if cleanupPending {
			resultErr = errors.Join(resultErr, stage.Cleanup())
		}
	}()
	result.StageID = stage.ID()
	result.PatchSHA256 = stage.PatchSHA256()
	result.ChangedFileIDs = stage.ChangedFileIDs()
	result.ExpectedFiles = expectedFileAuthority(stage.ExpectedFiles())
	result.Steps = append(
		result.Steps,
		StepChangeStaged, StepStageFormatVerified, StepStageTestsVerified,
	)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	apply := configured.apply
	if apply == nil {
		apply = func(ctx context.Context, stage *changeapply.StagedChange) (omni.PatchApplyResult, error) {
			return stage.ApplyVerified(ctx)
		}
	}
	result.CommitOutcome = CommitUnknown
	applyResult, err := apply(ctx, stage)
	if err != nil {
		return result, fmt.Errorf(
			"authoritative apply failed; the commit boundary may have been crossed: %w", err,
		)
	}
	if err := validateApplyResult(objective.Root, repository.snapshot, result.ChangedFileIDs, applyResult); err != nil {
		return result, fmt.Errorf("authoritative apply returned invalid commit evidence: %w", err)
	}
	if err := reconcileExpectedRepository(
		ctx, objective.Root, repository.snapshot, result.ExpectedFiles,
	); err != nil {
		return result, fmt.Errorf("authoritative commit reconciliation failed: %w", err)
	}
	result.CommitOutcome = CommitSucceeded
	result.Steps = append(result.Steps, StepAuthoritativeApplied, StepObjectiveCompleted)
	result.Complete = true
	cleanupPending = false
	if err := stage.Cleanup(); err != nil {
		return result, fmt.Errorf("objective committed and completed but stage cleanup failed: %w", err)
	}
	return result, nil
}
