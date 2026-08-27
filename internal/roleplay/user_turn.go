package roleplay

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxUserTurnBytes      = 4 * 1024
	MaxUserTurnParts      = 16
	NarratorPersonaName   = "Narrator"
	LegacyUserPersonaName = "Unattributed user"
)

type UserPersonaKind string

const (
	UserPersonaCharacter UserPersonaKind = "character"
	UserPersonaNarrator  UserPersonaKind = "narrator"
	// UserPersonaLegacy records the honest absence of typed authority on turns
	// created before the persona boundary existed. New requests cannot select it.
	UserPersonaLegacy UserPersonaKind = "legacy_untyped"
)

type UserContributionKind string

const (
	UserContributionDialogue           UserContributionKind = "dialogue"
	UserContributionAction             UserContributionKind = "action"
	UserContributionActionDialogue     UserContributionKind = "action_dialogue"
	UserContributionNarration          UserContributionKind = "narration"
	UserContributionDirection          UserContributionKind = "direction"
	UserContributionNarrationDirection UserContributionKind = "narration_direction"
	UserContributionStructured         UserContributionKind = "structured_turn"
	UserContributionCommand            UserContributionKind = "command"
	UserContributionLegacy             UserContributionKind = "legacy_untyped"
)

type UserTurnPartKind string

const (
	UserTurnPartAction  UserTurnPartKind = "action"
	UserTurnPartMessage UserTurnPartKind = "message"
	UserTurnPartEvent   UserTurnPartKind = "event"
)

type UserTurnPart struct {
	Kind UserTurnPartKind `json:"kind"`
	Text string           `json:"text"`
}

type UserTurnRequest struct {
	PersonaKind      UserPersonaKind      `json:"persona_kind"`
	CharacterID      string               `json:"character_id,omitempty"`
	ContributionKind UserContributionKind `json:"contribution_kind"`
	Parts            []UserTurnPart       `json:"parts"`
}

type UserTurnAuthority struct {
	PersonaKind      UserPersonaKind      `json:"persona_kind"`
	CharacterID      string               `json:"character_id,omitempty"`
	PersonaName      string               `json:"persona_name"`
	PersonaSummary   string               `json:"persona_summary,omitempty"`
	ContributionKind UserContributionKind `json:"contribution_kind"`
	Parts            []UserTurnPart       `json:"parts"`
	ExactText        string               `json:"exact_text"`
}

func (request UserTurnRequest) ValidateForExactText(exactText string) error {
	if request.PersonaKind == UserPersonaLegacy || request.ContributionKind == UserContributionLegacy {
		return fmt.Errorf("legacy roleplay user-turn authority cannot be submitted")
	}
	if err := validateUserTurnPair(request.PersonaKind, request.CharacterID, request.ContributionKind); err != nil {
		return err
	}
	if request.ContributionKind == UserContributionCommand {
		if len(request.Parts) != 0 {
			return fmt.Errorf("roleplay command cannot carry turn-builder parts")
		}
		return validateUserTurnText(request.ContributionKind, exactText)
	}
	derived, composed, err := composeUserTurnParts(request.PersonaKind, request.Parts)
	if err != nil {
		return err
	}
	if derived != request.ContributionKind {
		return fmt.Errorf("roleplay contribution kind differs from its exact turn parts")
	}
	if exactText != composed {
		return fmt.Errorf("roleplay prompt differs from its exact ordered turn parts")
	}
	return validateUserTurnText(request.ContributionKind, exactText)
}

func (authority UserTurnAuthority) Validate() error {
	if err := validateUserTurnPair(
		authority.PersonaKind, authority.CharacterID, authority.ContributionKind,
	); err != nil {
		return err
	}
	if err := validateUserTurnText(authority.ContributionKind, authority.ExactText); err != nil {
		return err
	}
	if len(authority.Parts) != 0 {
		derived, composed, err := composeUserTurnParts(authority.PersonaKind, authority.Parts)
		if err != nil {
			return err
		}
		if derived != authority.ContributionKind || composed != authority.ExactText {
			return fmt.Errorf("roleplay user turn parts differ from persisted contribution authority")
		}
	} else if authority.ContributionKind == UserContributionNarrationDirection ||
		authority.ContributionKind == UserContributionStructured {
		return fmt.Errorf("combined narrator contribution requires exact turn parts")
	}
	switch authority.PersonaKind {
	case UserPersonaCharacter:
		if err := validateName(authority.PersonaName, "roleplay user persona name"); err != nil {
			return err
		}
		return validateSimulationText(
			"roleplay user persona summary", authority.PersonaSummary, MaxSimulationTextBytes, true,
		)
	case UserPersonaNarrator:
		if authority.PersonaName != NarratorPersonaName || authority.PersonaSummary != "" {
			return fmt.Errorf("narrator turn requires exact narrator presentation authority")
		}
	case UserPersonaLegacy:
		if authority.PersonaName != LegacyUserPersonaName || authority.PersonaSummary != "" {
			return fmt.Errorf("historical turn requires exact unattributed presentation authority")
		}
	}
	return nil
}

