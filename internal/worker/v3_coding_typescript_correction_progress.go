package worker

import (
	"fmt"
	"strings"
)

const (
	// Semantic correction remains tightly bounded because each accepted state
	// transition consumes one guidance call and one executor call.
	maxDirectCodingTypeScriptStageSemanticCorrections = 3
	// Deterministic closure may consume every registered, compiler-proven local
	// transition slot without reducing the separate semantic-call budget. This
	// stage-transition bound is independent of the per-receipt projection bound.
	maxDirectCodingTypeScriptStageDeterministicCorrections = 8
)

type directCodingTypeScriptCorrectionKind string

const (
	directCodingTypeScriptCorrectionDeterministic directCodingTypeScriptCorrectionKind = "deterministic"
	directCodingTypeScriptCorrectionSemantic      directCodingTypeScriptCorrectionKind = "semantic"
)

type directCodingTypeScriptCorrectionState struct {
	blockID    string
	diagnostic string
}

type directCodingTypeScriptCorrectionProgress struct {
	seen                          map[directCodingTypeScriptCorrectionState]struct{}
	stageDeterministicCorrections map[string]int
	stageSemanticCorrections      map[string]int
}

func newDirectCodingTypeScriptCorrectionProgress() *directCodingTypeScriptCorrectionProgress {
	return &directCodingTypeScriptCorrectionProgress{
		seen:                          make(map[directCodingTypeScriptCorrectionState]struct{}),
		stageDeterministicCorrections: make(map[string]int),
		stageSemanticCorrections:      make(map[string]int),
	}
}

func (progress *directCodingTypeScriptCorrectionProgress) beginStage() error {
	if progress == nil || progress.seen == nil ||
		progress.stageDeterministicCorrections == nil || progress.stageSemanticCorrections == nil {
		return fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	clear(progress.stageDeterministicCorrections)
	clear(progress.stageSemanticCorrections)
	return nil
}

func (progress *directCodingTypeScriptCorrectionProgress) observeDeterministic(
	blockID string,
	verificationStage string,
	diagnostic string,
) error {
	return progress.observe(
		directCodingTypeScriptCorrectionDeterministic,
		blockID, verificationStage, diagnostic,
	)
}

func (progress *directCodingTypeScriptCorrectionProgress) observeSemantic(
	blockID string,
	verificationStage string,
	diagnostic string,
) error {
	return progress.observe(
		directCodingTypeScriptCorrectionSemantic,
		blockID, verificationStage, diagnostic,
	)
}

func (progress *directCodingTypeScriptCorrectionProgress) observe(
	kind directCodingTypeScriptCorrectionKind,
	blockID string,
	verificationStage string,
	diagnostic string,
) error {
	if progress == nil || progress.seen == nil ||
		progress.stageDeterministicCorrections == nil || progress.stageSemanticCorrections == nil {
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
	corrections, limit := progress.stageSemanticCorrections, maxDirectCodingTypeScriptStageSemanticCorrections
	if kind == directCodingTypeScriptCorrectionDeterministic {
		corrections, limit = progress.stageDeterministicCorrections,
			maxDirectCodingTypeScriptStageDeterministicCorrections
	} else if kind != directCodingTypeScriptCorrectionSemantic {
		return fmt.Errorf("TypeScript correction progress kind %q is invalid", kind)
	}
	if corrections[state.blockID] >= limit {
		return fmt.Errorf(
			"TypeScript staged %s correction exhausted the %d-correction code-owned limit for block %s",
			kind, limit, state.blockID,
		)
	}
	progress.seen[state] = struct{}{}
	corrections[state.blockID]++
	return nil
}
