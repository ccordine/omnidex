package roleplay

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxOngoingActionBytes = 512

const AuthoritySimulationState AuthorityNamespace = "SIMULATION_STATE"

type OngoingActionSourceKind string

const (
	OngoingActionSourceResponse   OngoingActionSourceKind = "response"
	OngoingActionSourceUserAction OngoingActionSourceKind = "user_action"

	UserActionOngoingActionSourcePosition = -1
)

var ongoingActionStateIdentity = identityKind{
	name:    "roleplay ongoing-action state",
	pattern: regexp.MustCompile(`^rpo_[0-9a-f]{32}$`),
}

type OngoingActionState struct {
	ID                        string
	Ordinal                   int64
	WorldID                   string
	CharacterID               string
	SourceCompletionOperation string
	SourceKind                OngoingActionSourceKind
	SourcePosition            int
	SourceMessageID           int64
	OngoingAction             *string
	Authority                 AuthorityNamespace
	CreatedAt                 time.Time
}

type OngoingActionResolution struct {
	CompletionOperation   string
	SourceKind            OngoingActionSourceKind
	SourcePosition        int
	WorldID               string
	CharacterID           string
	SourceMessageID       int64
	PreviousStateID       *string
	CurrentStateID        *string
	PreviousOngoingAction *string
	OngoingAction         *string
	Changed               bool
	Authority             AuthorityNamespace
	CreatedAt             time.Time
}

type NarrativeOngoingAction struct {
	CharacterName string `json:"character_name"`
	Action        string `json:"action"`
}

// ValidateOngoingActionText validates one complete current-action semantic
// leaf. An absent current action is represented separately, never by blank
// text.
func ValidateOngoingActionText(value string) error {
	if value == "" || value != strings.TrimSpace(value) ||
		len(value) > MaxOngoingActionBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') {
		return fmt.Errorf(
			"roleplay ongoing action must be 1 to %d trimmed UTF-8 bytes without NUL",
			MaxOngoingActionBytes,
		)
	}
	return nil
}
