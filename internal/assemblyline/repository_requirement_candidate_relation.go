package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkRepositoryRequirementCandidateRelation WorkKind = "repository_requirement_candidate_relation"

	RepositoryRequirementCandidatesSameChange      = "SAME_WORKSPACE_CHANGE_REQUIREMENT"
	RepositoryRequirementCandidatesDistinctChanges = "DISTINCT_WORKSPACE_CHANGE_REQUIREMENTS"

	RepositoryRequirementCandidateRelationSchemaV1 = "omnidex.repository-requirement-candidate-relation.v1"
)

type RepositoryRequirementCandidateRelationInput struct {
	Candidate           string `json:"candidate"`
	AcceptedRequirement string `json:"accepted_requirement"`
}

type RepositoryRequirementCandidateRelationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewRepositoryRequirementCandidateRelationJob(
	input RepositoryRequirementCandidateRelationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkRepositoryRequirementCandidateRelation,
		input,
	)
}

func (input RepositoryRequirementCandidateRelationInput) validate() error {
	if err := validateRepositoryRequirementStatement(
		"repository requirement relation candidate",
		input.Candidate,
	); err != nil {
		return err
	}
	if err := validateRepositoryRequirementStatement(
		"repository requirement relation accepted requirement",
		input.AcceptedRequirement,
	); err != nil {
		return err
	}
	if input.Candidate == input.AcceptedRequirement {
		return fmt.Errorf("identical repository requirement candidates must be deduplicated by code")
	}
	return nil
}

func BuildRepositoryRequirementCandidateRelationPrompt(
	input RepositoryRequirementCandidateRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one pairwise semantic relation: do the exact candidate clause and the already retained requirement express the same requested existing-workspace change?",
		"Compare only whether these two statements express the same requested workspace change.",
		"Return SAME_WORKSPACE_CHANGE_REQUIREMENT when retaining both would duplicate one requested change despite wording differences. Return DISTINCT_WORKSPACE_CHANGE_REQUIREMENTS when each statement adds a separately satisfiable change or constraint.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"CANDIDATE CLAUSE:\n" + input.Candidate,
		"ALREADY RETAINED REQUIREMENT:\n" + input.AcceptedRequirement,
	}, "\n\n"), nil
}

func DecodeRepositoryRequirementCandidateRelationResult(
	input RepositoryRequirementCandidateRelationInput,
	raw string,
) (RepositoryRequirementCandidateRelationResult, error) {
	var zero RepositoryRequirementCandidateRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"repository requirement candidate relation",
		raw,
		maximumStringBytes(
			RepositoryRequirementCandidatesSameChange,
			RepositoryRequirementCandidatesDistinctChanges,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := repositoryRequirementCandidateRelationAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RepositoryRequirementCandidateRelationResult{
		Schema:          RepositoryRequirementCandidateRelationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result RepositoryRequirementCandidateRelationResult) ValidateFor(
	input RepositoryRequirementCandidateRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != RepositoryRequirementCandidateRelationSchemaV1 {
		return fmt.Errorf(
			"repository requirement candidate relation schema must be %q",
			RepositoryRequirementCandidateRelationSchemaV1,
		)
	}
	authoritySHA256, err := repositoryRequirementCandidateRelationAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("repository requirement candidate relation authority hash does not match")
	}
	switch result.Relation {
	case RepositoryRequirementCandidatesSameChange,
		RepositoryRequirementCandidatesDistinctChanges:
		return nil
	default:
		return fmt.Errorf(
			"repository requirement candidate relation value %q is not registered",
			result.Relation,
		)
	}
}

func repositoryRequirementCandidateRelationAuthoritySHA256(
	input RepositoryRequirementCandidateRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode repository requirement candidate relation authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
