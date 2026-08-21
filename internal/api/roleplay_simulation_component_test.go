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
			`data-recyclr-target="roleplay-simulation"`,
			`data-recyclr-target="roleplay-composer-authority"`,
			`data-recyclr-target="roleplay-cast-sidebar"`, fixture.world, fixture.scene,
			`data-roleplay-setup-flow`, `role="tablist"`,
			`data-roleplay-setup-tab="scene"`, `data-roleplay-setup-tab="cast"`,
			`data-roleplay-setup-tab="state"`, `data-roleplay-setup-tab="actions"`,
			`data-roleplay-setup-panel="scene"`, `data-roleplay-setup-panel="cast"`,
			`data-roleplay-setup-panel="state"`, `data-roleplay-setup-panel="actions"`,
			`data-action="chat#selectRoleplaySetupSection"`,
			"Scene sheet", "Turn order", "Meters", "Inventory", "Configured interactions",
			"Item templates", `data-action="submit->chat#updateRoleplayScene"`,
			`data-action="submit->chat#saveRoleplaySceneDraftParticipant"`, `name="expected_draft_revision"`,
			fixture.meter, fixture.item, fixture.interaction,
			`data-action="submit->chat#setRoleplayMeter"`, `data-action="chat#useRoleplayCommand"`,
			`data-action="chat#openRoleplayCharacterEditor"`,
			`data-action="submit->chat#downloadRoleplayModel"`,
		} {
			if !strings.Contains(component.HTML.Bundle, expected) {
				t.Errorf("fixture %d component lacks %q: %s", index, expected, component.HTML.Bundle)
			}
		}
		if strings.Contains(component.HTML.Bundle, "voice rewrite") ||
			strings.Contains(component.HTML.Bundle, "voice preservation") {
			t.Errorf("fixture %d exposes retired post-response voice stations: %s", index, component.HTML.Bundle)
		}
		if strings.Contains(component.HTML.Bundle, "continuity check") ||
			strings.Contains(component.HTML.Bundle, "frozen world state") {
			t.Errorf("fixture %d describes superseded whole-state roleplay context: %s", index, component.HTML.Bundle)
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

func TestRoleplayComposerSelectsUserPersonaSeparatelyFromResponder(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	gryphID := "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	state.AllParticipants = append(state.AllParticipants, roleplay.SceneParticipantProjection{
		CharacterID: gryphID, Name: "Gryph <Artificer>", TurnPosition: 1,
	})
	state.UserPersonaCharacters = append(state.UserPersonaCharacters, roleplay.SimulationCharacterSummary{
		ID: gryphID, WorldID: state.World.ID, LibraryID: testRoleplayLibraryID(gryphID),
		Name: "Gryph <Artificer>", CreatedAt: time.Now().UTC(),
	})
	state.CharacterGeneration[gryphID] = testCharacterGenerationMap(
		state.UserPersonaCharacters[len(state.UserPersonaCharacters)-1:],
	)[gryphID]
	state.LastUserTurn = &roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: gryphID,
		PersonaName: "Gryph <Artificer>", PersonaSummary: "An artificer from afar.",
		ContributionKind: roleplay.UserContributionActionDialogue,
		Parts: []roleplay.UserTurnPart{
			{Kind: roleplay.UserTurnPartAction, Text: "I lift the key."},
			{Kind: roleplay.UserTurnPartMessage, Text: "Lead on."},
		},
		ExactText: "[Action]\nI lift the key.\n\n[Message]\nLead on.",
	}
	activeGeneration := state.CharacterGeneration[string(state.Channel.RoleplayViewpointCharacterID)]
	activeGeneration.Config.NarrativeModel = "qwen3.5:9b"
	state.CharacterGeneration[string(state.Channel.RoleplayViewpointCharacterID)] = activeGeneration
	state.ActiveGeneration = &activeGeneration
	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`data-chat-target="roleplayPersona"`, `aria-label="Acting as"`,
		`value="narrator" data-persona-kind="narrator"`,
		`value="rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" data-persona-kind="character" selected`,
		`value="rpc_0123456789abcdef0123456789abcdef" data-persona-kind="character"`,
		`Gryph &lt;Artificer&gt;`, `aria-label="Create an identity"`, `>Characters</span>`,
		`aria-label="Toggle Rin as a responder"`, `aria-label="Toggle Gryph &lt;Artificer&gt; as a responder"`,
	} {
		if !strings.Contains(component.HTML.Bundle, required) {
			t.Errorf("roleplay composer lacks %q: %s", required, component.HTML.Bundle)
		}
	}
	for _, obsolete := range []string{
		`data-chat-target="roleplayContribution"`, `Scene / world`, `Responder / model`,
		`<strong>Rin</strong> responds`, `Archive · Atrium · revision 7`,
	} {
		if strings.Contains(component.HTML.Bundle, obsolete) {
			t.Fatalf("minimal roleplay composer retained obsolete authority slab %q", obsolete)
		}
	}
}

