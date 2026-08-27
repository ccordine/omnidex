package api

import (
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

func renderRoleplayCharacterRoster(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("World cast", "Choose scene participation here. Select a character in the sidebar to edit their sheet, model, and research access."))
	if len(state.Characters) == 0 {
		body.WriteString(chatEmptyState("No characters are available in this world."))
	}
	for _, character := range state.Characters {
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">`)
		body.WriteString(`<h5 class="text-sm font-semibold text-zinc-100">` + html.EscapeString(character.Name) + `</h5>`)
		if state.CharacterHasPersona[character.ID] {
			body.WriteString(`<p class="mt-2 text-[11px] text-emerald-200">Character sheet configured.</p>`)
			body.WriteString(renderRoleplaySceneDraftParticipantForm(state, character))
		} else {
			body.WriteString(`<p class="mt-2 text-[11px] text-amber-200">Character sheet required before adding this character to a scene.</p>`)
		}
		body.WriteString(`</article>`)
	}
	pagination, err := renderRoleplayPagination(
		state, "characters", state.Page.Characters, len(state.Characters), state.CharactersMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func renderRoleplaySceneDraftParticipantForm(
	state roleplaySimulationComponentState,
	character roleplay.SimulationCharacterSummary,
) string {
	selected := false
	position := 0
	for index, participant := range state.SceneDraft.Participants {
		if participant.CharacterID == character.ID {
			selected = true
			position = index + 1
			break
		}
	}
	checked := ""
	label := "Add to scene draft"
	if selected {
		checked = " checked"
		label = "Keep in scene draft"
	}
	positionText := "Not selected"
	if position > 0 {
		positionText = "Turn position " + strconv.Itoa(position)
	}
	return `<form data-action="submit->chat#saveRoleplaySceneDraftParticipant" data-character-id="` +
		html.EscapeString(character.ID) + `" data-draft-revision="` + strconv.FormatInt(state.SceneDraft.Revision, 10) +
		`" data-characters-offset="` + strconv.Itoa(state.Page.Characters) +
		`" class="mt-3 rounded-md border border-white/10 bg-zinc-950/45 p-2.5">` +
		`<div class="flex items-center justify-between gap-2"><label class="flex items-center gap-2 text-[11px] text-zinc-300">` +
		`<input type="checkbox" name="selected"` + checked + `><span>` + html.EscapeString(label) + `</span></label>` +
		`<span class="text-[10px] text-zinc-500">` + html.EscapeString(positionText) + `</span></div>` +
		`<div class="mt-2">` + roleplaySubmitButton("Save scene selection") + `</div></form>`
}

func renderRoleplayInventory(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Inventory", "Items held by the current initiative character."))
	if len(state.Inventory) == 0 {
		body.WriteString(chatEmptyState("The inventory is empty."))
	}
	for _, item := range state.Inventory {
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">`)
		body.WriteString(`<div class="flex items-start justify-between gap-2"><h5 class="text-sm font-semibold text-zinc-100">` +
			html.EscapeString(item.Name) + `</h5><span class="rounded border border-white/10 px-2 py-1 text-[10px] text-zinc-400">`)
		if item.UsePolicy == roleplay.ItemUseInfinite {
			body.WriteString(`unlimited`)
		} else {
			body.WriteString(strconv.Itoa(item.RemainingUses) + ` uses`)
		}
		body.WriteString(`</span></div><p class="mt-2 whitespace-pre-wrap text-xs text-zinc-300">` +
			html.EscapeString(item.Description) + `</p></article>`)
	}
	pagination, err := renderRoleplayPagination(
		state, "inventory", state.Page.Inventory, len(state.Inventory), state.InventoryMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func roleplaySectionStart(title, description string) string {
	return `<section class="space-y-2 rounded-md border border-white/10 bg-zinc-950/35 p-3"><div><h4 class="text-xs font-semibold uppercase tracking-[.12em] text-zinc-200">` +
		html.EscapeString(title) + `</h4><p class="mt-1 text-[11px] text-zinc-500">` + html.EscapeString(description) + `</p></div>`
}

func roleplayTextInput(name, label, value, inputType string, required bool) string {
	requiredAttribute := ""
	if required {
		requiredAttribute = " required"
	}
	return `<label class="block text-[11px] text-zinc-400"><span>` + html.EscapeString(label) +
		`</span><input name="` + html.EscapeString(name) + `" type="` + html.EscapeString(inputType) +
		`" value="` + html.EscapeString(value) + `"` + requiredAttribute +
		` class="mt-1 w-full rounded-md border border-white/10 bg-zinc-950 px-2.5 py-2 text-xs text-zinc-100 outline-none focus:border-violet-300/40"></label>`
}

func roleplayTextArea(name, label, value string, required bool) string {
	requiredAttribute := ""
	if required {
		requiredAttribute = " required"
	}
	return `<label class="block text-[11px] text-zinc-400"><span>` + html.EscapeString(label) +
		`</span><textarea name="` + html.EscapeString(name) + `" rows="2"` + requiredAttribute +
		` class="mt-1 w-full resize-y rounded-md border border-white/10 bg-zinc-950 px-2.5 py-2 text-xs text-zinc-100 outline-none focus:border-violet-300/40">` +
		html.EscapeString(value) + `</textarea></label>`
}

func roleplaySubmitButton(label string) string {
	return `<button type="submit" class="inline-flex items-center rounded-md border border-violet-300/30 bg-violet-300/10 px-2.5 py-1.5 text-[11px] font-semibold text-violet-100 transition hover:bg-violet-300/20 disabled:cursor-wait disabled:opacity-60">` +
		html.EscapeString(label) + `</button>`
}
