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
	guidanceModel string,
	executorModel string,
	runtime exactTypeScriptConvergenceRuntime,
) (result ExactTypeScriptConvergence, returnErr error) {
	started := time.Now()
	result = ExactTypeScriptConvergence{
		SourceOpeningID: point.Call.ID, SourceGapOpeningID: point.Gap.ID,
		GuidanceModel: strings.TrimSpace(guidanceModel),
		ExecutorModel: strings.TrimSpace(executorModel),
		Terminal:      ExactTypeScriptConvergenceFailed,
	}
	defer func() { result.WallDuration = time.Since(started) }()
	if ctx == nil || runtime.verify == nil || runtime.guide == nil || runtime.execute == nil ||
		result.GuidanceModel == "" || result.ExecutorModel == "" {
		return result, fmt.Errorf(
			"TypeScript convergence replay requires context, compiler, guidance provider, executor provider, and both models",
		)
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
	if strings.TrimSpace(input.RepairGuidance) != "" {
		return result, fmt.Errorf("TypeScript convergence source opening must contain compiler authority, not prior repair guidance")
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
	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if currentDiagnostic.RepairRegion == nil {
			return result, fmt.Errorf(
				"TypeScript convergence diagnostic before iteration %d lacks one exact repair region",
				iteration,
			)
		}
		capabilities := append([]string(nil), input.Capabilities...)
		permittedSymbols := append([]string(nil), input.PermittedSymbols...)
		if directCodingTypeScriptRepairRegionHasExactIncompatibility(currentDiagnostic.RepairRegion) {
			capabilities = nil
			permittedSymbols = nil
		}
		guidanceJob, err := assemblyline.NewTypeScriptRepairGuidanceJob(
			assemblyline.TypeScriptRepairGuidanceInput{
				Language: "typescript", Signature: input.Signature,
				Capabilities:     capabilities,
				PermittedSymbols: permittedSymbols,
				RepairRegion:     currentDiagnostic.RepairRegion,
				Diagnostic:       currentDiagnostic.ModelFeedback,
			},
		)
		if err != nil {
			return result, fmt.Errorf("derive TypeScript repair guidance %d: %w", iteration, err)
		}
		entry := ExactTypeScriptConvergenceIteration{Number: iteration}
		guidanceReplay, guidanceErr := runtime.guide(ctx, guidanceJob, iteration)
		entry.GuidanceReplay = guidanceReplay
		var artifactErr *ExactStationReplayArtifactError
		if guidanceErr != nil {
			if errors.As(guidanceErr, &artifactErr) {
				entry.GuidanceArtifactError = artifactErr.Error()
			}
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("execute TypeScript repair guidance %d: %w", iteration, guidanceErr)
		}
		instruction := strings.TrimSpace(guidanceReplay.Artifact.Source)
		if instruction == "" {
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("TypeScript repair guidance iteration %d returned no exact instruction", iteration)
		}
		if directCodingTypeScriptCompilerContainsPathIdentity(instruction) {
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf(
				"TypeScript repair guidance iteration %d contains path identity",
				iteration,
			)
		}
		entry.Instruction = instruction
		executionJob, err := assemblyline.NewFragmentCorrectionJob(
			assemblyline.FragmentCorrectionInput{
				Language: "typescript", Signature: input.Signature,
				RepairRegion:   currentDiagnostic.RepairRegion,
				RepairGuidance: instruction,
			},
		)
		if err != nil {
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("derive guided TypeScript correction %d: %w", iteration, err)
		}
		executionReplay, executionErr := runtime.execute(ctx, executionJob, iteration)
		entry.ExecutionReplay = executionReplay
		artifactErr = nil
		if executionErr != nil {
			if errors.As(executionErr, &artifactErr) {
				entry.ExecutionArtifactError = artifactErr.Error()
			}
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("execute guided TypeScript correction %d: %w", iteration, executionErr)
		}
		replacement := executionReplay.Artifact.Source
		if strings.TrimSpace(replacement) == "" {
			result.Iterations = append(result.Iterations, entry)
			return result, fmt.Errorf("TypeScript convergence iteration %d returned no exact artifact", iteration)
		}
		candidate := replacement
		if currentDiagnostic.RepairRegion != nil {
			candidate, err = assemblyline.ApplyTypeScriptFragmentRepairRegion(
				current, *currentDiagnostic.RepairRegion, replacement,
			)
			if err != nil {
				result.Iterations = append(result.Iterations, entry)
				return result, fmt.Errorf("apply TypeScript convergence repair region %d: %w", iteration, err)
			}
		}
		if candidate == current {
			entry.AfterDiagnostic = currentDiagnostic
			result.Iterations = append(result.Iterations, entry)
			result.Terminal = ExactTypeScriptConvergenceStalled
			return result, fmt.Errorf(
				"guided TypeScript executor made no source change at iteration %d",
				iteration,
			)
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
			result.Iterations = append(result.Iterations, entry)
			result.Terminal = ExactTypeScriptConvergenceStalled
			return result, fmt.Errorf(
				"guided TypeScript repair made no verified compiler progress at iteration %d: assessment=%s repeated=%t",
				iteration, delta.Assessment, repeatsAccepted,
			)
		}
		result.Iterations = append(result.Iterations, entry)
		current = candidate
		currentDiagnostic = diagnostic
		result.FinalSource, result.FinalSourceSHA256 = current, replaySHA256(current)
		seenAccepted[state] = struct{}{}
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