func TestRoleplayCharacterSidebarSeparatesEditToggleAndOrder(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	second := roleplay.SimulationCharacterSummary{
		ID: "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", WorldID: state.World.ID,
		LibraryID: "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Gryph <Artificer>", CreatedAt: time.Now().UTC(),
	}
	state.UserPersonaCharacters = append(state.UserPersonaCharacters, second)
	state.CharacterGeneration[second.ID] = testCharacterGenerationMap([]roleplay.SimulationCharacterSummary{second})[second.ID]
	active := state.CharacterGeneration[string(state.Channel.RoleplayViewpointCharacterID)]
	active.Config.NarrativeModel = "qwen3.5:9b"
	state.CharacterGeneration[active.CharacterID] = active
	state.ActiveGeneration = &active

	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	markup := component.HTML.Bundle
	if count := strings.Count(markup, `data-action="chat#openRoleplayCharacterEditor"`); count != 2 {
		t.Fatalf("character editor action count=%d, want 2: %s", count, markup)
	}
	for _, required := range []string{
		`aria-label="Edit Rin"`, `aria-label="Edit Gryph &lt;Artificer&gt;"`,
		`aria-label="Toggle Rin as a responder"`, `aria-label="Toggle Gryph &lt;Artificer&gt; as a responder"`,
		`aria-label="Reorder Rin"`, `draggable="true"`,
	} {
		if !strings.Contains(markup, required) {
			t.Errorf("character sidebar lacks %q: %s", required, markup)
		}
	}
	if count := strings.Count(markup, `submit->chat#downloadRoleplayModel`); count != 1 {
		t.Fatalf("inline download form count=%d, want 1: %s", count, markup)
	}
	for _, obsolete := range []string{
		`saveRoleplayGeneration`, `name="narrative_model"`, `aria-label="Response model for`,
	} {
		if strings.Contains(markup, obsolete) {
			t.Fatalf("sidebar retained inline character editing %q: %s", obsolete, markup)
		}
	}
	if strings.Contains(markup, `data-action="chat#toggleRoleplayResponder" data-roleplay-character-id="rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" aria-label="Edit`) {
		t.Fatalf("character name is still wired into responder toggling: %s", markup)
	}
}

func TestRoleplaySimulationRendersWithUnavailablePersistedNarrativeModel(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	characterID := string(state.Channel.RoleplayViewpointCharacterID)
	generation := state.CharacterGeneration[characterID]
	generation.Config.NarrativeModel = "removed-model:9b"
	state.CharacterGeneration[characterID] = generation
	state.ActiveGeneration = &generation
	state.InstalledModelNames = []string{"replacement-model:4b"}

	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	if !component.Configured || component.SceneRevision == nil {
		t.Fatalf("stale model prevented the authoritative RP UI from rendering: %+v", component)
	}
}