func (authority UserTurnAuthority) IsCharacter() bool {
	return authority.PersonaKind == UserPersonaCharacter
}

// OngoingActionContribution returns the exact typed user contribution only
// when its persisted modality can establish or change that character's
// current action. Code decides this from typed parts; prose is never scanned.
func (authority UserTurnAuthority) OngoingActionContribution() (string, bool, error) {
	if err := authority.Validate(); err != nil {
		return "", false, err
	}
	if !authority.IsCharacter() {
		return "", false, nil
	}
	actions := make([]string, 0, len(authority.Parts))
	for _, part := range authority.Parts {
		if part.Kind == UserTurnPartAction {
			actions = append(actions, "[Action]\n"+part.Text)
		}
	}
	if len(actions) == 0 {
		return "", false, nil
	}
	return strings.Join(actions, "\n\n"), true, nil
}

// CanonContribution returns only the typed portion of a user turn that can
// establish fictional state. Narrator directions are instructions to the
// response station, not fictional events, so code excludes them without
// asking a model to rediscover that already-typed distinction.
func (authority UserTurnAuthority) CanonContribution() (string, bool, error) {
	if err := authority.Validate(); err != nil {
		return "", false, err
	}
	switch authority.PersonaKind {
	case UserPersonaCharacter:
		return authority.ExactText, true, nil
	case UserPersonaNarrator:
		if authority.ContributionKind == UserContributionCommand ||
			authority.ContributionKind == UserContributionDirection {
			return "", false, nil
		}
		if authority.ContributionKind == UserContributionNarration && len(authority.Parts) == 0 {
			return authority.ExactText, true, nil
		}
		sections := make([]string, 0, len(authority.Parts))
		for _, part := range authority.Parts {
			switch part.Kind {
			case UserTurnPartAction:
				sections = append(sections, "[Action]\n"+part.Text)
			case UserTurnPartEvent:
				sections = append(sections, "[Event]\n"+part.Text)
			}
		}
		if len(sections) == 0 {
			return "", false, nil
		}
		return strings.Join(sections, "\n\n"), true, nil
	case UserPersonaLegacy:
		return "", false, nil
	default:
		return "", false, fmt.Errorf(
			"roleplay canon contribution has unsupported persona kind %q",
			authority.PersonaKind,
		)
	}
}

func (authority UserTurnAuthority) Equal(other UserTurnAuthority) bool {
	return authority.PersonaKind == other.PersonaKind &&
		authority.CharacterID == other.CharacterID &&
		authority.PersonaName == other.PersonaName &&
		authority.PersonaSummary == other.PersonaSummary &&
		authority.ContributionKind == other.ContributionKind &&
		authority.ExactText == other.ExactText && slices.Equal(authority.Parts, other.Parts)
}

func validateUserTurnSceneAuthority(
	authority UserTurnAuthority,
	participantIDs []string,
) error {
	if !authority.IsCharacter() {
		return nil
	}
	if !slices.Contains(participantIDs, authority.CharacterID) {
		return fmt.Errorf(
			"%w: selected user persona must be a current scene participant",
			ErrSimulationIllegal,
		)
	}
	return nil
}

