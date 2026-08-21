package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelref"
	"github.com/gryph/omnidex/internal/roleplay"
)

const roleplayCharacterEditorTarget = "roleplay-character-editor"

type roleplayCharacterEditorState struct {
	Channel             model.Channel
	World               roleplay.World
	Character           roleplay.CharacterProjection
	Persona             roleplay.PersonaProjection
	PersonaConfigured   bool
	Capability          roleplay.CharacterCapabilityProjection
	Generation          roleplay.CharacterGenerationProjection
	InstalledModelNames []string
}

type roleplayCharacterEditorResponse struct {
	ChannelID   model.ChannelID   `json:"channel_id"`
	WorldID     string            `json:"world_id"`
	CharacterID string            `json:"character_id"`
	HTML        chatComponentHTML `json:"html"`
}

func renderRoleplayCharacterEditor(
	state roleplayCharacterEditorState,
) (roleplayCharacterEditorResponse, error) {
	if err := validateRoleplayCharacterEditorState(state); err != nil {
		return roleplayCharacterEditorResponse{}, err
	}
	characterID := state.Character.CharacterID
	name := html.EscapeString(state.Character.CharacterName)
	personaRevision := int64(0)
	personaSheet := roleplay.PersonaSheet{}
	personaStatus := `<span class="text-[10px] text-amber-200">Sheet required</span>`
	if state.PersonaConfigured {
		personaRevision = state.Persona.Revision
		personaSheet = state.Persona.Sheet
		personaStatus = `<span class="text-[10px] text-emerald-200">Revision ` + strconv.FormatInt(personaRevision, 10) + `</span>`
	}
	body := `<div data-roleplay-character-editor-character="` + html.EscapeString(characterID) + `" class="space-y-3">
  <header class="flex items-start justify-between gap-3">
    <div class="min-w-0"><p class="text-[10px] font-semibold uppercase tracking-[.16em] text-violet-200/80">Character</p><h3 class="mt-1 truncate text-base font-semibold text-zinc-100">` + name + `</h3></div>` + personaStatus + `
  </header>
  <section class="rounded-xl border border-white/10 bg-white/[.025] p-3">
    <h4 class="text-xs font-semibold text-zinc-200">Persona</h4>
    <p class="mt-1 text-[11px] text-zinc-500">Shared identity, voice, traits, and goals.</p>` +
		renderRoleplayCharacterEditorPersonaForm(characterID, personaRevision, personaSheet) + `
  </section>
  <section class="rounded-xl border border-white/10 bg-white/[.025] p-3">
    <h4 class="text-xs font-semibold text-zinc-200">Response model</h4>
    <p class="mt-1 text-[11px] text-zinc-500">Changes apply to this character's next response.</p>` +
		renderRoleplayCharacterEditorGenerationForm(characterID, name, state.Generation.Config, state.InstalledModelNames) + `
  </section>
  <section class="rounded-xl border border-white/10 bg-white/[.025] p-3">
    <h4 class="text-xs font-semibold text-zinc-200">Research</h4>` +
		renderRoleplayCharacterEditorResearchForm(characterID, state.Capability.WebResearch) + `
  </section>
</div>`
	return roleplayCharacterEditorResponse{
		ChannelID: state.Channel.ID, WorldID: state.World.ID, CharacterID: characterID,
		HTML: chatComponentHTML{Bundle: renderRecyclrTemplateHTML(roleplayCharacterEditorTarget, body, "innerHTML")},
	}, nil
}

