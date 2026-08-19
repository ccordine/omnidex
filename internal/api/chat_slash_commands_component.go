package api

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	chatSlashCommandOptionsTarget  = "slash-command-options"
	chatSlashCommandListID         = "slash-command-list"
	maxChatSlashCommandBundleBytes = 1024 * 1024
)

var chatSlashCommandOptionIDPattern = regexp.MustCompile(`^slash-command-option-[0-9a-f]{16}$`)

type chatSlashCommandsComponent struct {
	ChannelID    model.ChannelID   `json:"channel_id"`
	CommandCount int               `json:"command_count"`
	HTML         chatComponentHTML `json:"html"`
}

func renderChatSlashCommandsComponent(
	channel model.Channel,
	projection *roleplay.SimulationSlashCommandProjection,
) (chatSlashCommandsComponent, error) {
	if err := channel.ValidateStored(); err != nil {
		return chatSlashCommandsComponent{}, fmt.Errorf("slash command channel: %w", err)
	}
	if channel.Scope != model.ChannelScopeUser {
		return chatSlashCommandsComponent{}, fmt.Errorf("slash commands require a user conversation")
	}
	commands := []roleplay.SimulationSlashCommand{}
	switch channel.Mode {
	case model.ChannelModeAssistant:
		if projection != nil {
			return chatSlashCommandsComponent{}, fmt.Errorf("assistant slash commands cannot carry roleplay authority")
		}
	case model.ChannelModeRoleplay:
		if projection == nil {
			return chatSlashCommandsComponent{}, fmt.Errorf("roleplay slash commands require simulation authority")
		}
		if projection.Schema != roleplay.SimulationSlashCommandProjectionSchemaV1 ||
			projection.WorldID == "" || projection.SceneID == "" || projection.SceneRevision < 1 ||
			projection.ActiveCharacterID == "" || projection.ActiveCharacterName == "" {
			return chatSlashCommandsComponent{}, fmt.Errorf("roleplay slash command projection authority is invalid")
		}
		commands = projection.Commands
	default:
		return chatSlashCommandsComponent{}, fmt.Errorf("slash command channel mode is unsupported")
	}
	if len(commands) > roleplay.MaxSimulationSlashCommands {
		return chatSlashCommandsComponent{}, fmt.Errorf(
			"slash command component exceeds its %d-option bound", roleplay.MaxSimulationSlashCommands,
		)
	}

	var markup strings.Builder
	markup.WriteString(`<div id="` + chatSlashCommandListID + `" role="listbox" aria-label="Available slash commands" data-slash-command-list data-slash-command-channel-id="` +
		html.EscapeString(string(channel.ID)) + `" class="max-h-72 space-y-1 overflow-y-auto p-1">`)
	seenIDs := make(map[string]struct{}, len(commands))
	seenExact := make(map[string]struct{}, len(commands))
	for index, command := range commands {
		if err := validateChatSlashCommand(command, index); err != nil {
			return chatSlashCommandsComponent{}, err
		}
		if _, duplicate := seenIDs[command.ID]; duplicate {
			return chatSlashCommandsComponent{}, fmt.Errorf("slash command component repeats an option identity")
		}
		if _, duplicate := seenExact[command.Insertion]; duplicate {
			return chatSlashCommandsComponent{}, fmt.Errorf("slash command component repeats exact syntax")
		}
		seenIDs[command.ID] = struct{}{}
		seenExact[command.Insertion] = struct{}{}
		prefix := "/" + command.Key
		markup.WriteString(`<button id="` + html.EscapeString(command.ID) +
			`" type="button" role="option" aria-selected="false" tabindex="-1" data-chat-slash-option data-action="chat#chooseSlashCommand" data-slash-command="` +
			html.EscapeString(command.Insertion) + `" data-slash-command-cursor="` + strconv.Itoa(command.CursorUTF16) +
			`" data-slash-command-prefix="` + html.EscapeString(prefix) + `" data-slash-command-kind="` +
			html.EscapeString(string(command.Kind)) + `" data-slash-command-key="` + html.EscapeString(command.Key) +
			`" data-slash-command-order="` + strconv.Itoa(command.DisplayOrder) +
			`" class="flex w-full items-start gap-3 rounded-md px-3 py-2 text-left text-xs text-zinc-300 outline-none transition hover:bg-white/[.06] focus:bg-violet-300/10 focus:text-violet-50 aria-selected:bg-violet-300/10 aria-selected:text-violet-50">` +
			`<code class="min-w-[9rem] shrink-0 text-violet-200">` + html.EscapeString(command.Display) + `</code>` +
			`<span class="min-w-0"><span class="block font-semibold text-zinc-100">` + html.EscapeString(command.Label) + `</span>` +
			`<span class="mt-0.5 block text-[11px] leading-4 text-zinc-500">` + html.EscapeString(command.Description) + `</span></span></button>`)
	}
	emptyText := "No available slash commands match this prefix."
	emptyHidden := " hidden"
	if len(commands) == 0 {
		emptyHidden = ""
		if channel.Mode == model.ChannelModeAssistant {
			emptyText = "No slash commands are available for this assistant conversation."
		} else {
			emptyText = "No slash commands are currently available for the active character."
		}
	}
	markup.WriteString(`<p data-slash-command-no-match role="status"` + emptyHidden +
		` class="rounded-md px-3 py-4 text-center text-xs text-zinc-500">` + html.EscapeString(emptyText) + `</p></div>`)
	if markup.Len() > maxChatSlashCommandBundleBytes {
		return chatSlashCommandsComponent{}, fmt.Errorf(
			"slash command component exceeds its %d-byte render bound", maxChatSlashCommandBundleBytes,
		)
	}

	return chatSlashCommandsComponent{
		ChannelID: channel.ID, CommandCount: len(commands),
		HTML: chatComponentHTML{Bundle: renderRecyclrTemplateHTML(
			chatSlashCommandOptionsTarget, markup.String(), "innerHTML",
		)},
	}, nil
}

