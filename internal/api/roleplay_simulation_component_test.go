package api

import (
	"html"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayItemTemplatesRenderCanonicalGiveAndTakeAffordances(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Held key", "Inspect")
	state.ItemTemplates[0].Name = "Captain's field kit (Mk. 2)"
	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	markup := html.UnescapeString(component.HTML.Bundle)
	for _, exact := range []string{`/give "Captain's field kit (Mk. 2)"`, `/take "Captain's field kit (Mk. 2)"`} {
		if !strings.Contains(markup, exact) {
			t.Errorf("component lacks canonical item text %q: %s", exact, markup)
		}
	}
	if !strings.Contains(markup, `data-roleplay-page-section="item-templates"`) && state.ItemTemplatesMore {
		t.Fatalf("item-template page omitted its server cursor: %s", markup)
	}
}

func TestRoleplaySimulationComponentRendersGenericServerAuthority(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		world       string
		scene       string
		meter       string
		item        string
		interaction string
	}{
		{"Orbital Archive", "Docking Window", "Signal", "Cipher Lens", "Calibrate"},
		{"Council Chamber", "Open Deliberation", "Consensus", "Sealed Brief", "Propose"},
	}
	for index, fixture := range fixtures {
		state := configuredRoleplayComponentFixture(index, fixture.world, fixture.scene, fixture.meter, fixture.item, fixture.interaction)
		component, err := renderRoleplaySimulationComponent(state)
		if err != nil {
			t.Fatalf("fixture %d: %v", index, err)
		}
		for _, expected := range []string{
			`data-recyclr-target="roleplay-simulation"`, fixture.world, fixture.scene,
			"Scene sheet", "Character sheets", "Turn order", "Meters", "Inventory", "Configured interactions",
			"Item templates", `data-action="submit->chat#updateRoleplayScene"`,
			`data-action="submit->chat#saveRoleplaySceneDraftParticipant"`, `name="expected_draft_revision"`,
			fixture.meter, fixture.item, fixture.interaction,
			`data-action="submit->chat#setRoleplayMeter"`, `data-action="chat#useRoleplayCommand"`,
			`data-action="submit->chat#configureRoleplayResearch"`, `/research`,
		} {
			if !strings.Contains(component.HTML.Bundle, expected) {
				t.Errorf("fixture %d component lacks %q: %s", index, expected, component.HTML.Bundle)
			}
		}
		if !component.Configured || component.SceneRevision == nil || *component.SceneRevision != 7 {
			t.Fatalf("fixture %d response=%+v", index, component)
		}
		for _, forbidden := range []string{"tool", "operation"} {
			if strings.Contains(strings.ToLower(component.HTML.Bundle), forbidden) {
				t.Errorf("fixture %d capability UI exposes forbidden execution language %q", index, forbidden)
			}
		}
	}
}

func TestRoleplaySimulationComponentRendersExplicitUnconfiguredSetup(t *testing.T) {
	t.Parallel()
	state := baseRoleplayComponentFixture("Unconfigured <world>")
	state.Characters = []roleplay.SimulationCharacterSummary{{
		ID: "rpc_11111111111111111111111111111111", WorldID: state.World.ID,
		Name: "Rin <script>", CreatedAt: time.Now().UTC(),
	}, {
		ID: "rpc_22222222222222222222222222222222", WorldID: state.World.ID,
		Name: "Sol", CreatedAt: time.Now().UTC(),
	}}
	state.CharacterHasPersona = map[string]bool{"rpc_22222222222222222222222222222222": true}
	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Simulation setup required before sending a turn.",
		`data-action="submit->chat#createRoleplayCharacter"`,
		`data-action="submit->chat#saveRoleplayPersona"`,
		`data-action="submit->chat#createRoleplayScene"`,
		`Unconfigured &lt;world&gt;`, `Rin &lt;script&gt;`,
		`data-action="submit->chat#saveRoleplaySceneDraftParticipant"`,
		`data-character-id="rpc_22222222222222222222222222222222"`,
		`name="expected_draft_revision"`,
	} {
		if !strings.Contains(component.HTML.Bundle, expected) {
			t.Errorf("unconfigured component lacks %q: %s", expected, component.HTML.Bundle)
		}
	}
	if component.Configured || component.SceneRevision != nil || strings.Contains(component.HTML.Bundle, "<script>") {
		t.Fatalf("unconfigured response invented or leaked state: %+v %s", component, component.HTML.Bundle)
	}
	if strings.Contains(component.HTML.Bundle, `Ordered participant IDs`) || strings.Contains(component.HTML.Bundle, `break-all`) {
		t.Fatalf("unconfigured response asks users to transcribe opaque identities: %s", component.HTML.Bundle)
	}
}

