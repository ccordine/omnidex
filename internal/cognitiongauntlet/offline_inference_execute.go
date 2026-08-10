package cognitiongauntlet

import (
	"context"
	"fmt"
	"math"
	"os"
	"syscall"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

func runControlledPublicFullCognition(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
	control inferenceProcessControl,
) (PublicFullCognitionRunResult, error) {
	if err := control.Validate(); err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	run, prior, err := resumeRuntimePrefix(control, publicSHA, episode, request)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	execution, err := preparePublicFullCognition(ctx, bundle, request)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	if prior != nil {
		if err := verifyResumedInferenceBoundary(
			ctx, control, publicSHA, execution, &run, *prior,
		); err != nil {
			return PublicFullCognitionRunResult{}, err
		}
	}
	paused := false
	for run.Cycles < uint32(bundle.Authority.Budget.RuntimeCycles) {
		if control.Mode == inferenceRecordResumeBaseline {
			if err := recordResumeBaselineCheckpoint(ctx, control, execution, run.PolicyCalls); err != nil {
				return PublicFullCognitionRunResult{}, err
			}
		}
		if !paused && control.Mode == inferenceStopBeforeNextCall {
			paused, err = stopAtInferenceBoundary(
				ctx, control, publicSHA, execution, &run,
			)
			if err != nil {
				return PublicFullCognitionRunResult{}, err
			}
		}
		step, stepErr := execution.components.runtime.Step(ctx, execution.binding)
		accumulateRuntimeStep(&run, step)
		if stepErr != nil {
			return cancelAndFinishPublicCognition(
				ctx, bundle, request, execution, run,
				fmt.Errorf("execute controlled public cognition cycle %d: %w", run.Cycles, stepErr),
			)
		}
		if step.State == cognitionruntime.StepEpisodeCompleted ||
			step.State == cognitionruntime.StepEpisodeFailed {
			run.Terminal = step
			return finishPublicFullCognition(ctx, bundle, request, execution, run)
		}
	}
	return cancelAndFinishPublicCognition(
		ctx, bundle, request, execution, run,
		fmt.Errorf("%w: controlled public cognition exhausted %d frozen cycles",
			cognitionruntime.ErrRunCycleLimit, bundle.Authority.Budget.RuntimeCycles),
	)
}

func resumeRuntimePrefix(
	control inferenceProcessControl,
	publicSHA string,
	episode cognition.EpisodeRef,
	request PublicFullCognitionRunRequest,
) (cognitionruntime.RunResult, *PausedInferenceCheckpoint, error) {
	if control.ResumeCheckpointPath == "" {
		return cognitionruntime.RunResult{}, nil, nil
	}
	prior, err := LoadPausedInferenceCheckpoint(control.ResumeCheckpointPath)
	if err != nil {
		return cognitionruntime.RunResult{}, nil, err
	}
	if err := episode.Validate(); err != nil {
		return cognitionruntime.RunResult{}, nil, fmt.Errorf("resume cognition episode authority is invalid")
	}
	actor := prior.PreCall.Bound.Attempt
	if prior.PublicRunAuthoritySHA256 != publicSHA ||
		prior.Episode != episode ||
		actor.JobID != request.Attempt.JobID || actor.Generation != request.Attempt.Generation ||
		actor.StepID != request.Attempt.StepID || request.Attempt.Attempt != int64(actor.Attempt)+1 ||
		actor.WorkerID == request.Attempt.WorkerID ||
		prior.Boundary != control.ResumeBoundary {
		return cognitionruntime.RunResult{}, nil, fmt.Errorf("replacement inference changed its sealed prefix authority")
	}
	return runResultFromPrefix(prior.Prefix), &prior, nil
}

func verifyResumedInferenceBoundary(
	ctx context.Context,
	control inferenceProcessControl,
	publicSHA string,
	execution publicFullCognitionExecution,
	run *cognitionruntime.RunResult,
	prior PausedInferenceCheckpoint,
) error {
	current, err := execution.components.repository.CognitionEpisode(ctx, execution.episode.ID)
	if err != nil {
		return err
	}
	if current.SuccessfulActions < 0 || current.SuccessfulActions > math.MaxUint32 {
		return fmt.Errorf("replacement inference durable action count is invalid")
	}
	if control.ResumeBoundary.Kind == inferenceBoundaryActions &&
		current.SuccessfulActions != int64(control.ResumeBoundary.Count) {
		return fmt.Errorf("replacement inference changed the durable resume boundary")
	}
	checkpoint, err := pauseInferenceCheckpoint(
		ctx, control.ResumeVerificationPath, publicSHA, execution, run,
		uint32(current.SuccessfulActions), control.ResumeBoundary,
	)
	if err != nil {
		return err
	}
	if checkpoint.Prefix != prior.Prefix ||
		checkpoint.SuccessfulActions != prior.SuccessfulActions {
		return fmt.Errorf("replacement inference changed its sealed runtime prefix")
	}
	if _, err := NewTakeoverContinuityProof(prior.PreCall, checkpoint.PreCall); err != nil {
		return err
	}
	return stopAndReauthorizeInference(ctx, execution)
}

func stopAtInferenceBoundary(
	ctx context.Context,
	control inferenceProcessControl,
	publicSHA string,
	execution publicFullCognitionExecution,
	run *cognitionruntime.RunResult,
) (bool, error) {
	episode, err := execution.components.repository.CognitionEpisode(ctx, execution.episode.ID)
	if err != nil {
		return false, err
	}
	if episode.SuccessfulActions < 0 || episode.SuccessfulActions > math.MaxUint32 {
		return false, fmt.Errorf("durable cognition action count exceeds the process control boundary")
	}
	actions := uint32(episode.SuccessfulActions)
	current := actions
	if control.StopBoundary.Kind == inferenceBoundaryDecisions {
		current = run.PolicyCalls
	}
	if current < control.StopBoundary.Count {
		return false, nil
	}
	if current != control.StopBoundary.Count {
		return false, fmt.Errorf("inference process crossed its registered checkpoint boundary")
	}
	_, err = pauseInferenceCheckpoint(
		ctx, control.CheckpointPath, publicSHA, execution, run, actions, control.StopBoundary,
	)
	if err != nil {
		return false, err
	}
	return true, stopAndReauthorizeInference(ctx, execution)
}

func pauseInferenceCheckpoint(
	ctx context.Context,
	path string,
	publicSHA string,
	execution publicFullCognitionExecution,
	run *cognitionruntime.RunResult,
	actions uint32,
	boundary inferenceBoundary,
) (PausedInferenceCheckpoint, error) {
	preCall, err := CaptureSemanticPreCallCheckpoint(
		ctx, execution.components.repository, execution.episode.ID, episodeAttemptAuthority(execution),
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, err
	}
	checkpoint, err := NewPausedInferenceCheckpoint(
		publicSHA, execution.episode, preCall, *run, actions, boundary,
	)
	if err != nil {
		return PausedInferenceCheckpoint{}, err
	}
	if err := SealPausedInferenceCheckpoint(path, checkpoint); err != nil {
		return PausedInferenceCheckpoint{}, err
	}
	return checkpoint, nil
}

func stopAndReauthorizeInference(
	ctx context.Context,
	execution publicFullCognitionExecution,
) error {
	if err := syscall.Kill(os.Getpid(), syscall.SIGSTOP); err != nil {
		return fmt.Errorf("stop cognition inference at registered boundary: %w", err)
	}
	if err := execution.components.store.AuthorizeAttempt(ctx, execution.binding.Attempt); err != nil {
		return fmt.Errorf("reauthorize resumed cognition inference: %w", err)
	}
	return nil
}

func episodeAttemptAuthority(execution publicFullCognitionExecution) model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: execution.binding.Attempt.JobID, Generation: execution.binding.Attempt.Generation,
		StepID: execution.binding.Attempt.StepID, Attempt: int64(execution.binding.Attempt.Attempt),
		WorkerID: execution.binding.Attempt.WorkerID,
	}
}

func accumulateRuntimeStep(run *cognitionruntime.RunResult, step cognitionruntime.StepResult) {
	run.Cycles++
	if step.PolicyCalled {
		run.PolicyCalls++
	}
	if step.RecoveredDecision {
		run.RecoveredDecisions++
	}
	if step.RecoveredAction {
		run.RecoveredActions++
	}
	if step.RecoveredProgress {
		run.RecoveredProgress++
	}
	if step.RecoveredPolicyOutcome {
		run.RecoveredPolicyOutcomes++
	}
	run.AbandonedPolicyCalls += step.AbandonedPolicyCalls
	run.EnvironmentActions += step.EnvironmentActions
}
