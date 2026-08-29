package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

// RoleplayUserTurnProjection is the minimum model-visible identity and
// modality authority for one exact user contribution. Character IDs and
// persistence controls remain code-only.
type RoleplayUserTurnProjection struct {
	PersonaKind      roleplay.UserPersonaKind      `json:"persona_kind"`
	PersonaName      string                        `json:"persona_name"`
	PersonaSummary   string                        `json:"persona_summary,omitempty"`
	ContributionKind roleplay.UserContributionKind `json:"contribution_kind"`
	Parts            []roleplay.UserTurnPart       `json:"parts,omitempty"`
}

func ProjectRoleplayUserTurn(
	authority roleplay.UserTurnAuthority,
) (RoleplayUserTurnProjection, error) {
	if err := authority.Validate(); err != nil {
		return RoleplayUserTurnProjection{}, err
	}
	if authority.PersonaKind == roleplay.UserPersonaLegacy {
		return RoleplayUserTurnProjection{}, fmt.Errorf(
			"historical untyped user turn cannot become current response authority",
		)
	}
	projection := RoleplayUserTurnProjection{
		PersonaKind: authority.PersonaKind, PersonaName: authority.PersonaName,
		PersonaSummary: authority.PersonaSummary, ContributionKind: authority.ContributionKind,
		Parts: append([]roleplay.UserTurnPart(nil), authority.Parts...),
	}
	if err := projection.validate(); err != nil {
		return RoleplayUserTurnProjection{}, err
	}
	return projection, nil
}

func (projection RoleplayUserTurnProjection) validate() error {
	if err := validateContextText("roleplay user persona name", projection.PersonaName, 256); err != nil {
		return err
	}
	if err := validateOptionalContextText(
		"roleplay user persona summary", projection.PersonaSummary, roleplay.MaxSimulationTextBytes,
	); err != nil {
		return err
	}
	switch projection.PersonaKind {
	case roleplay.UserPersonaCharacter:
		if projection.ContributionKind != roleplay.UserContributionDialogue &&
			projection.ContributionKind != roleplay.UserContributionAction &&
			projection.ContributionKind != roleplay.UserContributionActionDialogue &&
			projection.ContributionKind != roleplay.UserContributionStructured {
			return fmt.Errorf("roleplay character user contribution is incompatible with its persona")
		}
	case roleplay.UserPersonaNarrator:
		if projection.PersonaName != roleplay.NarratorPersonaName || projection.PersonaSummary != "" {
			return fmt.Errorf("roleplay narrator projection changed exact presentation authority")
		}
		if projection.ContributionKind != roleplay.UserContributionNarration &&
			projection.ContributionKind != roleplay.UserContributionDirection &&
			projection.ContributionKind != roleplay.UserContributionNarrationDirection &&
			projection.ContributionKind != roleplay.UserContributionCommand {
			return fmt.Errorf("roleplay narrator contribution is incompatible with its persona")
		}
	default:
		return fmt.Errorf("roleplay response cannot receive user persona kind %q", projection.PersonaKind)
	}
	if len(projection.Parts) != 0 {
		authority := roleplay.UserTurnAuthority{
			PersonaKind: projection.PersonaKind, PersonaName: projection.PersonaName,
			PersonaSummary: projection.PersonaSummary, ContributionKind: projection.ContributionKind,
			Parts: projection.Parts,
		}
		request := roleplay.UserTurnRequest{
			PersonaKind: projection.PersonaKind, ContributionKind: projection.ContributionKind,
			Parts: projection.Parts,
		}
		if projection.PersonaKind == roleplay.UserPersonaCharacter {
			// The persisted character identity intentionally remains code-only. A
			// valid placeholder lets the shared deterministic composer verify the
			// exact ordered part semantics without exposing that identity.
			request.CharacterID = "rpc_00000000000000000000000000000000"
			authority.CharacterID = request.CharacterID
		}
		exact, err := roleplay.ComposeUserTurn(request)
		if err != nil {
			return err
		}
		authority.ExactText = exact
		if err := authority.Validate(); err != nil {
			return fmt.Errorf("roleplay user turn parts: %w", err)
		}
	} else if projection.ContributionKind == roleplay.UserContributionStructured {
		return fmt.Errorf("structured roleplay turn requires exact ordered parts")
	}
	return nil
}
