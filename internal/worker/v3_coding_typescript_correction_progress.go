package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptCorrectionState struct {
	blockID           string
	candidate         string
	verificationStage string
	diagnostic        string
}

type directCodingTypeScriptCorrectionProgress struct {
	seen map[directCodingTypeScriptCorrectionState]struct{}
}

func newDirectCodingTypeScriptCorrectionProgress() *directCodingTypeScriptCorrectionProgress {
	return &directCodingTypeScriptCorrectionProgress{
		seen: make(map[directCodingTypeScriptCorrectionState]struct{}),
	}
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
	progress.seen[state] = struct{}{}
	return nil
}

type directCodingTypeScriptSyntaxProgress struct {
	failure     assemblyline.TypeScriptSyntaxFailure
	occurrences int
}

type directCodingTypeScriptSyntaxRepair struct {
	radius           int
	wholeDeclaration bool
}

func (progress *directCodingTypeScriptSyntaxProgress) next(
	failure assemblyline.TypeScriptSyntaxFailure,
) directCodingTypeScriptSyntaxRepair {
	if progress.failure == failure {
		progress.occurrences++
	} else {
		progress.failure = failure
		progress.occurrences = 1
	}
	switch progress.occurrences {
	case 1:
		return directCodingTypeScriptSyntaxRepair{radius: 2}
	case 2:
		return directCodingTypeScriptSyntaxRepair{radius: 4}
	default:
		return directCodingTypeScriptSyntaxRepair{wholeDeclaration: true}
	}
}

func (repair directCodingTypeScriptSyntaxRepair) verificationStage() string {
	if repair.wholeDeclaration {
		return "fragment_syntax_whole_declaration"
	}
	return fmt.Sprintf("fragment_syntax_radius_%d", repair.radius)
}
