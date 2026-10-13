package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/chatcommands"
)

// ConversationCLICommandReference binds one opaque instruction reference to
// its code-owned CLI meaning. Command names never appear in model text.
type ConversationCLICommandReference struct {
	Token   string            `json:"token"`
	Command chatcommands.Name `json:"command"`
}

func validateConversationCLICommands(input ConversationResponseInput) error {
	if len(input.CLICommands) == 0 {
		return nil
	}
	if input.Kind != ObjectiveKindAnswer || input.RoleplayIdentity != nil {
		return fmt.Errorf("CLI command facts require an assistant answer")
	}
	if len(input.CLICommands) > chatcommands.Count() {
		return fmt.Errorf("CLI command facts exceed the registered command bound")
	}
	seenTokens := make(map[string]struct{}, len(input.CLICommands))
	seenCommands := make(map[chatcommands.Name]struct{}, len(input.CLICommands))
	for _, reference := range input.CLICommands {
		if !opaqueArtifactPattern.MatchString(reference.Token) ||
			!conversationInstructionMentionsToken(input.ExactInstruction, reference.Token) {
			return fmt.Errorf("CLI command fact requires an exact opaque instruction reference")
		}
		if _, exists := chatcommands.Lookup(reference.Command); !exists {
			return fmt.Errorf("CLI command fact names an unregistered command")
		}
		if _, duplicate := seenTokens[reference.Token]; duplicate {
			return fmt.Errorf("CLI command fact repeats an instruction reference")
		}
		if _, duplicate := seenCommands[reference.Command]; duplicate {
			return fmt.Errorf("CLI command fact repeats a registered command")
		}
		seenTokens[reference.Token] = struct{}{}
		seenCommands[reference.Command] = struct{}{}
	}
	return nil
}

func conversationInstructionMentionsToken(instruction, token string) bool {
	for _, span := range opaqueArtifactMentionPattern.FindAllStringIndex(instruction, -1) {
		if instruction[span[0]:span[1]] == token && artifactTokenBoundary(instruction, span[0], span[1]) {
			return true
		}
	}
	return false
}

func renderConversationCLICommands(input ConversationResponseInput) (string, error) {
	meanings := make([]string, 0, len(input.CLICommands))
	for _, reference := range input.CLICommands {
		definition, exists := chatcommands.Lookup(reference.Command)
		if !exists {
			return "", fmt.Errorf("CLI command fact names an unregistered command")
		}
		meanings = append(meanings, reference.Token+": "+definition.Meaning)
	}
	return strings.Join(meanings, "\n"), nil
}
