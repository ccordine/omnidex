package api

import (
	"html"
	"strconv"
	"strings"
)

func renderRoleplayCastSidebar(state roleplaySimulationComponentState) (string, error) {
	sceneRevision := int64(0)
	if state.Scene != nil {
		sceneRevision = state.Scene.Revision
	}
	enabled := make(map[string]int, len(state.AllParticipants))
	for index, participant := range state.AllParticipants {
		enabled[participant.CharacterID] = index
	}
	ordered := make([]string, len(state.AllParticipants))
	for index, participant := range state.AllParticipants {
		ordered[index] = renderRoleplayCastCharacter(
			participant.CharacterID, participant.Name, true, index, sceneRevision, true,
		)
	}
	for _, character := range state.UserPersonaCharacters {
		if _, exists := enabled[character.ID]; exists {
			continue
		}
		ordered = append(ordered, renderRoleplayCastCharacter(
			character.ID, character.Name, false, -1, sceneRevision, state.Scene != nil,
		))
	}
	if len(ordered) == 0 {
		ordered = append(ordered, `<li class="px-2 py-3 text-xs text-zinc-500">No characters are in this world.</li>`)
	}
	setupHint := ""
	if state.Scene == nil {
		setupHint = `<p class="shrink-0 border-b border-white/10 px-3 py-2 text-[10px] leading-4 text-amber-200/80">Create a scene to enable responders. Character editing is available now.</p>`
	}
	return `<div class="flex h-full min-h-0 flex-col" data-roleplay-scene-revision="` +
		strconv.FormatInt(sceneRevision, 10) + `">
  <div class="flex shrink-0 items-center justify-between border-b border-white/10 px-3 py-2">
    <span class="text-[10px] font-semibold uppercase tracking-[.16em] text-zinc-500">Characters</span>
    <button type="button" data-action="chat#openRoleplayWorldSetup" data-roleplay-setup-section="scene" aria-label="Scene settings" title="Scene settings" class="rounded px-1.5 py-1 text-xs text-zinc-600 hover:text-violet-200">⚙</button>
  </div>
  ` + setupHint + `
  <ul data-roleplay-cast-list class="scrollbar min-h-0 flex-1 space-y-1 overflow-y-auto p-2">` + strings.Join(ordered, "") + `</ul>
</div>`, nil
}

func renderRoleplayCastCharacter(
	characterID string,
	name string,
	isEnabled bool,
	position int,
	revision int64,
	canToggle bool,
) string {
	enabled := strconv.FormatBool(isEnabled)
	positionValue := ""
	if isEnabled {
		positionValue = strconv.Itoa(position)
	}
	checked := `<span aria-hidden="true" class="grid h-4 w-4 place-items-center rounded border border-white/15 text-[10px] text-transparent">✓</span>`
	rowClass := "text-zinc-500"
	if isEnabled {
		checked = `<span aria-hidden="true" class="grid h-4 w-4 place-items-center rounded border border-violet-300/30 bg-violet-300/15 text-[10px] text-violet-100">✓</span>`
		rowClass = "text-zinc-200"
	}
	escapedName := html.EscapeString(name)
	dragHandle := `<button type="button" draggable="false" disabled aria-label="Reordering is available after scene creation" class="cursor-not-allowed px-1 py-1 text-zinc-800">⋮⋮</button>`
	toggleAction := ` disabled`
	if canToggle {
		toggleAction = ` data-action="chat#toggleRoleplayResponder"`
	}
	if isEnabled && canToggle {
		dragHandle = `<button type="button" draggable="true" data-action="dragstart->chat#roleplayResponderDragStart dragend->chat#roleplayResponderDragEnd" aria-label="Reorder ` + escapedName + `" title="Drag to reorder" class="cursor-grab px-1 py-1 text-zinc-700 hover:text-zinc-400 active:cursor-grabbing">⋮⋮</button>`
	}
	return `<li data-roleplay-cast-character="` + html.EscapeString(characterID) +
		`" data-roleplay-cast-enabled="` + enabled + `" data-roleplay-cast-position="` + positionValue +
		`" data-action="dragover->chat#roleplayResponderDragOver drop->chat#roleplayResponderDrop" class="group flex items-center gap-1 rounded-lg border border-transparent px-1 py-1 transition hover:border-white/10 hover:bg-white/[.025] ` + rowClass + `">
  ` + dragHandle + `
  <button type="button"` + toggleAction + ` data-roleplay-character-id="` + html.EscapeString(characterID) +
		`" data-roleplay-enabled="` + enabled + `" data-roleplay-scene-revision="` + strconv.FormatInt(revision, 10) +
		`" aria-pressed="` + enabled + `" aria-label="Toggle ` + escapedName + ` as a responder" class="grid h-7 w-7 shrink-0 place-items-center rounded disabled:cursor-not-allowed disabled:opacity-40">` + checked + `</button>
  <button type="button" data-action="chat#openRoleplayCharacterEditor" data-roleplay-character-id="` + html.EscapeString(characterID) +
		`" aria-label="Edit ` + escapedName + `" class="min-w-0 flex-1 truncate rounded px-1 py-1.5 text-left text-xs font-medium hover:text-violet-100">` + escapedName + `</button>
</li>`
}
