package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const DeclarationArtifactBoundarySchemaV1 = "omnidex.declaration-artifact-boundary.v1"

const maxDeclarationBoundaryQuoteBytes = 1024

var opaqueDeclarationPattern = regexp.MustCompile(`^DECLARATION_[1-9][0-9]*$`)

type DeclarationArtifactBoundary string

const (
	DeclarationBoundaryIndependentArtifact DeclarationArtifactBoundary = "independent_artifact"
	DeclarationBoundaryExistingArtifact    DeclarationArtifactBoundary = "existing_artifact"
	DeclarationBoundaryNone                DeclarationArtifactBoundary = "none"
)

type DeclarationArtifactBoundaryInput struct {
	RequirementQuote string `json:"requirement_quote"`
	GoSignature      string `json:"go_signature"`
	DeclarationID    string `json:"declaration_id"`
}

type DeclarationArtifactBoundaryDecision struct {
	Schema        string                      `json:"schema"`
	DeclarationID string                      `json:"declaration_id"`
	Boundary      DeclarationArtifactBoundary `json:"boundary"`
}

func NewDeclarationArtifactBoundaryJob(input DeclarationArtifactBoundaryInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDeclarationArtifactBoundary, input, input.validate)
}

func (input DeclarationArtifactBoundaryInput) validate() error {
	if err := validateRequirementQuote("declaration artifact boundary", input.RequirementQuote); err != nil {
		return err
	}
	if len(input.RequirementQuote) > maxDeclarationBoundaryQuoteBytes {
		return fmt.Errorf("declaration artifact boundary quote exceeds %d bytes", maxDeclarationBoundaryQuoteBytes)
	}
	if input.GoSignature == "" || input.GoSignature != strings.TrimSpace(input.GoSignature) ||
		strings.ContainsAny(input.GoSignature, "\r\n") || len(input.GoSignature) > 1024 ||
		!strings.HasPrefix(input.GoSignature, "func ") {
		return fmt.Errorf("declaration artifact boundary requires one exact bounded Go function signature")
	}
	if !opaqueDeclarationPattern.MatchString(input.DeclarationID) {
		return fmt.Errorf("declaration artifact boundary ID %q is not code-owned and opaque", input.DeclarationID)
	}
	return nil
}

func (decision DeclarationArtifactBoundaryDecision) ValidateFor(input DeclarationArtifactBoundaryInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != DeclarationArtifactBoundarySchemaV1 {
		return fmt.Errorf("declaration artifact boundary schema must be %q", DeclarationArtifactBoundarySchemaV1)
	}
	if decision.DeclarationID != input.DeclarationID {
		return fmt.Errorf(
			"declaration artifact boundary ID %q does not match focused declaration %q",
			decision.DeclarationID, input.DeclarationID,
		)
	}
	switch decision.Boundary {
	case DeclarationBoundaryIndependentArtifact,
		DeclarationBoundaryExistingArtifact,
		DeclarationBoundaryNone:
		return nil
	default:
		return fmt.Errorf("declaration artifact boundary %q is unsupported", decision.Boundary)
	}
}

func BuildDeclarationArtifactBoundaryPrompt(input DeclarationArtifactBoundaryInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the explicit semantic artifact boundary of FOCUSED_DECLARATION in REQUIREMENT_QUOTE.",
		"Choose independent_artifact when the quote explicitly requires the declaration to be an independently owned artifact, existing_artifact when it explicitly requires the declaration to belong to a previously established artifact, or none when neither relationship is specified.",
		"FOCUSED_DECLARATION: " + input.DeclarationID,
		"EXACT_GO_SIGNATURE: " + input.GoSignature,
		"REQUIREMENT_QUOTE:\n" + input.RequirementQuote,
	}, "\n\n"), nil
}

func DeclarationArtifactBoundaryResponseSchema(input DeclarationArtifactBoundaryInput) map[string]any {
	return objectSchema(
		[]string{"schema", "declaration_id", "boundary"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": DeclarationArtifactBoundarySchemaV1,
			},
			"declaration_id": map[string]any{"type": "string", "const": input.DeclarationID},
			"boundary": enumSchema(
				DeclarationBoundaryIndependentArtifact,
				DeclarationBoundaryExistingArtifact,
				DeclarationBoundaryNone,
			),
		},
	)
}
