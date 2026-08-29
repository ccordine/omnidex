package api

import (
	"html"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCharacterEditorRendersOnlyExactCharacterAuthority(t *testing.T) {
	t.Parallel()
	state := roleplayCharacterEditorFixture(t)
	component, err := renderRoleplayCharacterEditor(state)
	if err != nil {
		t.Fatal(err)
	}
	if component.ChannelID != state.Channel.ID || component.WorldID != state.World.ID ||
		component.CharacterID != state.Character.CharacterID {
		t.Fatalf("component authority changed: %+v", component)
	}
	markup := html.UnescapeString(component.HTML.Bundle)
	for _, required := range []string{
		`data-recyclr-target="roleplay-character-editor"`,
		`data-roleplay-character-editor-character="rpc_0123456789abcdef0123456789abcdef"`,
		`Mara <Vey>`, `Revision 4`, `Keeps exact canon.`, `Dry & precise`,
		`data-action="submit->chat#saveRoleplayCharacterPersona"`,
		`data-action="change->chat#saveRoleplayCharacterGeneration"`,
		`data-action="change->chat#saveRoleplayCharacterResearch"`,
		`name="expected_revision" value="4"`,
		`name="expected_revision" value="7"`,
		`<option value="qwen3.5:9b" selected>qwen3.5:9b</option>`,
		`<option value="dolphin3:latest">dolphin3:latest</option>`,
		`name="enabled" checked`,
	} {
		if !strings.Contains(markup, required) {
			t.Errorf("character editor lacks %q: %s", required, markup)
		}
	}
	for _, forbidden := range []string{
		"OTHER_CHARACTER_SENTINEL", "Scene sheet", "Turn order", `data-characters-offset=`,
	} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("character editor leaked unrelated authority %q: %s", forbidden, markup)
		}
	}
}

func TestRoleplayCharacterEditorSupportsUnconfiguredPersona(t *testing.T) {
	t.Parallel()
	state := roleplayCharacterEditorFixture(t)
	state.Persona = roleplay.PersonaProjection{}
	state.PersonaConfigured = false
	component, err := renderRoleplayCharacterEditor(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`Sheet required`, `name="expected_revision" value="0"`, `name="summary"`} {
		if !strings.Contains(component.HTML.Bundle, expected) {
			t.Errorf("unconfigured persona editor lacks %q: %s", expected, component.HTML.Bundle)
		}
	}
}

func TestRoleplayCharacterEditorRejectsCrossWorldAuthority(t *testing.T) {
	t.Parallel()
	state := roleplayCharacterEditorFixture(t)
	state.Character.WorldID = "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, err := renderRoleplayCharacterEditor(state)
	if err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("error=%v, want projection authority failure", err)
	}
}

func TestRoleplayCharacterEditorKeepsUnavailableConfiguredModelExplicit(t *testing.T) {
	t.Parallel()
	state := roleplayCharacterEditorFixture(t)
	state.InstalledModelNames = []string{"dolphin3:latest", "gemma3:4b"}

	component, err := renderRoleplayCharacterEditor(state)
	if err != nil {
		t.Fatal(err)
	}
	markup := html.UnescapeString(component.HTML.Bundle)
	for _, required := range []string{
		`<option value="qwen3.5:9b" selected disabled>qwen3.5:9b — unavailable; select a replacement</option>`,
		`<option value="dolphin3:latest">dolphin3:latest</option>`,
		`<option value="gemma3:4b">gemma3:4b</option>`,
	} {
		if !strings.Contains(markup, required) {
			t.Errorf("stale-model editor lacks %q: %s", required, markup)
		}
	}
	if strings.Contains(markup, `<option value="" selected>Use global default</option>`) {
		t.Fatalf("stale model was silently replaced with the global default: %s", markup)
	}
	if strings.Count(markup, `value="qwen3.5:9b"`) != 1 {
		t.Fatalf("stale configured model was duplicated: %s", markup)
	}
}

func roleplayCharacterEditorFixture(t *testing.T) roleplayCharacterEditorState {
	t.Helper()
	base := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	characterID := string(base.Channel.RoleplayViewpointCharacterID)
	projection := roleplay.CharacterProjection{
		Schema: roleplay.CharacterProjectionSchemaV1, Authority: roleplay.AuthorityCharacterKnowledge,
		WorldID: base.World.ID, WorldName: base.World.Name,
		CharacterID: characterID, CharacterName: "Mara <Vey>", Facts: []roleplay.ContextFact{},
	}
	fingerprint, err := roleplay.ExactCharacterProjectionFingerprint(projection)
	if err != nil {
		t.Fatal(err)
	}
	projection.Fingerprint = fingerprint
	generation := base.CharacterGeneration[characterID]
	generation.Config.Revision = 7
	generation.Config.NarrativeModel = "qwen3.5:9b"
	return roleplayCharacterEditorState{
		Channel: base.Channel, World: base.World, Character: projection,
		PersonaConfigured: true,
		Persona: roleplay.PersonaProjection{
			CharacterID: characterID, Revision: 4,
			Sheet: roleplay.PersonaSheet{
				Summary: "Keeps exact canon.", Voice: "Dry & precise",
				Traits: []string{"watchful"}, Goals: []string{"remember"},
			},
			UpdatedAt: time.Now().UTC(),
		},
		Capability: roleplay.CharacterCapabilityProjection{
			WorldID: base.World.ID, CharacterID: characterID, WebResearch: true,
		},
		Generation: generation, InstalledModelNames: []string{"dolphin3:latest", "qwen3.5:9b"},
	}
}
