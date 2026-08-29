package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingTypeScriptRepairModelResolver func() (string, string, error)

type directCodingTypeScriptRepairEvents struct {
	guidanceStarted   func(string)
	correctionStarted func(string)
}

func (s *directCodingSession) convergeDirectCodingTypeScriptGuidedRepair(
	target assemblyline.SourceBlock,
	tsx bool,
	dialect string,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	failure string,
) (string, error) {
	guidanceModel, correctionModel, err := s.typeScriptRepairModels()
	if err != nil {
		return "", err
	}
	workerRuntime := directCodingWorkerRuntime(s)
	return convergeDirectCodingTypeScriptGuidedRepairWithRuntime(
		workerRuntime, guidanceModel, correctionModel, s.typeScriptRepairEvents(),
		target, tsx, dialect, available, current, repairRegion, failure,
	)
}

func (s *directCodingSession) typeScriptRepairEvents() directCodingTypeScriptRepairEvents {
	return directCodingTypeScriptRepairEvents{
		guidanceStarted: func(detail string) {
			s.runtime.svc.emitStepEvent(
				s.runtime.claim.Authority, "coding_fragment_repair_guidance_started", detail,
			)
		},
		correctionStarted: func(detail string) {
			s.runtime.svc.emitStepEvent(
				s.runtime.claim.Authority, "coding_fragment_correction_started", detail,
			)
		},
	}
}

func (s *directCodingSession) typeScriptRepairModels() (string, string, error) {
	if s == nil {
		return "", "", fmt.Errorf("TypeScript repair requires one active coding session")
	}
	guidanceModel, err := s.workerModel(station.CodingFragmentRepairGuidance)
	if err != nil {
		return "", "", err
	}
	correctionModel, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return "", "", err
	}
	return guidanceModel, correctionModel, nil
}

func convergeDirectCodingTypeScriptGuidedRepairWithRuntime(
	workerRuntime typedWorkerRuntime,
	guidanceModel string,
	correctionModel string,
	events directCodingTypeScriptRepairEvents,
	target assemblyline.SourceBlock,
	tsx bool,
	dialect string,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	failure string,
) (string, error) {
	workerRuntime.MaxAttempts = 1
	events.emitGuidanceStarted(fmt.Sprintf(
		"block=%s exact_failure=%s", target.ID,
		safeLine(trimForBudget(failure, 500), "unknown"),
	))
	guidance, err := runDirectCodingTypeScriptRepairGuidance(
		workerRuntime, guidanceModel, target, dialect, available, current, repairRegion, failure,
	)
	if err != nil {
		return "", fmt.Errorf("derive TypeScript repair guidance: %w", err)
	}
	guidance = strings.TrimSpace(guidance)
	events.emitCorrectionStarted(
		fmt.Sprintf("block=%s guidance_bytes=%d", target.ID, len(guidance)),
	)
	source, err := runDirectCodingTypeScriptFragmentWorker(
		workerRuntime, correctionModel,
		directCodingTypeScriptFragmentJob{
			block: target, tsx: tsx, available: available, current: current,
			repairRegion: repairRegion, repairGuidance: guidance,
		},
	)
	if err != nil {
		return "", fmt.Errorf("execute TypeScript repair guidance: %w", err)
	}
	return source, nil
}

func (events directCodingTypeScriptRepairEvents) emitGuidanceStarted(detail string) {
	if events.guidanceStarted != nil {
		events.guidanceStarted(detail)
	}
}

func (events directCodingTypeScriptRepairEvents) emitCorrectionStarted(detail string) {
	if events.correctionStarted != nil {
		events.correctionStarted(detail)
	}
}
