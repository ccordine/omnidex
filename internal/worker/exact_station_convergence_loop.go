package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func convergeExactTypeScriptStationWithRuntime(
	ctx context.Context,
	point queue.StationCallReplayPoint,
	modelName string,
	runtime exactTypeScriptConvergenceRuntime,
) (result ExactTypeScriptConvergence, returnErr error) {
	started := time.Now()
	result = ExactTypeScriptConvergence{
		SourceOpeningID: point.Call.ID, SourceGapOpeningID: point.Gap.ID,
		Model: strings.TrimSpace(modelName), Terminal: ExactTypeScriptConvergenceFailed,
	}
	defer func() { result.WallDuration = time.Since(started) }()
	if ctx == nil || runtime.verify == nil || runtime.replay == nil {
		return result, fmt.Errorf("TypeScript convergence replay requires context, compiler, and provider")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	boundary, err := validateExactStationReplayPoint(point)
	if err != nil {
		return result, err
	}
	if boundary.Job.Kind != assemblyline.WorkFragmentCorrection {
		return result, fmt.Errorf("TypeScript convergence replay requires one fragment correction opening")
	}
	var input assemblyline.FragmentCorrectionInput
	if err := json.Unmarshal(boundary.Job.Payload, &input); err != nil {
		return result, fmt.Errorf("decode TypeScript convergence correction: %w", err)
	}
	if input.Language != "typescript" || input.RepairRegion != nil {
		return result, fmt.Errorf("TypeScript convergence replay requires one whole-declaration TypeScript correction")
	}
	baseline, err := runtime.verify(ctx, input.CurrentDeclaration)
	if err != nil {
		return result, fmt.Errorf("verify TypeScript convergence baseline: %w", err)
	}
	if baseline == nil || strings.TrimSpace(baseline.ModelFeedback) != strings.TrimSpace(input.Diagnostic) {
		return result, fmt.Errorf("TypeScript convergence compiler baseline differs from frozen diagnostic authority")
	}
	expectedDiagnostics := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(input.Diagnostic), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			expectedDiagnostics = append(expectedDiagnostics, line)
		}
	}
	if len(baseline.CompilerDiagnostics) < len(expectedDiagnostics) {
		return result, fmt.Errorf(
			"TypeScript convergence compiler diagnostic prefix differs from frozen authority: expected=%q actual=%q",
			expectedDiagnostics, baseline.CompilerDiagnostics,
		)
	}
	for index, expected := range expectedDiagnostics {
		if baseline.CompilerDiagnostics[index] != expected {
			return result, fmt.Errorf(
				"TypeScript convergence compiler diagnostic prefix differs from frozen authority: expected=%q actual=%q",
				expectedDiagnostics, baseline.CompilerDiagnostics,
			)
		}
	}
	result.Baseline = *baseline
	current := input.CurrentDeclaration
	currentDiagnostic := baseline
	seen := map[string]struct{}{exactTypeScriptConvergenceState(current, baseline): {}}
	seenJobs := map[string]struct{}{boundary.Job.ID: {}}
	job := boundary.Job
	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		replay, replayErr := runtime.replay(ctx, job, iteration)
		var artifactErr *ExactStationReplayArtifactError
		if replayErr != nil && !errors.As(replayErr, &artifactErr) {
			return result, replayErr
		}
		entry := ExactTypeScriptConvergenceIteration{Number: iteration, Replay: replay}
		if artifactErr != nil {
			entry.ArtifactError = artifactErr.Error()
			result.Iterations = append(result.Iterations, entry)
			rejectedState := replaySHA256(current) + ":" + replaySHA256(replay.Generation.Content) + ":" + replaySHA256(artifactErr.Error())
			if _, repeated := seen[rejectedState]; repeated {
				result.Terminal = ExactTypeScriptConvergenceCycle
				return result, fmt.Errorf("TypeScript convergence stopped on exact rejected-response cycle at iteration %d", iteration)
			}
			seen[rejectedState] = struct{}{}
			input.CurrentDeclaration = current
			input.Diagnostic = directCodingTypeScriptFragmentFailure(
				currentDiagnostic.ModelFeedback, artifactErr,
			)
			job, err = assemblyline.NewFragmentCorrectionJob(input)
			if err != nil {
				return result, fmt.Errorf("derive rejected-response correction %d: %w", iteration+1, err)
			}
			if _, repeated := seenJobs[job.ID]; repeated {
				result.Terminal = ExactTypeScriptConvergenceCycle
				return result, fmt.Errorf("TypeScript convergence stopped on repeated correction packet at iteration %d", iteration)
			}
			seenJobs[job.ID] = struct{}{}
			continue
		}
		candidate := replay.Artifact.Source
		if strings.TrimSpace(candidate) == "" {
			return result, fmt.Errorf("TypeScript convergence iteration %d returned no exact artifact", iteration)
		}
		if candidate == current {
			entry.AfterDiagnostic = currentDiagnostic
			result.Iterations = append(result.Iterations, entry)
			result.Terminal = ExactTypeScriptConvergenceNoOp
			result.FinalSource, result.FinalSourceSHA256 = current, replaySHA256(current)
			return result, fmt.Errorf("TypeScript convergence stopped on exact no-op at iteration %d", iteration)
		}
		diagnostic, err := runtime.verify(ctx, candidate)
		if err != nil {
			return result, fmt.Errorf("verify TypeScript convergence iteration %d: %w", iteration, err)
		}
		entry.AfterDiagnostic = diagnostic
		result.Iterations = append(result.Iterations, entry)
		result.FinalSource, result.FinalSourceSHA256 = candidate, replaySHA256(candidate)
		if diagnostic == nil {
			result.Terminal = ExactTypeScriptConvergenceCompiled
			return result, nil
		}
		state := exactTypeScriptConvergenceState(candidate, diagnostic)
		if _, repeated := seen[state]; repeated {
			result.Terminal = ExactTypeScriptConvergenceCycle
			return result, fmt.Errorf("TypeScript convergence stopped on exact cycle at iteration %d", iteration)
		}
		seen[state] = struct{}{}
		current = candidate
		currentDiagnostic = diagnostic
		input.CurrentDeclaration = candidate
		input.Diagnostic = diagnostic.ModelFeedback
		job, err = assemblyline.NewFragmentCorrectionJob(input)
		if err != nil {
			return result, fmt.Errorf("derive TypeScript convergence correction %d: %w", iteration+1, err)
		}
		seenJobs[job.ID] = struct{}{}
	}
}

func exactTypeScriptConvergenceState(
	source string,
	diagnostic *ExactTypeScriptReplayDiagnostic,
) string {
	feedback := "compiled"
	if diagnostic != nil {
		feedback = diagnostic.ModelFeedbackSHA256 + ":" + diagnostic.CompilerOutputSHA256
	}
	return replaySHA256(source) + ":" + feedback
}