func validateUserTurnPair(
	persona UserPersonaKind,
	characterID string,
	contribution UserContributionKind,
) error {
	switch persona {
	case UserPersonaCharacter:
		if err := validateIdentity(characterID, characterIdentity); err != nil {
			return fmt.Errorf("roleplay user persona: %w", err)
		}
		if contribution != UserContributionDialogue && contribution != UserContributionAction &&
			contribution != UserContributionActionDialogue && contribution != UserContributionStructured {
			return fmt.Errorf("character persona requires dialogue, action, action_dialogue, or structured_turn contribution")
		}
	case UserPersonaNarrator:
		if characterID != "" {
			return fmt.Errorf("narrator persona cannot carry a character identity")
		}
		if contribution != UserContributionNarration && contribution != UserContributionDirection &&
			contribution != UserContributionNarrationDirection &&
			contribution != UserContributionCommand {
			return fmt.Errorf("narrator persona requires narration, direction, combined narration/direction, or command contribution")
		}
	case UserPersonaLegacy:
		if characterID != "" || contribution != UserContributionLegacy {
			return fmt.Errorf("historical roleplay turn authority is contradictory")
		}
	default:
		return fmt.Errorf("roleplay user persona kind %q is unsupported", persona)
	}
	return nil
}

func ComposeUserTurn(request UserTurnRequest) (string, error) {
	if request.ContributionKind == UserContributionCommand {
		return "", fmt.Errorf("roleplay command text is not composed from turn parts")
	}
	derived, exact, err := composeUserTurnParts(request.PersonaKind, request.Parts)
	if err != nil {
		return "", err
	}
	if derived != request.ContributionKind {
		return "", fmt.Errorf("roleplay contribution kind differs from its exact turn parts")
	}
	return exact, nil
}

func composeUserTurnParts(
	persona UserPersonaKind,
	parts []UserTurnPart,
) (UserContributionKind, string, error) {
	if len(parts) < 1 || len(parts) > MaxUserTurnParts {
		return "", "", fmt.Errorf("roleplay turn builder requires 1..%d ordered parts", MaxUserTurnParts)
	}
	hasAction := false
	hasMessage := false
	hasEvent := false
	sections := make([]string, len(parts))
	for index, part := range parts {
		if len(part.Text) == 0 || len(part.Text) > MaxUserTurnBytes || !utf8.ValidString(part.Text) ||
			strings.IndexByte(part.Text, 0) >= 0 || strings.TrimSpace(part.Text) == "" || part.Text != strings.TrimSpace(part.Text) {
			return "", "", fmt.Errorf("roleplay turn part %d must contain exact trimmed nonblank UTF-8 text", index)
		}
		switch part.Kind {
		case UserTurnPartAction:
			hasAction = true
			sections[index] = "[Action]\n" + part.Text
		case UserTurnPartMessage:
			hasMessage = true
			sections[index] = "[Message]\n" + part.Text
		case UserTurnPartEvent:
			hasEvent = true
			sections[index] = "[Event]\n" + part.Text
		default:
			return "", "", fmt.Errorf("roleplay turn part %d has unsupported kind %q", index, part.Kind)
		}
	}
	exact := strings.Join(sections, "\n\n")
	if len(exact) > MaxUserTurnBytes {
		return "", "", fmt.Errorf("roleplay composed turn exceeds %d UTF-8 bytes", MaxUserTurnBytes)
	}
	switch persona {
	case UserPersonaCharacter:
		if hasEvent {
			return UserContributionStructured, exact, nil
		}
		if hasAction && hasMessage {
			return UserContributionActionDialogue, exact, nil
		}
		if hasAction {
			return UserContributionAction, exact, nil
		}
		return UserContributionDialogue, exact, nil
	case UserPersonaNarrator:
		if (hasAction || hasEvent) && hasMessage {
			return UserContributionNarrationDirection, exact, nil
		}
		if hasAction || hasEvent {
			return UserContributionNarration, exact, nil
		}
		return UserContributionDirection, exact, nil
	default:
		return "", "", fmt.Errorf("roleplay turn builder cannot use persona kind %q", persona)
	}
}

func validateUserTurnText(kind UserContributionKind, exactText string) error {
	if len(exactText) == 0 || len(exactText) > MaxUserTurnBytes || !utf8.ValidString(exactText) ||
		strings.IndexByte(exactText, 0) >= 0 || strings.TrimSpace(exactText) == "" {
		return fmt.Errorf("roleplay user turn must contain 1..%d nonblank UTF-8 bytes without NUL", MaxUserTurnBytes)
	}
	if kind == UserContributionLegacy {
		return nil
	}
	if kind == UserContributionCommand {
		if !strings.HasPrefix(exactText, "/") {
			return fmt.Errorf("roleplay command contribution must begin with a slash")
		}
	} else if strings.HasPrefix(exactText, "/") {
		return fmt.Errorf("slash input requires command contribution authority")
	}
	return nil
}
