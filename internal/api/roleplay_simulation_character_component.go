package api

import (
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

func renderRoleplayCharacterRoster(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Character setup", "Create characters and revisioned sheets for this fictional world."))
	body.WriteString(`<form data-action="submit->chat#createRoleplayCharacter" class="flex items-end gap-2 rounded-md border border-white/10 bg-zinc-900/40 p-3">` +
		`<div class="min-w-0 flex-1">` + roleplayTextInput("name", "New character name", "", "text", true) + `</div>` +
		roleplaySubmitButton("Create") + `</form>`)
	if len(state.Characters) == 0 {
		body.WriteString(chatEmptyState("No characters are available in this world."))
	}
	for _, character := range state.Characters {
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">`)
		body.WriteString(`<h5 class="text-sm font-semibold text-zinc-100">` + html.EscapeString(character.Name) + `</h5>`)
		if state.CharacterHasPersona[character.ID] {
			body.WriteString(`<p class="mt-2 text-[11px] text-emerald-200">Character sheet configured. Edit it in the paginated sheets below.</p>`)
			body.WriteString(renderRoleplaySceneDraftParticipantForm(state, character))
		} else {
			body.WriteString(renderRoleplayPersonaForm(character.ID, 0, roleplay.PersonaSheet{}))
		}
		body.WriteString(renderRoleplayResearchCapabilityForm(state, character))
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

func renderRoleplayResearchCapabilityForm(
	state roleplaySimulationComponentState,
	character roleplay.SimulationCharacterSummary,
) string {
	capability := state.CharacterCapabilities[character.ID]
	checked := ""
	if capability.WebResearch {
		checked = " checked"
	}
	var syntax string
	if capability.WebResearch {
		exact := `/research "question"`
		syntax = `<div class="mt-2 flex items-center justify-between gap-2"><code class="rounded border border-white/10 bg-zinc-950 px-2 py-1 text-[10px] text-violet-100">` +
			html.EscapeString(exact) + `</code><button type="button" data-action="chat#useRoleplayCommand" data-roleplay-command="` +
			html.EscapeString(exact) + `" class="rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-300 transition hover:border-violet-300/40">Place in composer</button></div>`
	}
	return `<form data-action="submit->chat#configureRoleplayResearch" data-character-id="` +
		html.EscapeString(character.ID) + `" data-characters-offset="` + strconv.Itoa(state.Page.Characters) +
		`" class="mt-3 rounded-md border border-white/10 bg-zinc-950/45 p-2.5">` +
		`<div class="flex items-center justify-between gap-2"><label class="flex items-center gap-2 text-[11px] text-zinc-300">` +
		`<input type="checkbox" name="enabled"` + checked + `><span>Allow web research</span></label>` +
		roleplaySubmitButton("Save access") + `</div>` + syntax + `</form>`
}

func renderRoleplayPersonaSheets(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Character sheets", "Persona edits require the exact current revision."))
	if len(state.Personas) == 0 {
		body.WriteString(chatEmptyState("No character sheets are configured on this page."))
	}
	for _, persona := range state.Personas {
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">`)
		body.WriteString(`<div class="flex items-start justify-between gap-2"><div><h5 class="text-sm font-semibold text-zinc-100">` +
			html.EscapeString(persona.Name) + `</h5></div><span class="rounded border border-white/10 px-2 py-1 text-[10px] text-zinc-400">revision ` +
			strconv.FormatInt(persona.Projection.Revision, 10) + `</span></div>`)
		body.WriteString(renderRoleplayPersonaForm(
			persona.Projection.CharacterID, persona.Projection.Revision, persona.Projection.Sheet,
		))
		body.WriteString(`</article>`)
	}
	pagination, err := renderRoleplayPagination(
		state, "personas", state.Page.Personas, len(state.Personas), state.PersonasMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func renderRoleplayPersonaForm(characterID string, revision int64, sheet roleplay.PersonaSheet) string {
	return `<form data-action="submit->chat#saveRoleplayPersona" data-character-id="` +
		html.EscapeString(characterID) + `" class="mt-3 space-y-2">` +
		`<input type="hidden" name="expected_revision" value="` + strconv.FormatInt(revision, 10) + `">` +
		roleplayTextArea("summary", "Summary", sheet.Summary, true) +
		roleplayTextInput("voice", "Voice", sheet.Voice, "text", false) +
		roleplayTextArea("traits", "Traits, one per line", strings.Join(sheet.Traits, "\n"), false) +
		roleplayTextArea("goals", "Goals, one per line", strings.Join(sheet.Goals, "\n"), false) +
		roleplaySubmitButton("Save character sheet") + `</form>`
}

func renderRoleplayInventory(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Inventory", "Items held by the current scene's active character."))
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