func TestRoleplaySimulationComponentUsesOnlyServerPageCursors(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Glass Key", "Inspect")
	state.Characters = []roleplay.SimulationCharacterSummary{
		{ID: "rpc_11111111111111111111111111111111", WorldID: state.World.ID, Name: "One", CreatedAt: time.Now()},
		{ID: "rpc_22222222222222222222222222222222", WorldID: state.World.ID, Name: "Two", CreatedAt: time.Now()},
		{ID: "rpc_33333333333333333333333333333333", WorldID: state.World.ID, Name: "Three", CreatedAt: time.Now()},
		{ID: "rpc_44444444444444444444444444444444", WorldID: state.World.ID, Name: "Four", CreatedAt: time.Now()},
	}
	state.CharactersMore = true
	state.ItemTemplates = []roleplay.ItemTemplateDefinition{
		{ID: "rpi_11111111111111111111111111111111", WorldID: state.World.ID, Name: "Alpha", Description: "A.", UsePolicy: roleplay.ItemUseInfinite, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}}},
		{ID: "rpi_22222222222222222222222222222222", WorldID: state.World.ID, Name: "Beta", Description: "B.", UsePolicy: roleplay.ItemUseInfinite, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}}},
		{ID: "rpi_33333333333333333333333333333333", WorldID: state.World.ID, Name: "Gamma", Description: "C.", UsePolicy: roleplay.ItemUseInfinite, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}}},
		{ID: "rpi_44444444444444444444444444444444", WorldID: state.World.ID, Name: "Delta", Description: "D.", UsePolicy: roleplay.ItemUseInfinite, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}}},
	}
	state.ItemTemplatesMore = true
	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-action="chat#loadRoleplayPage"`, `data-roleplay-page-section="characters"`,
		`data-characters-offset="4"`, `data-personas-offset="0"`, `data-inventory-offset="0"`,
		`data-roleplay-page-section="item-templates"`, `data-item-templates-offset="4"`,
	} {
		if !strings.Contains(component.HTML.Bundle, expected) {
			t.Errorf("component lacks server cursor %q: %s", expected, component.HTML.Bundle)
		}
	}
}

func configuredRoleplayComponentFixture(
	index int,
	worldName, sceneTitle, meterName, itemName, interactionName string,
) roleplaySimulationComponentState {
	state := baseRoleplayComponentFixture(worldName)
	viewpointID := string(state.Channel.RoleplayViewpointCharacterID)
	sceneID := "rps_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state.Scene = &roleplay.SceneSheet{
		ID: sceneID, WorldID: state.World.ID, Title: sceneTitle, Description: "Exact persisted scene.",
		Revision: 7, ActiveCharacterID: viewpointID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	state.ActiveCharacterName = "Rin"
	state.Participants = []roleplay.SceneParticipantProjection{{CharacterID: viewpointID, Name: "Rin", TurnPosition: 0}}
	state.AllParticipants = append([]roleplay.SceneParticipantProjection(nil), state.Participants...)
	state.SceneDraft = roleplaySceneDraft{
		Schema: roleplaySceneDraftSchema, ChannelID: state.Channel.ID, WorldID: state.World.ID,
		Revision: 3, SceneRevision: state.Scene.Revision,
		Participants: []roleplaySceneDraftParticipant{{CharacterID: viewpointID}},
	}
	state.Characters = []roleplay.SimulationCharacterSummary{{
		ID: viewpointID, WorldID: state.World.ID, Name: "Rin", CreatedAt: time.Now().UTC(),
	}}
	state.CharacterHasPersona = map[string]bool{viewpointID: true}
	state.CharacterNames = map[string]string{viewpointID: "Rin"}
	state.CharacterCapabilities = map[string]roleplay.CharacterCapabilityProjection{
		viewpointID: {WorldID: state.World.ID, CharacterID: viewpointID, WebResearch: true},
	}
	state.Personas = []roleplayNamedPersona{{Name: "Rin", Projection: roleplay.PersonaProjection{
		CharacterID: viewpointID, Revision: 3,
		Sheet:     roleplay.PersonaSheet{Summary: "Archivist", Voice: "Measured", Traits: []string{"Patient"}, Goals: []string{"Understand"}},
		UpdatedAt: time.Now().UTC(),
	}}}
	state.Meters = []roleplay.MeterProjection{{Key: "signal", Name: meterName, Minimum: 0, Maximum: 12, Value: 5, Revision: 2}}
	state.Inventory = []roleplay.InventoryItemProjection{{
		ID: "rpv_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TemplateID: "rpi_cccccccccccccccccccccccccccccccc",
		Name: itemName, Description: "A configured item.", UsePolicy: roleplay.ItemUseFinite, RemainingUses: 2,
	}}
	state.Interactions = []roleplay.InteractionCommandDefinition{{
		ID: "rpa_dddddddddddddddddddddddddddddddd", WorldID: state.World.ID,
		Key: "inspect", Name: interactionName, Description: "Examine the current situation.",
		ArgumentMode: roleplay.CommandArgumentRequired, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: 1}},
	}}
	state.ItemTemplates = []roleplay.ItemTemplateDefinition{{
		ID: "rpi_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", WorldID: state.World.ID,
		Name: itemName, Description: "A configured item template.", UsePolicy: roleplay.ItemUseFinite,
		InitialUses: 2, Priority: 1, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: -1}},
	}}
	_ = index
	return state
}

func baseRoleplayComponentFixture(worldName string) roleplaySimulationComponentState {
	now := time.Now().UTC()
	return roleplaySimulationComponentState{
		Channel: model.Channel{
			ID: "story-42", Scope: model.ChannelScopeUser, Name: "Story", Tags: []string{"user-channel"},
			ProjectID: 42, WorkspaceRoot: "/workspace/story", Mode: model.ChannelModeRoleplay,
			RoleplayViewpointCharacterID: "rpc_0123456789abcdef0123456789abcdef", CreatedAt: now, UpdatedAt: now,
		},
		World: roleplay.World{
			ID: "rpw_0123456789abcdef0123456789abcdef", ChannelID: "story-42", Name: worldName,
			Authority: roleplay.AuthorityFictionalCanon, CreatedAt: now,
		},
		SceneDraft: emptyRoleplaySceneDraft("story-42", "rpw_0123456789abcdef0123456789abcdef"),
	}
}
