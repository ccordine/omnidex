package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

type RoleplayCanonSourceKind string

const (
	RoleplayCanonSourceUserContribution  RoleplayCanonSourceKind = "user_contribution"
	RoleplayCanonSourceAssistantResponse RoleplayCanonSourceKind = "assistant_response"
)

// RoleplayCanonSource projects exactly one accepted fictional contribution.
// Source identities and persistence recipients remain code-only.
type RoleplayCanonSource struct {
	Kind                  RoleplayCanonSourceKind       `json:"kind"`
	AttributedPersonaName string                        `json:"attributed_persona_name"`
	ExactContribution     string                        `json:"exact_contribution"`
	PersonaKind           roleplay.UserPersonaKind      `json:"persona_kind,omitempty"`
	ContributionKind      roleplay.UserContributionKind `json:"contribution_kind,omitempty"`
}

// RoleplayCanonAntecedent is the minimum accepted user contribution needed to
// resolve referents in one assistant response. It is never a fact source.
type RoleplayCanonAntecedent struct {
	PersonaKind         roleplay.UserPersonaKind      `json:"persona_kind"`
	PersonaName         string                        `json:"persona_name"`
	ContributionKind    roleplay.UserContributionKind `json:"contribution_kind"`
	ContributionContext string                        `json:"contribution_context"`
}

func ProjectRoleplayUserCanonSource(
	authority roleplay.UserTurnAuthority,
) (RoleplayCanonSource, bool, error) {
	contribution, present, err := authority.CanonContribution()
	if err != nil || !present {
		return RoleplayCanonSource{}, present, err
	}
	source := RoleplayCanonSource{
		Kind:                  RoleplayCanonSourceUserContribution,
		AttributedPersonaName: authority.PersonaName,
		ExactContribution:     contribution,
		PersonaKind:           authority.PersonaKind,
		ContributionKind:      authority.ContributionKind,
	}
	if err := source.validate(); err != nil {
		return RoleplayCanonSource{}, false, err
	}
	return source, true, nil
}

func ProjectRoleplayCanonAntecedent(
	authority roleplay.UserTurnAuthority,
	modelVisibleContribution string,
) (RoleplayCanonAntecedent, error) {
	if err := authority.Validate(); err != nil {
		return RoleplayCanonAntecedent{}, err
	}
	if authority.PersonaKind == roleplay.UserPersonaLegacy {
		return RoleplayCanonAntecedent{}, fmt.Errorf(
			"historical untyped user turn cannot become current canon antecedent",
		)
	}
	antecedent := RoleplayCanonAntecedent{
		PersonaKind: authority.PersonaKind, PersonaName: authority.PersonaName,
		ContributionKind:    authority.ContributionKind,
		ContributionContext: modelVisibleContribution,
	}
	if err := antecedent.validate(); err != nil {
		return RoleplayCanonAntecedent{}, err
	}
	return antecedent, nil
}

func NewRoleplayAssistantCanonSource(
	personaName string,
	exactResponse string,
) (RoleplayCanonSource, error) {
	source := RoleplayCanonSource{
		Kind:                  RoleplayCanonSourceAssistantResponse,
		AttributedPersonaName: personaName,
		ExactContribution:     exactResponse,
	}
	if err := source.validate(); err != nil {
		return RoleplayCanonSource{}, err
	}
	return source, nil
}

func (source RoleplayCanonSource) validate() error {
	maximum := 0
	switch source.Kind {
	case RoleplayCanonSourceUserContribution:
		maximum = roleplay.MaxUserTurnBytes
		if err := validateRoleplayCanonModality(
			source.PersonaKind, source.ContributionKind,
		); err != nil {
			return err
		}
		if source.ContributionKind == roleplay.UserContributionCommand {
			return fmt.Errorf("roleplay command cannot become a fictional canon source")
		}
		if source.ContributionKind == roleplay.UserContributionDirection {
			return fmt.Errorf("roleplay direction cannot become a fictional canon source")
		}
	case RoleplayCanonSourceAssistantResponse:
		maximum = roleplay.MaxNarrativeResponseBytes
		if source.PersonaKind != "" || source.ContributionKind != "" {
			return fmt.Errorf("roleplay assistant canon source cannot carry user modality authority")
		}
	default:
		return fmt.Errorf("roleplay canon source kind %q is unsupported", source.Kind)
	}
	if err := validateContextText(
		"roleplay canon attributed persona name", source.AttributedPersonaName, 256,
	); err != nil {
		return err
	}
	if err := validateGroundedText(
		"roleplay canon exact contribution", source.ExactContribution,
		maximum, true,
	); err != nil {
		return err
	}
	if source.Kind == RoleplayCanonSourceUserContribution &&
		strings.HasPrefix(source.ExactContribution, "/") {
		return fmt.Errorf("roleplay user canon source cannot receive raw command bytes")
	}
	return nil
}

func (antecedent RoleplayCanonAntecedent) validate() error {
	if err := validateContextText(
		"roleplay canon antecedent persona name", antecedent.PersonaName, 256,
	); err != nil {
		return err
	}
	if err := validateGroundedText(
		"roleplay canon antecedent contribution", antecedent.ContributionContext,
		roleplay.MaxUserTurnBytes, false,
	); err != nil {
		return err
	}
	if strings.HasPrefix(antecedent.ContributionContext, "/") {
		return fmt.Errorf("roleplay canon antecedent cannot receive raw command bytes")
	}
	return validateRoleplayCanonModality(
		antecedent.PersonaKind, antecedent.ContributionKind,
	)
}

func validateRoleplayCanonModality(
	persona roleplay.UserPersonaKind,
	contribution roleplay.UserContributionKind,
) error {
	switch persona {
	case roleplay.UserPersonaCharacter:
		if contribution != roleplay.UserContributionDialogue &&
			contribution != roleplay.UserContributionAction &&
			contribution != roleplay.UserContributionActionDialogue &&
			contribution != roleplay.UserContributionStructured {
			return fmt.Errorf("roleplay canon character modality is invalid")
		}
	case roleplay.UserPersonaNarrator:
		if contribution != roleplay.UserContributionNarration &&
			contribution != roleplay.UserContributionDirection &&
			contribution != roleplay.UserContributionNarrationDirection &&
			contribution != roleplay.UserContributionCommand {
			return fmt.Errorf("roleplay canon narrator modality is invalid")
		}
	default:
		return fmt.Errorf("roleplay canon persona kind %q is unsupported", persona)
	}
	return nil
}
