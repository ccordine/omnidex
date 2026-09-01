package assemblyline

import "fmt"

const (
	WorkRoleplayCanonFactPresence WorkKind = "roleplay_canon_fact_presence"

	RoleplayCanonContributionEstablishesFact   = "ESTABLISHES_DURABLE_FICTIONAL_FACT"
	RoleplayCanonContributionEstablishesNoFact = "ESTABLISHES_NO_DURABLE_FICTIONAL_FACT"

	RoleplayCanonFactPresenceSchemaV1 = "omnidex.roleplay-canon-fact-presence.v1"
)

type RoleplayCanonFactPresenceResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewRoleplayCanonFactPresenceJob(
	input RoleplayCanonExtractionInput,
) (PortableJob, error) {
	return newPortableJob(WorkRoleplayCanonFactPresence, input)
}

func BuildRoleplayCanonFactPresencePrompt(
	input RoleplayCanonExtractionInput,
) (string, error) {
	authority, err := renderRoleplayCanonExtractionAuthority(input)
	if err != nil {
		return "", err
	}
	choices, err := roleplayCanonFactPresenceChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Does this contribution directly establish any durable fictional fact? Questions, requests, directions, commands, implications, restatements of established context, decorative detail, and real-world claims do not establish one.",
		[]string{authority},
		choices,
	)
}

func DecodeRoleplayCanonFactPresenceResult(
	input RoleplayCanonExtractionInput,
	raw string,
) (RoleplayCanonFactPresenceResult, error) {
	var zero RoleplayCanonFactPresenceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := roleplayCanonFactPresenceChoices()
	if err != nil {
		return zero, err
	}
	relation, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RoleplayCanonFactPresenceResult{
		Schema:          RoleplayCanonFactPresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        relation,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result RoleplayCanonFactPresenceResult) ValidateFor(
	input RoleplayCanonExtractionInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != RoleplayCanonFactPresenceSchemaV1 {
		return fmt.Errorf("roleplay canon fact presence schema must be %q", RoleplayCanonFactPresenceSchemaV1)
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("roleplay canon fact presence authority hash does not match")
	}
	switch result.Relation {
	case RoleplayCanonContributionEstablishesFact,
		RoleplayCanonContributionEstablishesNoFact:
		return nil
	default:
		return fmt.Errorf("roleplay canon fact presence relation %q is not registered", result.Relation)
	}
}

func roleplayCanonFactPresenceChoices() ([]OpaqueModelChoice, error) {
	present, err := NewOpaqueModelChoice(
		"The contribution directly establishes at least one durable fictional fact.",
		RoleplayCanonContributionEstablishesFact,
	)
	if err != nil {
		return nil, err
	}
	absent, err := NewOpaqueModelChoice(
		"The contribution directly establishes no durable fictional fact.",
		RoleplayCanonContributionEstablishesNoFact,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{present, absent}, nil
}
