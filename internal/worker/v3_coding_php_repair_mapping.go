package worker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

var (
	phpLintRepairPattern = regexp.MustCompile(
		`(?mi)(?:PHP\s+)?(?:Parse|Fatal) error:\s*(.+?)\s+in\s+[^\r\n]+\s+on line\s+([0-9]+)`,
	)
	phpRuntimeRepairPattern = regexp.MustCompile(
		`(?mi)(?:PHP\s+)?Fatal error:\s+Uncaught\s+[A-Za-z_\\][A-Za-z0-9_\\]*:\s*(.+?)\s+in\s+[^\r\n]+:[0-9]+`,
	)
)

func directCodingPHPRepairConfig() directCodingLanguageRepairConfig {
	return directCodingLanguageRepairConfig{MapStageFailure: mapDirectCodingPHPStageFailure}
}

func mapDirectCodingPHPStageFailure(
	program directCodingProgram,
	documents []assemblyline.ComposedSourceDocument,
	command testCommand,
	output string,
) (directCodingLanguageStageRepair, bool, error) {
	document, found, err := directCodingCommandSourceDocument(documents, command.Args)
	if err != nil || !found {
		return directCodingLanguageStageRepair{}, false, err
	}
	switch command.Purpose {
	case verificationSyntax:
		return mapDirectCodingPHPSyntaxFailure(program, documents, document, output)
	case verificationTest:
		return mapDirectCodingPHPAcceptanceFailure(program, documents, document, output)
	default:
		return directCodingLanguageStageRepair{}, false, nil
	}
}

func mapDirectCodingPHPSyntaxFailure(
	program directCodingProgram,
	documents []assemblyline.ComposedSourceDocument,
	document assemblyline.ComposedSourceDocument,
	output string,
) (directCodingLanguageStageRepair, bool, error) {
	matches := phpLintRepairPattern.FindStringSubmatch(
		directCodingANSISequencePattern.ReplaceAllString(output, ""),
	)
	if len(matches) != 3 {
		return directCodingLanguageStageRepair{}, false, nil
	}
	line, err := strconv.Atoi(matches[2])
	if err != nil || line < 1 {
		return directCodingLanguageStageRepair{}, false, fmt.Errorf("PHP lint failure has an invalid source line")
	}
	blockID := ""
	span := assemblyline.SourceSpan{}
	for candidateID, candidateSpan := range document.Spans {
		if candidateSpan.Contains(line) {
			if blockID != "" {
				return directCodingLanguageStageRepair{}, false, fmt.Errorf("PHP lint line maps to multiple source blocks")
			}
			blockID, span = candidateID, candidateSpan
		}
	}
	ref, found := directCodingSourceBlockRef(program.Source, blockID)
	if !found || !ref.Block.Generated() {
		return directCodingLanguageStageRepair{}, false, nil
	}
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		return directCodingLanguageStageRepair{}, false, fmt.Errorf("generated verification source is not repair model context")
	}
	diagnostic := fmt.Sprintf(
		"DECLARATION_LOCATION: line %d\nSOURCE_DIAGNOSTIC: %s",
		line-span.StartLine+1, strings.TrimSpace(matches[1]),
	)
	if err := validateDirectCodingPHPRepairDiagnostic(documents, diagnostic); err != nil {
		return directCodingLanguageStageRepair{}, false, err
	}
	return directCodingLanguageStageRepair{Target: ref, Diagnostic: diagnostic}, true, nil
}

func mapDirectCodingPHPAcceptanceFailure(
	program directCodingProgram,
	documents []assemblyline.ComposedSourceDocument,
	document assemblyline.ComposedSourceDocument,
	output string,
) (directCodingLanguageStageRepair, bool, error) {
	verification := assemblyline.SourceBlock{}
	for _, sourceDocument := range program.Source.Documents {
		if sourceDocument.ID != document.ID {
			continue
		}
		for _, block := range sourceDocument.Blocks {
			if block.Generated() && block.Role == assemblyline.SourceBlockTaskVerification {
				if verification.ID != "" {
					return directCodingLanguageStageRepair{}, false, fmt.Errorf("focused PHP verification has multiple generated oracles")
				}
				verification = block
			}
		}
	}
	if verification.ID == "" {
		return directCodingLanguageStageRepair{}, false, nil
	}
	ownerID := ""
	for _, dependencyID := range verification.DependsOn {
		dependency, found := directCodingSourceBlueprintBlock(program.Source, dependencyID)
		if !found || !dependency.Generated() ||
			dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		if ownerID != "" {
			return directCodingLanguageStageRepair{}, false, fmt.Errorf("focused PHP verification has multiple implementation owners")
		}
		ownerID = dependencyID
	}
	target, found := directCodingSourceBlockRef(program.Source, ownerID)
	if !found {
		return directCodingLanguageStageRepair{}, false, fmt.Errorf("focused PHP verification has no generated implementation owner")
	}
	matches := phpRuntimeRepairPattern.FindStringSubmatch(
		directCodingANSISequencePattern.ReplaceAllString(output, ""),
	)
	if len(matches) != 2 {
		return directCodingLanguageStageRepair{}, false, nil
	}
	diagnostic := "SOURCE_DIAGNOSTIC: " + strings.TrimSpace(matches[1])
	if err := validateDirectCodingPHPRepairDiagnostic(documents, diagnostic); err != nil {
		return directCodingLanguageStageRepair{}, false, err
	}
	return directCodingLanguageStageRepair{Target: target, Diagnostic: diagnostic}, true, nil
}

func directCodingCommandSourceDocument(
	documents []assemblyline.ComposedSourceDocument,
	args []string,
) (assemblyline.ComposedSourceDocument, bool, error) {
	matched := assemblyline.ComposedSourceDocument{}
	for _, document := range documents {
		for _, argument := range args {
			if argument != document.Path {
				continue
			}
			if matched.ID != "" && matched.ID != document.ID {
				return assemblyline.ComposedSourceDocument{}, false, fmt.Errorf("stage command names multiple source documents")
			}
			matched = document
		}
	}
	return matched, matched.ID != "", nil
}

func validateDirectCodingPHPRepairDiagnostic(
	documents []assemblyline.ComposedSourceDocument,
	diagnostic string,
) error {
	paths := make([]string, len(documents))
	for index, document := range documents {
		paths[index] = document.Path
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(paths)
	if err != nil {
		return err
	}
	diagnostic = strings.TrimSpace(diagnostic)
	if diagnostic == "" || len(diagnostic) > maxDirectCodingLanguageRepairDiagnosticBytes {
		return fmt.Errorf("PHP repair diagnostic is empty or oversized")
	}
	return assemblyline.ValidatePathFreeModelContextWithProvenance(
		"PHP repair diagnostic", provenance, diagnostic,
	)
}
