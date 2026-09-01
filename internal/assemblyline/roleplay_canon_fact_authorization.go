package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

type RoleplayCanonFactCandidateAuthorizationInput struct {
	Authority RoleplayCanonExtractionInput `json:"authority"`
	Candidate string                       `json:"candidate"`
}

type RoleplayCanonFactCandidateAuthorization struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewRoleplayCanonFactCandidateAuthorizationJob(
	input RoleplayCanonFactCandidateAuthorizationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkRoleplayCanonFactCandidateAuthorization, input,
	)
}

func (input RoleplayCanonFactCandidateAuthorizationInput) validate() error {
	if err := input.Authority.validate(); err != nil {
		return err
	}
	return roleplay.ValidateCanonFact(input.Candidate)
}

func BuildRoleplayCanonFactCandidateAuthorizationPrompt(
	input RoleplayCanonFactCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderRoleplayCanonExtractionAuthority(input.Authority)
	if err != nil {
		return "", err
	}
	choices, err := roleplayCanonFactAuthorizationChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Is the exact candidate a durable fictional fact directly established by the exact current contribution?",
		[]string{
			"The contribution is the only fact source. Context may resolve a reference but cannot independently establish the candidate. Implications, requests, questions, directions, decorative detail, inferred visibility, real-world claims, and facts merely plausible for a later turn are not established by the contribution.",
			authority,
			"Exact candidate fact:\n" + input.Candidate,
		},
		choices,
	)
}

func DecodeRoleplayCanonFactCandidateAuthorization(
	input RoleplayCanonFactCandidateAuthorizationInput,
	raw string,
) (RoleplayCanonFactCandidateAuthorization, error) {
	var zero RoleplayCanonFactCandidateAuthorization
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := roleplayCanonFactAuthorizationChoices()
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
	result := RoleplayCanonFactCandidateAuthorization{
		Schema: RoleplayCanonFactAuthorizationSchemaV1, AuthoritySHA256: authoritySHA256, Relation: leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func roleplayCanonFactAuthorizationChoices() ([]OpaqueModelChoice, error) {
	established, err := NewOpaqueModelChoice(
		"The complete candidate is directly entailed by the current contribution.",
		RoleplayCanonFactEstablished,
	)
	if err != nil {
		return nil, err
	}
	notEstablished, err := NewOpaqueModelChoice(
		"The complete candidate is not directly entailed by the current contribution.",
		RoleplayCanonFactNotEstablished,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{established, notEstablished}, nil
}

func (result RoleplayCanonFactCandidateAuthorization) ValidateFor(
	input RoleplayCanonFactCandidateAuthorizationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != RoleplayCanonFactAuthorizationSchemaV1 {
		return fmt.Errorf("roleplay canon fact authorization schema must be %q", RoleplayCanonFactAuthorizationSchemaV1)
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("roleplay canon fact authorization authority hash does not match")
	}
	switch result.Relation {
	case RoleplayCanonFactEstablished, RoleplayCanonFactNotEstablished:
		return nil
	default:
		return fmt.Errorf("roleplay canon fact authorization value %q is not registered", result.Relation)
	}
}
