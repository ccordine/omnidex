package worker

import (
	"fmt"
	"strings"
)

const maxDirectCodingTypeScriptStageCorrections = maxTypedWorkerAttempts

type directCodingTypeScriptCorrectionState struct {
	blockID           string
	candidate         string
	verificationStage string
	diagnostic        string
}

type directCodingTypeScriptCorrectionProgress struct {
	seen             map[directCodingTypeScriptCorrectionState]struct{}
	stageCorrections int
}

func newDirectCodingTypeScriptCorrectionProgress() *directCodingTypeScriptCorrectionProgress {
	return &directCodingTypeScriptCorrectionProgress{
		seen: make(map[directCodingTypeScriptCorrectionState]struct{}),
	}
}

func (progress *directCodingTypeScriptCorrectionProgress) beginStage() error {
	if progress == nil || progress.seen == nil {
		return fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	progress.stageCorrections = 0
	return nil
}

func (progress *directCodingTypeScriptCorrectionProgress) observe(
	blockID string,
	candidate string,
	verificationStage string,
	diagnostic string,
) error {
	if progress == nil || progress.seen == nil {
		return fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	state := directCodingTypeScriptCorrectionState{
		blockID:           strings.TrimSpace(blockID),
		candidate:         strings.TrimSpace(candidate),
		verificationStage: strings.TrimSpace(verificationStage),
		diagnostic:        strings.TrimSpace(diagnostic),
	}
	if state.blockID == "" || state.verificationStage == "" || state.diagnostic == "" {
		return fmt.Errorf(
			"TypeScript correction progress requires one block, verification stage, and exact diagnostic",
		)
	}
	if _, repeated := progress.seen[state]; repeated {
		return fmt.Errorf(
			"repeated candidate/diagnostic correction state rejected for block %s: %s",
			state.blockID,
			safeLine(firstDirectCodingDiagnosticLine(state.diagnostic), "unknown"),
		)
	}
	if progress.stageCorrections >= maxDirectCodingTypeScriptStageCorrections {
		return fmt.Errorf(
			"TypeScript staged correction exhausted the %d-correction code-owned limit for block %s",
			maxDirectCodingTypeScriptStageCorrections, state.blockID,
		)
	}
	progress.seen[state] = struct{}{}
	progress.stageCorrections++
	return nil
}
