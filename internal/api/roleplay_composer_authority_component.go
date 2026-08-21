package api

import (
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
	for _, character := range state.UserPersonaCharacters {
		options.WriteString(`<option value="` + html.EscapeString(character.ID) + `" data-persona-kind="character"`)
		if selected == character.ID {
			options.WriteString(` selected`)
		}
		options.WriteString(`>` + html.EscapeString(character.Name) + `</option>`)
	}
	return `<div data-roleplay-composer-configured="true" class="flex min-w-0 flex-wrap items-center gap-2 px-1 pb-1">
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
</div>`, nil
}

func reusableRoleplayComposerPersona(state roleplaySimulationComponentState) string {
	if selected := state.Page.ComposerPersonaCharacter; selected != "" {
		return selected
	}
	if state.LastUserTurn == nil || state.LastUserTurn.PersonaKind != roleplay.UserPersonaCharacter {
		return "narrator"
	}
	for _, character := range state.UserPersonaCharacters {
		if character.ID == state.LastUserTurn.CharacterID {
			return character.ID
		}
	}
	return "narrator"
}
