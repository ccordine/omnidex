package api

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayComposerOffersOnlyEligibleScenePersonas(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	activeID := state.Scene.ActiveCharacterID
	eligible := roleplay.SimulationCharacterSummary{
		ID: "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", WorldID: state.World.ID,
		LibraryID: "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Mara", CreatedAt: time.Now().UTC(),
	}
	outsideScene := roleplay.SimulationCharacterSummary{
		ID: "rpc_cccccccccccccccccccccccccccccccc", WorldID: state.World.ID,
		LibraryID: "rpl_cccccccccccccccccccccccccccccccc", Name: "Ivo", CreatedAt: time.Now().UTC(),
	}
	state.UserPersonaCharacters = append(state.UserPersonaCharacters, eligible, outsideScene)
	for id, generation := range testCharacterGenerationMap([]roleplay.SimulationCharacterSummary{eligible, outsideScene}) {
		state.CharacterGeneration[id] = generation
	}
	state.AllParticipants = append(state.AllParticipants, roleplay.SceneParticipantProjection{
		CharacterID: eligible.ID, Name: eligible.Name, TurnPosition: 1,
	})
	for _, selectedID := range []string{activeID, eligible.ID} {
		state.LastUserTurn = &roleplay.UserTurnAuthority{
			PersonaKind: roleplay.UserPersonaCharacter, CharacterID: selectedID,
			PersonaName:      state.CharacterNames[selectedID],
			ContributionKind: roleplay.UserContributionDialogue, ExactText: "The exact prior line.",
		}
		markup, err := renderRoleplayComposerAuthority(state)
		if err != nil {
			t.Fatal(err)
		}
		for _, participantID := range []string{activeID, eligible.ID} {
			expected := `value="` + participantID + `" data-persona-kind="character"`
			if participantID == selectedID {
				expected += ` selected`
			}
			if !strings.Contains(markup, expected) {
				t.Errorf("selection %q: composer lacks %q: %s", selectedID, expected, markup)
			}
		}
		if strings.Contains(markup, `<option value="`+outsideScene.ID+`"`) {
			t.Errorf("selection %q: composer offers outside-scene persona: %s", selectedID, markup)
		}
	}

	state.LastUserTurn = &roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: outsideScene.ID,
		PersonaName:      outsideScene.Name,
		ContributionKind: roleplay.UserContributionDialogue, ExactText: "A prior line from outside the current scene.",
	}
	markup, err := renderRoleplayComposerAuthority(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`value="narrator" data-persona-kind="narrator" selected`,
		`value="` + activeID + `" data-persona-kind="character"`,
		`value="` + eligible.ID + `" data-persona-kind="character"`,
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("outside-scene selection: composer lacks %q: %s", expected, markup)
		}
	}
}

func TestRoleplayComposerKeepsSoleParticipantUnavailableAsActingPersona(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	activeID := state.Scene.ActiveCharacterID
	state.LastUserTurn = &roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaCharacter, CharacterID: activeID,
		PersonaName:      state.ActiveCharacterName,
		ContributionKind: roleplay.UserContributionDialogue, ExactText: "The exact prior line.",
	}
	markup, err := renderRoleplayComposerAuthority(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`value="narrator" data-persona-kind="narrator" selected`,
		`Initiative: <strong`, state.ActiveCharacterName,
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("sole-participant composer lacks %q: %s", expected, markup)
		}
	}
	if strings.Contains(markup, `<option value="`+activeID+`"`) {
		t.Fatalf("sole participant was offered despite leaving no AI responder: %s", markup)
	}
}

func TestRoleplayComposerOmitsMisleadingSingularRoundModel(t *testing.T) {
	t.Parallel()
	state := configuredRoleplayComponentFixture(0, "Archive", "Atrium", "Charge", "Key", "Inspect")
	active := *state.ActiveGeneration
	active.Config.NarrativeModel = "qwen3.5:9b"
	state.ActiveGeneration = &active
	state.CharacterGeneration[active.CharacterID] = active

	markup, err := renderRoleplayComposerAuthority(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-roleplay-response-authority`, `Atrium`, `Rin`,
		`data-roleplay-authority-initiative`, `Round 3`, `Turn 8`, `Time 7`,
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("composer lacks authoritative response detail %q: %s", expected, markup)
		}
	}
	if strings.Contains(markup, `data-roleplay-authority-model`) || strings.Contains(markup, `Model:`) {
		t.Fatalf("composer presented one initiative-character model as round authority: %s", markup)
	}
}
