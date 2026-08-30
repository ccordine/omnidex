package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkRepositoryRequirementCandidateAuthorization WorkKind = "repository_requirement_candidate_authorization"

	RepositoryRequirementCandidateRequiresChange = "REQUIRES_EXISTING_WORKSPACE_CHANGE"
	RepositoryRequirementCandidateNoChange       = "DOES_NOT_REQUIRE_EXISTING_WORKSPACE_CHANGE"

	RepositoryRequirementCandidateAuthorizationSchemaV1 = "omnidex.repository-requirement-candidate-authorization.v1"
)

type RepositoryRequirementCandidateAuthorizationInput struct {
	Authority      RepositoryRequirementInterpretationInput `json:"authority"`
	Inventory      RepositoryRequirementInventory           `json:"inventory"`
	CandidateIndex int                                      `json:"candidate_index"`
}

type RepositoryRequirementCandidateAuthorizationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewRepositoryRequirementCandidateAuthorizationJob(
	input RepositoryRequirementCandidateAuthorizationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRepositoryRequirementCandidateAuthorization,
		input,
		input.validate,
	)
}

func (input RepositoryRequirementCandidateAuthorizationInput) validate() error {
	if err := input.Inventory.ValidateFor(input.Authority); err != nil {
		return err
	}
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return fmt.Errorf("repository requirement candidate index is outside inventory")
	}
	return nil
}

func (input RepositoryRequirementCandidateAuthorizationInput) candidate() string {
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return ""
	}
	return input.Inventory.Candidates[input.CandidateIndex]
}

func BuildRepositoryRequirementCandidateAuthorizationPrompt(
	input RepositoryRequirementCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(
		input.Authority.UserRequest,
		input.Authority.Context,
	)
	return strings.Join([]string{
		"Answer one semantic relation about the exact source clause below: does that clause explicitly require a persisted change to the existing workspace or explicitly constrain such a requested change?",
		"Evaluate only whether this exact clause requires or constrains a persisted workspace change.",
		"Return REQUIRES_EXISTING_WORKSPACE_CHANGE for an explicit request to create, modify, remove, or preserve repository behavior, source, tests, configuration, or documentation, including a constraint that directly governs a requested workspace change.",
		"Return DOES_NOT_REQUIRE_EXISTING_WORKSPACE_CHANGE for background, product identity alone, explanation, advice, a question, current status, a command that only runs or inspects existing software, or a verification direction that does not itself request a persisted repository change.",
		"Judge only what the immutable request establishes for this exact quoted candidate. Do not infer customary work, prerequisites, enhancements, or likely consequences.",
		"Return only the raw registered relation, with no JSON, label, Markdown, or explanation.",
		"IMMUTABLE EXISTING-REPOSITORY REQUEST AND ESTABLISHED FACTS:\n" + projection,
		"EXACT SOURCE CLAUSE CANDIDATE:\n" + input.candidate(),
		"FINAL QUESTION:\nDoes this exact clause require or directly constrain a persisted existing-workspace change? Return only REQUIRES_EXISTING_WORKSPACE_CHANGE or DOES_NOT_REQUIRE_EXISTING_WORKSPACE_CHANGE.",
	}, "\n\n"), nil
}

func DecodeRepositoryRequirementCandidateAuthorizationResult(
	input RepositoryRequirementCandidateAuthorizationInput,
	raw string,
) (RepositoryRequirementCandidateAuthorizationResult, error) {
	var zero RepositoryRequirementCandidateAuthorizationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"repository requirement candidate authorization",
		raw,
		maximumStringBytes(
			RepositoryRequirementCandidateRequiresChange,
			RepositoryRequirementCandidateNoChange,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := repositoryRequirementCandidateAuthorizationAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RepositoryRequirementCandidateAuthorizationResult{
		Schema:          RepositoryRequirementCandidateAuthorizationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result RepositoryRequirementCandidateAuthorizationResult) ValidateFor(
	input RepositoryRequirementCandidateAuthorizationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != RepositoryRequirementCandidateAuthorizationSchemaV1 {
		return fmt.Errorf(
			"repository requirement candidate authorization schema must be %q",
			RepositoryRequirementCandidateAuthorizationSchemaV1,
		)
	}
	authoritySHA256, err := repositoryRequirementCandidateAuthorizationAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("repository requirement candidate authorization authority hash does not match")
	}
	switch result.Relation {
	case RepositoryRequirementCandidateRequiresChange,
		RepositoryRequirementCandidateNoChange:
		return nil
	default:
		return fmt.Errorf(
			"repository requirement candidate authorization value %q is not registered",
			result.Relation,
		)
	}
}

func repositoryRequirementCandidateAuthorizationAuthoritySHA256(
	input RepositoryRequirementCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode repository requirement candidate authorization authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