func validateRoleplayCharacterEditorState(state roleplayCharacterEditorState) error {
	if err := state.Channel.ValidateStored(); err != nil {
		return fmt.Errorf("roleplay character editor channel: %w", err)
	}
	if state.Channel.Mode != model.ChannelModeRoleplay || state.World.ChannelID != string(state.Channel.ID) ||
		state.World.Authority != roleplay.AuthorityFictionalCanon {
		return fmt.Errorf("roleplay character editor world does not match channel authority")
	}
	if err := state.Character.Validate(); err != nil {
		return fmt.Errorf("roleplay character editor projection: %w", err)
	}
	if state.Character.WorldID != state.World.ID {
		return fmt.Errorf("roleplay character editor character does not belong to selected world")
	}
	characterID := state.Character.CharacterID
	if state.PersonaConfigured {
		if state.Persona.CharacterID != characterID || state.Persona.Revision < 1 {
			return fmt.Errorf("roleplay character editor persona authority is invalid")
		}
		revision := state.Persona.Revision
		if err := validateRoleplayPersonaRequest(roleplayPersonaRequest{
			ExpectedRevision: &revision, Summary: state.Persona.Sheet.Summary,
			Voice: state.Persona.Sheet.Voice, Traits: state.Persona.Sheet.Traits, Goals: state.Persona.Sheet.Goals,
		}); err != nil {
			return fmt.Errorf("roleplay character editor persona: %w", err)
		}
	} else if state.Persona.CharacterID != "" || state.Persona.Revision != 0 ||
		state.Persona.Sheet.Summary != "" || state.Persona.Sheet.Voice != "" ||
		len(state.Persona.Sheet.Traits) != 0 || len(state.Persona.Sheet.Goals) != 0 ||
		!state.Persona.UpdatedAt.IsZero() {
		return fmt.Errorf("roleplay character editor has unconfigured persona data")
	}
	if state.Capability.WorldID != state.World.ID || state.Capability.CharacterID != characterID {
		return fmt.Errorf("roleplay character editor capability authority is invalid")
	}
	if state.Generation.CharacterID != characterID {
		return fmt.Errorf("roleplay character editor generation authority is invalid")
	}
	if err := state.Generation.Config.Validate(); err != nil {
		return fmt.Errorf("roleplay character editor generation: %w", err)
	}
	installed := make(map[string]struct{}, len(state.InstalledModelNames))
	for _, modelName := range state.InstalledModelNames {
		if err := modelref.ValidateOllamaName(modelName); err != nil {
			return fmt.Errorf("roleplay character editor installed model: %w", err)
		}
		if _, duplicate := installed[modelName]; duplicate {
			return fmt.Errorf("roleplay character editor duplicates an installed model")
		}
		installed[modelName] = struct{}{}
	}
	return nil
}

func renderRoleplayCharacterEditorPersonaForm(
	characterID string,
	revision int64,
	sheet roleplay.PersonaSheet,
) string {
	return `<form data-action="submit->chat#saveRoleplayCharacterPersona" data-character-id="` +
		html.EscapeString(characterID) + `" class="mt-3 space-y-2">` +
		`<input type="hidden" name="expected_revision" value="` + strconv.FormatInt(revision, 10) + `">` +
		roleplayTextArea("summary", "Summary", sheet.Summary, true) +
		roleplayTextInput("voice", "Voice", sheet.Voice, "text", false) +
		roleplayTextArea("traits", "Traits, one per line", strings.Join(sheet.Traits, "\n"), false) +
		roleplayTextArea("goals", "Goals, one per line", strings.Join(sheet.Goals, "\n"), false) +
		roleplaySubmitButton("Save persona") + `</form>`
}

func renderRoleplayCharacterEditorGenerationForm(
	characterID string,
	characterName string,
	config roleplay.CharacterGenerationConfig,
	installedModels []string,
) string {
	return `<form data-action="change->chat#saveRoleplayCharacterGeneration" data-character-id="` +
		html.EscapeString(characterID) + `" class="mt-3">` +
		`<input type="hidden" name="expected_revision" value="` + strconv.FormatInt(config.Revision, 10) + `">` +
		roleplayCharacterEditorModelSelect(characterName, config.NarrativeModel, installedModels) + `</form>`
}

func roleplayCharacterEditorModelSelect(characterName, value string, installedModels []string) string {
	var options strings.Builder
	options.WriteString(`<option value=""`)
	if value == "" {
		options.WriteString(` selected`)
	}
	options.WriteString(`>Use global default</option>`)
	installed := make(map[string]struct{}, len(installedModels))
	for _, modelName := range installedModels {
		installed[modelName] = struct{}{}
	}
	if value != "" {
		if _, available := installed[value]; !available {
			options.WriteString(`<option value="` + html.EscapeString(value) + `" selected disabled>` +
				html.EscapeString(value) + ` — unavailable; select a replacement</option>`)
		}
	}
	for _, modelName := range installedModels {
		options.WriteString(`<option value="` + html.EscapeString(modelName) + `"`)
		if modelName == value {
			options.WriteString(` selected`)
		}
		options.WriteString(`>` + html.EscapeString(modelName) + `</option>`)
	}
	return `<label class="block text-[11px] text-zinc-400"><span>Model for ` + characterName +
		`</span><select name="narrative_model" aria-label="Response model for ` + characterName +
		`" class="mt-1 h-9 w-full rounded-md border border-white/10 bg-zinc-950 px-2 font-mono text-xs text-zinc-100 outline-none focus:border-violet-300/40">` +
		options.String() + `</select></label>`
}

func renderRoleplayCharacterEditorResearchForm(characterID string, enabled bool) string {
	checked := ""
	if enabled {
		checked = " checked"
	}
	return `<form data-action="change->chat#saveRoleplayCharacterResearch" data-character-id="` +
		html.EscapeString(characterID) + `" class="mt-3">` +
		`<label class="flex items-center gap-2 text-xs text-zinc-300"><input type="checkbox" name="enabled"` + checked +
		`><span>Allow explicit web research requests</span></label></form>`
}
