package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
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
	if baseline == nil || strings.TrimSpace(baseline.ModelFeedback) == "" {
		return result, fmt.Errorf("TypeScript convergence compiler baseline lacks one exact model failure")
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
		if exactTypeScriptReplayHistoricalDiagnostic(baseline.CompilerDiagnostics[index]) !=
			exactTypeScriptReplayHistoricalDiagnostic(expected) {
			return result, fmt.Errorf(
				"TypeScript convergence compiler diagnostic prefix differs from frozen authority: expected=%q actual=%q",
				expectedDiagnostics, baseline.CompilerDiagnostics,
			)
		}
	}
	result.Baseline = *baseline
	current := input.CurrentDeclaration
	currentDiagnostic := baseline
	result.FinalSource, result.FinalSourceSHA256 = current, replaySHA256(current)
	seenAccepted := map[string]struct{}{exactTypeScriptConvergenceState(current, baseline): {}}
	if baseline.RepairRegion == nil {
		return result, fmt.Errorf("TypeScript convergence compiler baseline lacks one exact repair region")
	}
	input.CurrentDeclaration = ""
	input.RepairRegion = baseline.RepairRegion
	input.Diagnostic = baseline.ModelFeedback
	job, err := assemblyline.NewFragmentCorrectionJob(input)
	if err != nil {
		return result, fmt.Errorf("derive initial current-contract TypeScript correction: %w", err)
	}
	var temperature *llm.ExactPreparedTemperature
	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		replay, replayErr := runtime.replay(ctx, job, iteration, temperature)
		var artifactErr *ExactStationReplayArtifactError
		if replayErr != nil && !errors.As(replayErr, &artifactErr) {
			return result, replayErr
		}
		entry := ExactTypeScriptConvergenceIteration{Number: iteration, Replay: replay}
		if artifactErr != nil {
			entry.ArtifactError = artifactErr.Error()
			input.CurrentDeclaration = ""
			input.Diagnostic = directCodingTypeScriptFragmentFailure(
				currentDiagnostic.ModelFeedback, artifactErr,
			)
			job, err = assemblyline.NewFragmentCorrectionJob(input)
			if err != nil {
				return result, fmt.Errorf("derive rejected-response correction %d: %w", iteration+1, err)
			}
			next, available, temperatureErr := exactTypeScriptNextExplorationTemperature(replay)
			if temperatureErr != nil {
				return result, temperatureErr
			}
			if !available {
				result.Iterations = append(result.Iterations, entry)
				result.Terminal = ExactTypeScriptConvergenceExplorationExhausted
				return result, fmt.Errorf(
					"TypeScript convergence exhausted profile temperature after rejected artifact at iteration %d",
					iteration,
				)
			}
			entry.NextTemperature = next
			result.Iterations = append(result.Iterations, entry)
			temperature = next
			continue
		}
		replacement := replay.Artifact.Source
		if strings.TrimSpace(replacement) == "" {
			return result, fmt.Errorf("TypeScript convergence iteration %d returned no exact artifact", iteration)
		}
		candidate := replacement
		if input.RepairRegion != nil {
			candidate, err = assemblyline.ApplyTypeScriptFragmentRepairRegion(
				current, *input.RepairRegion, replacement,
			)
			if err != nil {
				return result, fmt.Errorf("apply TypeScript convergence repair region %d: %w", iteration, err)
			}
		}
		if candidate == current {
			entry.AfterDiagnostic = currentDiagnostic
			next, available, temperatureErr := exactTypeScriptNextExplorationTemperature(replay)
			if temperatureErr != nil {
				return result, temperatureErr
			}
			if !available {
				result.Iterations = append(result.Iterations, entry)
				result.Terminal = ExactTypeScriptConvergenceExplorationExhausted
				return result, fmt.Errorf(
					"TypeScript convergence exhausted profile temperature on exact no-op at iteration %d",
					iteration,
				)
			}
			entry.NextTemperature = next
			result.Iterations = append(result.Iterations, entry)
			temperature = next
			continue
		}
		result.LastCandidate, result.LastCandidateSHA256 = candidate, replaySHA256(candidate)
		diagnostic, err := runtime.verify(ctx, candidate)
		if err != nil {
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("verify TypeScript convergence iteration %d: %w", iteration, err)
		}
		delta, err := exactTypeScriptDiagnosticDelta(currentDiagnostic, diagnostic)
		if err != nil {
			return result, fmt.Errorf("score TypeScript convergence iteration %d: %w", iteration, err)
		}
		entry.AfterDiagnostic = diagnostic
		entry.DiagnosticDelta = &delta
		if diagnostic == nil {
			result.Iterations = append(result.Iterations, entry)
			result.FinalSource, result.FinalSourceSHA256 = candidate, replaySHA256(candidate)
			result.Terminal = ExactTypeScriptConvergenceCompiled
			return result, nil
		}
		state := exactTypeScriptConvergenceState(candidate, diagnostic)
		_, repeatsAccepted := seenAccepted[state]
		if delta.Assessment != ExactTypeScriptConvergenceProgress || repeatsAccepted {
			next, available, temperatureErr := exactTypeScriptNextExplorationTemperature(replay)
			if temperatureErr != nil {
				return result, temperatureErr
			}
			if !available {
				result.Iterations = append(result.Iterations, entry)
				result.Terminal = ExactTypeScriptConvergenceExplorationExhausted
				return result, fmt.Errorf(
					"TypeScript convergence exhausted profile temperature without verified progress at iteration %d",
					iteration,
				)
			}
			entry.NextTemperature = next
			result.Iterations = append(result.Iterations, entry)
			temperature = next
			continue
		}
		result.Iterations = append(result.Iterations, entry)
		current = candidate
		currentDiagnostic = diagnostic
		result.FinalSource, result.FinalSourceSHA256 = current, replaySHA256(current)
		seenAccepted = map[string]struct{}{state: {}}
		temperature = nil
		if diagnostic.RepairRegion == nil {
			return result, fmt.Errorf("TypeScript convergence iteration %d lacks one exact next repair region", iteration)
		}
		input.CurrentDeclaration = ""
		input.RepairRegion = diagnostic.RepairRegion
		input.Diagnostic = diagnostic.ModelFeedback
		job, err = assemblyline.NewFragmentCorrectionJob(input)
		if err != nil {
			return result, fmt.Errorf("derive TypeScript convergence correction %d: %w", iteration+1, err)
		}
	}
}

func exactTypeScriptNextExplorationTemperature(
	replay ExactStationReplay,
) (*llm.ExactPreparedTemperature, bool, error) {
	next, available, err := llm.NextExactPreparedTemperature(
		replay.ExpectedIdentity, replay.Temperature,
	)
	if err != nil {
		return nil, false, fmt.Errorf("advance exact TypeScript convergence temperature: %w", err)
	}
	return next, available, nil
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
