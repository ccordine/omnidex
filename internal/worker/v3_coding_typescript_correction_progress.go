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

type directCodingTypeScriptPrimitiveNormalization struct {
	startByte   int
	replacement string
}

type directCodingTypeScriptCorrectionProgress struct {
	seen                          map[directCodingTypeScriptCorrectionState]struct{}
	stageDeterministicCorrections map[string]int
	stageSemanticCorrections      map[string]int
	stagePrimitiveNormalizations  map[string]map[directCodingTypeScriptPrimitiveNormalization]struct{}
}

func newDirectCodingTypeScriptCorrectionProgress() *directCodingTypeScriptCorrectionProgress {
	return &directCodingTypeScriptCorrectionProgress{
		seen:                          make(map[directCodingTypeScriptCorrectionState]struct{}),
		stageDeterministicCorrections: make(map[string]int),
		stageSemanticCorrections:      make(map[string]int),
		stagePrimitiveNormalizations: make(
			map[string]map[directCodingTypeScriptPrimitiveNormalization]struct{},
		),
	}
}

func (progress *directCodingTypeScriptCorrectionProgress) beginStage() error {
	if progress == nil || progress.seen == nil ||
		progress.stageDeterministicCorrections == nil || progress.stageSemanticCorrections == nil ||
		progress.stagePrimitiveNormalizations == nil {
		return fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	clear(progress.stageDeterministicCorrections)
	clear(progress.stageSemanticCorrections)
	clear(progress.stagePrimitiveNormalizations)
	return nil
}

func (progress *directCodingTypeScriptCorrectionProgress) authorizeDeterministicRepair(
	blockID string,
	repair directCodingTypeScriptDeterministicRepair,
) (bool, error) {
	if progress == nil || progress.stagePrimitiveNormalizations == nil {
		return false, fmt.Errorf("TypeScript correction progress authority is unavailable")
	}
	blockID = strings.TrimSpace(blockID)
	if blockID == "" || strings.TrimSpace(repair.Replacement) == "" ||
		repair.NormalizationStartByte == nil || *repair.NormalizationStartByte < 0 {
		return false, fmt.Errorf(
			"TypeScript deterministic repair authorization requires one block, replacement, and normalization occurrence",
		)
	}
	switch repair.Mechanism {
	case directCodingTypeScriptPrimitiveNullishNarrowing:
		return true, nil
	case directCodingTypeScriptPrimitiveReferenceNarrowing:
		key := directCodingTypeScriptPrimitiveNormalization{
			startByte:   *repair.NormalizationStartByte,
			replacement: repair.Replacement,
		}
		_, exists := progress.stagePrimitiveNormalizations[blockID][key]
		return exists, nil
	default:
		return false, fmt.Errorf("TypeScript deterministic repair mechanism %q is invalid", repair.Mechanism)
	}
}

func (progress *directCodingTypeScriptCorrectionProgress) recordDeterministicRepair(
	blockID string,
	repair directCodingTypeScriptDeterministicRepair,
) error {
	blockID = strings.TrimSpace(blockID)
	if progress == nil || progress.stagePrimitiveNormalizations == nil || blockID == "" ||
		repair.NormalizationStartByte == nil {
		return fmt.Errorf("record TypeScript deterministic repair requires exact progress authority")
	}
	owned := progress.stagePrimitiveNormalizations[blockID]
	for occurrence := range owned {
		if repair.StartByte <= occurrence.startByte {
			delete(owned, occurrence)
		}
	}
	if repair.Mechanism == directCodingTypeScriptPrimitiveNullishNarrowing {
		if owned == nil {
			owned = make(map[directCodingTypeScriptPrimitiveNormalization]struct{})
			progress.stagePrimitiveNormalizations[blockID] = owned
		}
		owned[directCodingTypeScriptPrimitiveNormalization{
			startByte:   *repair.NormalizationStartByte,
			replacement: repair.Replacement,
		}] = struct{}{}
	}
	return nil
}

func (progress *directCodingTypeScriptCorrectionProgress) invalidatePrimitiveNormalizations(
	blockID string,
) {
	delete(progress.stagePrimitiveNormalizations, strings.TrimSpace(blockID))
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
		progress.stageDeterministicCorrections == nil || progress.stageSemanticCorrections == nil ||
		progress.stagePrimitiveNormalizations == nil {
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
