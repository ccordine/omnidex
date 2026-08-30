package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

type RoleplayCanonFactCandidateRelationInput struct {
	Candidate    string `json:"candidate"`
	AcceptedFact string `json:"accepted_fact"`
}

type RoleplayCanonFactCandidateRelation struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewRoleplayCanonFactCandidateRelationJob(
	input RoleplayCanonFactCandidateRelationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRoleplayCanonFactCandidateRelation, input, input.validate,
	)
}

func (input RoleplayCanonFactCandidateRelationInput) validate() error {
	if err := roleplay.ValidateCanonFact(input.Candidate); err != nil {
		return err
	}
	if err := roleplay.ValidateCanonFact(input.AcceptedFact); err != nil {
		return err
	}
	if input.Candidate == input.AcceptedFact {
		return fmt.Errorf("exactly identical roleplay canon facts must be deduplicated by code")
	}
	return nil
}

func BuildRoleplayCanonFactCandidateRelationPrompt(
	input RoleplayCanonFactCandidateRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one pairwise semantic relation: do the candidate fact and the already accepted fact express the same durable fictional fact?",
		"Compare only whether these two statements express the same durable fictional fact.",
		"Return SAME_CANON_FACT when retaining both would duplicate one fact despite wording differences. Return DISTINCT_CANON_FACT when each fact adds a different durable assertion.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"CANDIDATE FACT:\n" + input.Candidate,
		"ALREADY ACCEPTED FACT:\n" + input.AcceptedFact,
	}, "\n\n"), nil
}

func DecodeRoleplayCanonFactCandidateRelation(
	input RoleplayCanonFactCandidateRelationInput,
	raw string,
) (RoleplayCanonFactCandidateRelation, error) {
	var zero RoleplayCanonFactCandidateRelation
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"roleplay canon fact candidate relation",
		raw,
		maximumStringBytes(RoleplayCanonFactsEquivalent, RoleplayCanonFactsDistinct),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RoleplayCanonFactCandidateRelation{
		Schema: RoleplayCanonFactRelationSchemaV1, AuthoritySHA256: authoritySHA256, Relation: leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result RoleplayCanonFactCandidateRelation) ValidateFor(
	input RoleplayCanonFactCandidateRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != RoleplayCanonFactRelationSchemaV1 {
		return fmt.Errorf("roleplay canon fact relation schema must be %q", RoleplayCanonFactRelationSchemaV1)
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("roleplay canon fact relation authority hash does not match")
	}
	switch result.Relation {
	case RoleplayCanonFactsEquivalent, RoleplayCanonFactsDistinct:
		return nil
	default:
		return fmt.Errorf("roleplay canon fact relation value %q is not registered", result.Relation)
	}
}
