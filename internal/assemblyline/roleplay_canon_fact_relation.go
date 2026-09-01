package assemblyline

import (
	"fmt"

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
	return newPortableJob(
		WorkRoleplayCanonFactCandidateRelation, input,
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
	choices, err := roleplayCanonFactRelationChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Do these two statements express the same durable fictional fact?",
		[]string{
			"Candidate fact:\n" + input.Candidate,
			"Already accepted fact:\n" + input.AcceptedFact,
		},
		choices,
	)
}

func DecodeRoleplayCanonFactCandidateRelation(
	input RoleplayCanonFactCandidateRelationInput,
	raw string,
) (RoleplayCanonFactCandidateRelation, error) {
	var zero RoleplayCanonFactCandidateRelation
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := roleplayCanonFactRelationChoices()
	if err != nil {
		return zero, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
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

func roleplayCanonFactRelationChoices() ([]OpaqueModelChoice, error) {
	same, err := NewOpaqueModelChoice(
		"Retaining both would duplicate one durable fictional fact despite wording differences.",
		RoleplayCanonFactsEquivalent,
	)
	if err != nil {
		return nil, err
	}
	distinct, err := NewOpaqueModelChoice(
		"Each statement adds a different durable fictional assertion.",
		RoleplayCanonFactsDistinct,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{same, distinct}, nil
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
