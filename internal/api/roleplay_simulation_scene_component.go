package api

import (
	"html"
	"strconv"
	"strings"
)

func renderRoleplaySceneSheet(state roleplaySimulationComponentState) string {
	scene := *state.Scene
	fields, selection := renderRoleplaySceneDraftSelection(
		state, "Select at least one character from the paginated roster before saving.",
	)
	return roleplaySectionStart("Scene sheet", "The current scene and revision are owned by the server.") +
		`<form data-action="submit->chat#updateRoleplayScene" data-scene-revision="` +
		strconv.FormatInt(scene.Revision, 10) + `" class="space-y-2 rounded-md border border-white/10 bg-zinc-900/60 p-3">` +
		`<input type="hidden" name="expected_revision" value="` + strconv.FormatInt(scene.Revision, 10) + `">` +
		`<input type="hidden" name="expected_draft_revision" value="` + strconv.FormatInt(state.SceneDraft.Revision, 10) + `">` +
		fields +
		`<div class="flex items-start justify-between gap-2"><h5 class="text-sm font-semibold text-zinc-100">` +
		html.EscapeString(scene.Title) + `</h5><span class="rounded border border-white/10 px-2 py-1 text-[10px] text-zinc-400">revision ` +
		strconv.FormatInt(scene.Revision, 10) + `</span></div>` +
		roleplayTextInput("title", "Scene title", scene.Title, "text", true) +
		roleplayTextArea("description", "Scene description", scene.Description, true) +
		`<div class="space-y-2"><p class="text-[11px] text-zinc-400">Draft turn order · ` +
		strconv.Itoa(len(state.SceneDraft.Participants)) + ` selected</p><ol class="space-y-2">` + selection + `</ol></div>` +
		`<p class="mt-2 text-[11px] text-violet-200">Current turn: ` + html.EscapeString(state.ActiveCharacterName) + `</p>` +
		roleplaySubmitButton("Save scene") + `</form></section>`
}

func renderRoleplaySceneForm(state roleplaySimulationComponentState) string {
	fields, selection := renderRoleplaySceneDraftSelection(
		state, "Add characters from the paginated roster before creating a scene.",
	)
	return `<form data-action="submit->chat#createRoleplayScene"` +
		` class="space-y-2 rounded-md border border-white/10 bg-zinc-900/40 p-3">` +
		`<input type="hidden" name="expected_draft_revision" value="` + strconv.FormatInt(state.SceneDraft.Revision, 10) + `">` +
		fields +
		roleplayTextInput("title", "Scene title", "", "text", true) +
		roleplayTextArea("description", "Scene description", "", true) +
		`<div class="space-y-2"><p class="text-[11px] text-zinc-400">Server draft turn order · ` +
		strconv.Itoa(len(state.SceneDraft.Participants)) + ` selected</p><ol class="space-y-2">` + selection + `</ol></div>` +
		roleplaySubmitButton("Create scene") + `</form>`
}

func renderRoleplaySceneDraftSelection(
	state roleplaySimulationComponentState,
	emptyMessage string,
) (string, string) {
	var fields strings.Builder
	var selection strings.Builder
	for index, participant := range state.SceneDraft.Participants {
		name := state.CharacterNames[participant.CharacterID]
		fields.WriteString(`<input type="hidden" name="participant_id" value="` +
			html.EscapeString(participant.CharacterID) + `">`)
		selection.WriteString(`<li class="rounded-md border border-white/10 bg-zinc-950/50 px-2.5 py-2 text-xs text-zinc-200">` +
			`<span class="mr-2 text-zinc-500">` + strconv.Itoa(index+1) + `.</span>` + html.EscapeString(name) + `</li>`)
	}
	if len(state.SceneDraft.Participants) == 0 {
		selection.WriteString(chatEmptyState(emptyMessage))
	}
	return fields.String(), selection.String()
}

func renderRoleplayTurnOrder(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Turn order", "The active participant advances only after a verified turn."))
	body.WriteString(`<ol class="space-y-2" start="` + strconv.Itoa(state.Page.TurnOrder+1) + `">`)
	for _, participant := range state.Participants {
		active := participant.CharacterID == state.Scene.ActiveCharacterID
		className := "border-white/10 bg-zinc-900/60 text-zinc-300"
		status := "waiting"
		if active {
			className = "border-violet-300/30 bg-violet-300/10 text-violet-100"
			status = "active"
		}
		body.WriteString(`<li class="flex items-center justify-between gap-2 rounded-md border px-3 py-2 ` + className + `">` +
			`<span class="text-xs"><span class="mr-2 text-zinc-500">` + strconv.Itoa(participant.TurnPosition+1) + `.</span>` +
			html.EscapeString(participant.Name) + `</span><span class="text-[10px] uppercase tracking-wide">` + status + `</span></li>`)
	}
	body.WriteString(`</ol>`)
	pagination, err := renderRoleplayPagination(
		state, "turn-order", state.Page.TurnOrder, len(state.Participants), state.ParticipantsMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func renderRoleplayMeters(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Meters", "Values shown here belong to the current scene's active character."))
	if len(state.Meters) == 0 {
		body.WriteString(chatEmptyState("No meters are configured."))
	}
	for _, meter := range state.Meters {
		body.WriteString(`<form data-action="submit->chat#setRoleplayMeter" data-character-id="` +
			html.EscapeString(state.Scene.ActiveCharacterID) + `" data-meter-key="` + html.EscapeString(meter.Key) +
			`" data-meter-revision="` + strconv.FormatInt(meter.Revision, 10) +
			`" class="rounded-md border border-white/10 bg-zinc-900/60 p-3">`)
		body.WriteString(`<div class="flex items-center justify-between gap-2"><label class="text-xs font-semibold text-zinc-100">` +
			html.EscapeString(meter.Name) + `</label><span class="text-[10px] text-zinc-500">` +
			strconv.Itoa(meter.Minimum) + ` – ` + strconv.Itoa(meter.Maximum) + `</span></div>`)
		body.WriteString(`<div class="mt-2 flex items-center gap-2"><input name="value" type="number" required min="` +
			strconv.Itoa(meter.Minimum) + `" max="` + strconv.Itoa(meter.Maximum) + `" value="` +
			strconv.Itoa(meter.Value) + `" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-950 px-2.5 py-1.5 text-xs text-zinc-100">` +
			roleplaySubmitButton("Set") + `</div></form>`)
	}
	pagination, err := renderRoleplayPagination(state, "meters", state.Page.Meters, len(state.Meters), state.MetersMore)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}