func TestRoleplaySimulationComponentRendersExplicitUnconfiguredSetup(t *testing.T) {
	t.Parallel()
	state := baseRoleplayComponentFixture("Unconfigured <world>")
	state.Characters = []roleplay.SimulationCharacterSummary{{
		ID: "rpc_11111111111111111111111111111111", WorldID: state.World.ID,
		LibraryID: "rpl_11111111111111111111111111111111", Name: "Rin <script>", CreatedAt: time.Now().UTC(),
	}, {
		ID: "rpc_22222222222222222222222222222222", WorldID: state.World.ID,
		LibraryID: "rpl_22222222222222222222222222222222", Name: "Sol", CreatedAt: time.Now().UTC(),
	}}
	state.UserPersonaCharacters = append([]roleplay.SimulationCharacterSummary(nil), state.Characters...)
	state.CharacterGeneration = testCharacterGenerationMap(state.Characters)
	state.CharacterHasPersona = map[string]bool{"rpc_22222222222222222222222222222222": true}
	component, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Simulation setup required before sending a turn.",
		`data-roleplay-setup-tab="cast"`, `data-roleplay-setup-tab="scene"`,
		`data-roleplay-setup-panel="cast"`, `data-roleplay-setup-panel="scene"`,
		`data-roleplay-default-setup-section="cast"`,
		`data-action="chat#openRoleplayCharacterEditor"`,
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
	if strings.Contains(component.HTML.Bundle, `chat#createRoleplayCharacter`) {
		t.Fatalf("unconfigured response retained world-local character creation: %s", component.HTML.Bundle)
	}
}

func TestRoleplaySimulationComponentUsesOnlyServerPageCursors(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Glass Key", "Inspect")
	state.Characters = []roleplay.SimulationCharacterSummary{
		{ID: "rpc_11111111111111111111111111111111", WorldID: state.World.ID, LibraryID: "rpl_11111111111111111111111111111111", Name: "One", CreatedAt: time.Now()},
		{ID: "rpc_22222222222222222222222222222222", WorldID: state.World.ID, LibraryID: "rpl_22222222222222222222222222222222", Name: "Two", CreatedAt: time.Now()},
		{ID: "rpc_33333333333333333333333333333333", WorldID: state.World.ID, LibraryID: "rpl_33333333333333333333333333333333", Name: "Three", CreatedAt: time.Now()},
		{ID: "rpc_44444444444444444444444444444444", WorldID: state.World.ID, LibraryID: "rpl_44444444444444444444444444444444", Name: "Four", CreatedAt: time.Now()},
	}
	state.UserPersonaCharacters = append(state.UserPersonaCharacters, state.Characters...)
	for id, generation := range testCharacterGenerationMap(state.Characters) {
		state.CharacterGeneration[id] = generation
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
		ID: viewpointID, WorldID: state.World.ID,
		LibraryID: testRoleplayLibraryID(viewpointID), Name: "Rin", CreatedAt: time.Now().UTC(),
	}}
	state.UserPersonaCharacters = append([]roleplay.SimulationCharacterSummary(nil), state.Characters...)
	state.CharacterGeneration = testCharacterGenerationMap(state.Characters)
	state.InstalledModelNames = []string{"dolphin3:latest", "qwen3.5:9b"}
	activeGeneration := state.CharacterGeneration[viewpointID]
	state.ActiveGeneration = &activeGeneration
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

func testCharacterGenerationMap(
	characters []roleplay.SimulationCharacterSummary,
) map[string]roleplay.CharacterGenerationProjection {
	result := make(map[string]roleplay.CharacterGenerationProjection, len(characters))
	for _, character := range characters {
		result[character.ID] = roleplay.CharacterGenerationProjection{
			CharacterID: character.ID,
			Config: roleplay.CharacterGenerationConfig{
				Schema:             roleplay.CharacterGenerationConfigSchemaV2,
				LibraryCharacterID: character.LibraryID,
				Revision:           1,
			},
			UpdatedAt: time.Now().UTC(),
		}
	}
	return result
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
