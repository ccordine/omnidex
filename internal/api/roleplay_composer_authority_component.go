package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

func renderRoleplayComposerAuthority(state roleplaySimulationComponentState) (string, error) {
	if state.Scene == nil {
		return `<p role="status" class="px-1 py-1 text-xs text-amber-200">Create a scene to begin.</p>`, nil
	}
	selected := reusableRoleplayComposerPersona(state)
	var options strings.Builder
	options.WriteString(`<option value="narrator" data-persona-kind="narrator"`)
	if selected == "narrator" {
		options.WriteString(` selected`)
	}
	options.WriteString(`>Narrator</option>`)
	for _, participant := range state.AllParticipants {
		if !roleplayComposerPersonaEligible(state, participant.CharacterID) {
			continue
		}
		options.WriteString(`<option value="` + html.EscapeString(participant.CharacterID) + `" data-persona-kind="character"`)
		if selected == participant.CharacterID {
			options.WriteString(` selected`)
		}
		options.WriteString(`>` + html.EscapeString(participant.Name) + `</option>`)
	}
	authority, err := renderRoleplayResponseAuthority(state)
	if err != nil {
		return "", err
	}
	return `<div data-roleplay-composer-configured="true" class="grid min-w-0 gap-1 px-1 pb-1">
  <div class="flex min-w-0 flex-wrap items-center gap-2">
    <label class="flex min-w-0 items-center gap-2 text-xs text-zinc-400">
      <span class="shrink-0">Acting as</span>
      <select data-chat-target="roleplayPersona" data-action="change->chat#roleplayPersonaChanged" aria-label="Acting as" class="min-w-0 max-w-52 rounded-lg border border-white/10 bg-zinc-950 px-2.5 py-1.5 text-xs font-semibold text-zinc-100 outline-none focus:border-violet-300/40">` + options.String() + `</select>
    </label>
    <button type="button" data-action="chat#showRoleplayPersonaCreator" aria-label="Create an identity" title="Create an identity" class="grid h-7 w-7 place-items-center rounded-lg border border-white/10 text-sm text-zinc-400 transition hover:border-violet-300/30 hover:text-violet-100">+</button>
    <span data-chat-target="roleplayPersonaCreator" class="hidden min-w-0 flex-1 items-center gap-1.5">
      <input data-chat-target="roleplayNewPersona" data-action="keydown->chat#roleplayPersonaCreatorKeydown" type="text" maxlength="256" placeholder="Identity name" aria-label="New roleplay identity name" class="min-w-32 flex-1 rounded-lg border border-white/10 bg-zinc-950 px-2.5 py-1.5 text-xs text-zinc-100 outline-none placeholder:text-zinc-600 focus:border-violet-300/40">
      <button type="button" data-action="chat#createRoleplayPersona" class="rounded-lg bg-violet-300 px-2.5 py-1.5 text-xs font-semibold text-zinc-950">Add</button>
      <button type="button" data-action="chat#hideRoleplayPersonaCreator" aria-label="Cancel identity creation" class="rounded-lg px-2 py-1.5 text-xs text-zinc-500 hover:text-zinc-200">Cancel</button>
    </span>
  </div>
  ` + authority + `
</div>`, nil
}

func reusableRoleplayComposerPersona(state roleplaySimulationComponentState) string {
	if selected := state.Page.ComposerPersonaCharacter; selected != "" {
		return selected
	}
	if state.LastUserTurn == nil || state.LastUserTurn.PersonaKind != roleplay.UserPersonaCharacter {
		return "narrator"
	}
	if roleplayComposerPersonaEligible(state, state.LastUserTurn.CharacterID) {
		return state.LastUserTurn.CharacterID
	}
	return "narrator"
}

func roleplayComposerPersonaEligible(state roleplaySimulationComponentState, characterID string) bool {
	if state.Scene == nil || characterID == "" || len(state.AllParticipants) < 2 {
		return false
	}
	for _, participant := range state.AllParticipants {
		if participant.CharacterID == characterID {
			return true
		}
	}
	return false
}

func validateRequestedRoleplayComposerPersona(state roleplaySimulationComponentState) error {
	selected := state.Page.ComposerPersonaCharacter
	if selected == "" {
		return nil
	}
	if !roleplayComposerPersonaEligible(state, selected) {
		return fmt.Errorf("%w: requested composer persona is not eligible in the current scene",
			roleplay.ErrSimulationIllegal)
	}
	for _, character := range state.UserPersonaCharacters {
		if character.ID == selected {
			return nil
		}
	}
	return fmt.Errorf("%w: requested composer persona is absent from world authority",
		roleplay.ErrSimulationIllegal)
}

func renderRoleplayResponseAuthority(state roleplaySimulationComponentState) (string, error) {
	if state.Scene == nil || state.ActiveGeneration == nil || state.ActiveCharacterName == "" {
		return "", fmt.Errorf("roleplay composer requires exact scene initiative authority")
	}
	if err := state.Scene.Initiative.Validate(); err != nil {
		return "", fmt.Errorf("roleplay composer initiative authority: %w", err)
	}
	var authority strings.Builder
	authority.WriteString(`<p data-roleplay-response-authority class="truncate text-[11px] text-zinc-500" title="Current initiative authority">Scene: <strong class="font-medium text-zinc-300">`)
	authority.WriteString(html.EscapeString(state.Scene.Title))
	authority.WriteString(`</strong> <span aria-hidden="true">·</span> Initiative: <strong class="font-medium text-violet-200">`)
	authority.WriteString(html.EscapeString(state.ActiveCharacterName))
	authority.WriteString(`</strong>`)
	authority.WriteString(fmt.Sprintf(
		` <span aria-hidden="true">·</span> <span data-roleplay-authority-initiative>Round %d <span aria-hidden="true">·</span> Turn %d <span aria-hidden="true">·</span> Time %d</span>`,
		state.Scene.Initiative.Round, state.Scene.Initiative.Turn,
		state.Scene.Initiative.FictionalTimeTick,
	))
	authority.WriteString(`</p>`)
	return authority.String(), nil
}
