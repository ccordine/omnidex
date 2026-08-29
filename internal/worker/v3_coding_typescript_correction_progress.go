package worker

import (
	"fmt"
	"strings"
)

// Each correction requires a distinct compiler-proven diagnostic. This is a
// stage bound over different verified failures, not a model-response retry.
const maxDirectCodingTypeScriptStageCorrections = 3

type directCodingTypeScriptCorrectionState struct {
	blockID    string
	diagnostic string
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
	verificationStage string,
	diagnostic string,
) error {
	if progress == nil || progress.seen == nil {
		return fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	state := directCodingTypeScriptCorrectionState{
		blockID:    strings.TrimSpace(blockID),
		diagnostic: strings.TrimSpace(diagnostic),
	}
	if state.blockID == "" || strings.TrimSpace(verificationStage) == "" || state.diagnostic == "" {
		return fmt.Errorf(
			"TypeScript correction progress requires one block, verification stage, and exact diagnostic",
		)
	}
	if _, repeated := progress.seen[state]; repeated {
		return fmt.Errorf(
			"repeated compiler diagnostic rejected for block %s; no distinct verified failure authorizes another repair call: %s",
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
