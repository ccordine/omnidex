package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) convergeDirectCodingTypeScriptGuidedRepair(
	target assemblyline.TypeScriptBlock,
	tsx bool,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	failure string,
) (string, error) {
	guidanceModel, err := s.workerModel(station.CodingFragmentRepairGuidance)
	if err != nil {
		return "", err
	}
	correctionModel, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return "", err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	workerRuntime.CorrectionModel = correctionModel
	seenGuidance := make(map[string]struct{}, maxTypedWorkerAttempts)
	var rejectedInstruction string
	var rejectionKind assemblyline.TypeScriptRepairGuidanceRejectionKind

	for attempt := 1; attempt <= maxTypedWorkerAttempts; attempt++ {
		s.runtime.svc.emitStepEvent(
			s.runtime.claim.Authority,
			"coding_fragment_repair_guidance_started",
			fmt.Sprintf(
				"block=%s attempt=%d exact_failure=%s", target.ID, attempt,
				safeLine(trimForBudget(failure, 500), "unknown"),
			),
		)
		var guidance string
		if rejectedInstruction == "" {
			guidance, err = runDirectCodingTypeScriptRepairGuidance(
				workerRuntime, guidanceModel, target, available, current, repairRegion, failure,
			)
		} else {
			guidance, err = runDirectCodingTypeScriptRepairGuidanceAfterRejection(
				workerRuntime, guidanceModel, target, available, current, repairRegion, failure,
				rejectedInstruction, rejectionKind,
			)
		}
		if err != nil {
			return "", fmt.Errorf("derive TypeScript repair guidance: %w", err)
		}
		guidance = strings.TrimSpace(guidance)
		if _, repeated := seenGuidance[guidance]; repeated {
			rejectedInstruction = guidance
			rejectionKind = assemblyline.TypeScriptRepairGuidanceRepeatedInstruction
			s.runtime.svc.emitStepEvent(
				s.runtime.claim.Authority,
				"coding_fragment_repair_guidance_rejected",
				fmt.Sprintf("block=%s reason=repeated_instruction", target.ID),
			)
			continue
		}
		seenGuidance[guidance] = struct{}{}

		s.runtime.svc.emitStepEvent(
			s.runtime.claim.Authority,
			"coding_fragment_correction_started",
			fmt.Sprintf("block=%s guidance_bytes=%d", target.ID, len(guidance)),
		)
		source, correctionErr := runDirectCodingTypeScriptFragmentWorker(
			workerRuntime, correctionModel,
			directCodingTypeScriptFragmentJob{
				block: target, tsx: tsx, available: available, current: current,
				repairRegion: repairRegion, repairGuidance: guidance,
			},
		)
		if correctionErr == nil {
			return source, nil
		}
		if !errors.Is(correctionErr, assemblyline.ErrTypeScriptFragmentRepairNoChange) &&
			!errors.Is(correctionErr, errDirectCodingTypeScriptUnchangedCorrection) {
			return "", correctionErr
		}
		rejectedInstruction = guidance
		rejectionKind = assemblyline.TypeScriptRepairGuidanceNoSourceChange
		s.runtime.svc.emitStepEvent(
			s.runtime.claim.Authority,
			"coding_fragment_repair_guidance_rejected",
			fmt.Sprintf("block=%s reason=no_source_change", target.ID),
		)
	}

	return "", fmt.Errorf(
		"TypeScript repair guidance failed to produce a source transition after %d bounded attempts",
		maxTypedWorkerAttempts,
	)
}
