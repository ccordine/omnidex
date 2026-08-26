package assemblyline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ArtifactCandidateSelectionSchemaV1 = "omnidex.artifact-candidate-selection.v1"
	ArtifactCandidateSelectionNone     = "NONE"
	maxArtifactSelectionCandidates     = 8
	maxArtifactCandidateDeclarations   = 4
	maxArtifactCandidateEvidenceBytes  = 256
)

var opaqueArtifactCandidatePattern = regexp.MustCompile(`^ARTIFACT_CANDIDATE_[1-9][0-9]*$`)

// ArtifactCandidateEvidence is a code-built semantic projection. Physical
// repository identity remains outside this model-visible value.
type ArtifactCandidateEvidence struct {
	CandidateID  string   `json:"candidate_id"`
	Declarations []string `json:"declarations"`
}

type ArtifactCandidateSelectionInput struct {
	RequirementQuote string                      `json:"requirement_quote"`
	Candidates       []ArtifactCandidateEvidence `json:"candidates"`
}

type ArtifactCandidateSelectionDecision struct {
	Schema      string `json:"schema"`
	CandidateID string `json:"candidate_id"`
}

func NewArtifactCandidateSelectionJob(input ArtifactCandidateSelectionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkArtifactCandidateSelection, input, input.validate)
}

func (input ArtifactCandidateSelectionInput) validate() error {
	if err := validateRequirementQuote("artifact candidate selection", input.RequirementQuote); err != nil {
		return err
	}
	if len(input.RequirementQuote) > maxDeclarationBoundaryQuoteBytes {
		return fmt.Errorf("artifact candidate selection quote exceeds %d bytes", maxDeclarationBoundaryQuoteBytes)
	}
	if len(input.Candidates) < 2 || len(input.Candidates) > maxArtifactSelectionCandidates {
		return fmt.Errorf(
			"artifact candidate selection requires 2-%d bounded candidates",
			maxArtifactSelectionCandidates,
		)
	}
	for index, candidate := range input.Candidates {
		expectedID := fmt.Sprintf("ARTIFACT_CANDIDATE_%d", index+1)
		if candidate.CandidateID != expectedID || !opaqueArtifactCandidatePattern.MatchString(candidate.CandidateID) {
			return fmt.Errorf("artifact candidate %d has non-code-owned identity %q", index, candidate.CandidateID)
		}
		if len(candidate.Declarations) < 1 || len(candidate.Declarations) > maxArtifactCandidateDeclarations {
			return fmt.Errorf(
				"artifact candidate %s requires 1-%d bounded declarations",
				candidate.CandidateID, maxArtifactCandidateDeclarations,
			)
		}
		seenDeclarations := make(map[string]struct{}, len(candidate.Declarations))
		for _, declaration := range candidate.Declarations {
			if declaration == "" || declaration != strings.TrimSpace(declaration) ||
				len(declaration) > maxArtifactCandidateEvidenceBytes || !utf8.ValidString(declaration) ||
				strings.ContainsAny(declaration, "\x00\r\n") {
				return fmt.Errorf("artifact candidate %s contains invalid semantic declaration evidence", candidate.CandidateID)
			}
			if err := ValidatePathFreeModelContext(
				"artifact candidate semantic declaration", declaration,
			); err != nil {
				return err
			}
			if _, duplicate := seenDeclarations[declaration]; duplicate {
				return fmt.Errorf("artifact candidate %s repeats declaration evidence", candidate.CandidateID)
			}
			seenDeclarations[declaration] = struct{}{}
		}
	}
	return nil
}

func (decision ArtifactCandidateSelectionDecision) ValidateFor(input ArtifactCandidateSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ArtifactCandidateSelectionSchemaV1 {
		return fmt.Errorf("artifact candidate selection schema must be %q", ArtifactCandidateSelectionSchemaV1)
	}
	if decision.CandidateID == ArtifactCandidateSelectionNone {
		return nil
	}
	for _, candidate := range input.Candidates {
		if decision.CandidateID == candidate.CandidateID {
			return nil
		}
	}
	return fmt.Errorf("artifact candidate selection %q is unavailable", decision.CandidateID)
}

func BuildArtifactCandidateSelectionPrompt(input ArtifactCandidateSelectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	evidence, err := json.Marshal(input.Candidates)
	if err != nil {
		return "", fmt.Errorf("encode artifact candidate evidence: %w", err)
	}
	return strings.Join([]string{
		"Select the one opaque candidate whose bounded declaration evidence resolves which known semantic artifact the exact requirement explicitly identifies as required to be absent.",
		"Return its opaque candidate ID, or NONE when the supplied evidence cannot distinguish exactly one.",
		"EXACT_REQUIREMENT:\n" + input.RequirementQuote,
		"BOUNDED_CANDIDATES:\n" + string(evidence),
	}, "\n\n"), nil
}

func ArtifactCandidateSelectionResponseSchema(input ArtifactCandidateSelectionInput) map[string]any {
	values := make([]string, 0, len(input.Candidates)+1)
	for _, candidate := range input.Candidates {
		values = append(values, candidate.CandidateID)
	}
	values = append(values, ArtifactCandidateSelectionNone)
	return objectSchema(
		[]string{"schema", "candidate_id"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ArtifactCandidateSelectionSchemaV1,
			},
			"candidate_id": map[string]any{"type": "string", "enum": values},
		},
	)
}