func validateChatSlashCommand(command roleplay.SimulationSlashCommand, index int) error {
	if !chatSlashCommandOptionIDPattern.MatchString(command.ID) {
		return fmt.Errorf("slash command option %d has invalid code-issued identity", index)
	}
	if !roleplaySimulationKeyPattern.MatchString(command.Key) || command.DisplayOrder != index {
		return fmt.Errorf("slash command option %d has invalid key or order", index)
	}
	switch command.Kind {
	case roleplay.SimulationSlashCommandInteraction, roleplay.SimulationSlashCommandGive,
		roleplay.SimulationSlashCommandTake, roleplay.SimulationSlashCommandResearch:
	default:
		return fmt.Errorf("slash command option %d has unsupported kind", index)
	}
	for name, value := range map[string]string{
		"insertion syntax": command.Insertion, "display syntax": command.Display,
		"label": command.Label, "description": command.Description,
	} {
		if err := validateChatText(value, "slash command "+name, roleplay.MaxSimulationTextBytes+64); err != nil {
			return fmt.Errorf("slash command option %d: %w", index, err)
		}
	}
	prefix := "/" + command.Key
	if command.Insertion != prefix && !strings.HasPrefix(command.Insertion, prefix+" ") {
		return fmt.Errorf("slash command option %d syntax does not match its key", index)
	}
	encodedInsertion := utf16.Encode([]rune(command.Insertion))
	maximumCursor := len(encodedInsertion)
	if command.CursorUTF16 < 0 || command.CursorUTF16 > maximumCursor {
		return fmt.Errorf("slash command option %d has invalid UTF-16 cursor authority", index)
	}
	if command.CursorUTF16 > 0 && command.CursorUTF16 < maximumCursor &&
		utf16.IsSurrogate(rune(encodedInsertion[command.CursorUTF16-1])) &&
		utf16.IsSurrogate(rune(encodedInsertion[command.CursorUTF16])) {
		return fmt.Errorf("slash command option %d cursor splits a UTF-16 surrogate pair", index)
	}
	if err := validateChatSlashCommandSyntax(command, maximumCursor); err != nil {
		return fmt.Errorf("slash command option %d: %w", index, err)
	}
	return nil
}

func validateChatSlashCommandSyntax(command roleplay.SimulationSlashCommand, insertionLength int) error {
	prefix := "/" + command.Key
	switch command.Kind {
	case roleplay.SimulationSlashCommandGive, roleplay.SimulationSlashCommandTake:
		expectedKey := string(roleplay.SimulationActionGive)
		expectedKind := roleplay.SimulationActionGive
		if command.Kind == roleplay.SimulationSlashCommandTake {
			expectedKey = string(roleplay.SimulationActionTake)
			expectedKind = roleplay.SimulationActionTake
		}
		action, err := roleplay.ParseSimulationAction(command.Insertion)
		if command.Key != expectedKey || err != nil || action.Kind != expectedKind ||
			!action.HasArgument || command.Display != command.Insertion || command.CursorUTF16 != insertionLength {
			return fmt.Errorf("%s command does not match its canonical item-action contract", command.Kind)
		}
	case roleplay.SimulationSlashCommandResearch:
		if command.Key != "research" || command.Insertion != `/research ""` ||
			command.Display != `/research "…"` || command.CursorUTF16 != 11 {
			return fmt.Errorf("research command does not match its bounded question-template contract")
		}
	case roleplay.SimulationSlashCommandInteraction:
		if command.Key == "give" || command.Key == "take" || command.Key == "research" {
			return fmt.Errorf("interaction command uses a reserved key")
		}
		if command.Insertion == prefix {
			if command.Display != prefix || command.CursorUTF16 != insertionLength {
				return fmt.Errorf("argument-free interaction command has inconsistent display or cursor")
			}
			return nil
		}
		expectedInsertion := prefix + ` ""`
		expectedDisplay := prefix + ` "…"`
		expectedCursor := len(utf16.Encode([]rune(strings.TrimSuffix(expectedInsertion, `"`))))
		if command.Insertion != expectedInsertion || command.Display != expectedDisplay ||
			command.CursorUTF16 != expectedCursor {
			return fmt.Errorf("required-argument interaction command has inconsistent template authority")
		}
	}
	return nil
}
