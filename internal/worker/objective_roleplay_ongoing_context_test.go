package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestOngoingActionIsOneOptionalCandidateAndClockRemainsRequired(t *testing.T) {
	characterID := "rpc_11111111111111111111111111111111"
	stateID := "rpo_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	projection := roleplay.NarrativeSimulationProjection{
		Scene: roleplay.NarrativeScene{
			Title: "Archive", Description: "Rain strikes the glass.",
			ActiveCharacterName: "Mara",
			Initiative: roleplay.SimulationInitiativeClock{
				Round: 3, Turn: 7, FictionalTimeTick: 6,
			},
		},
		Participants: []string{"Mara"},
		Viewpoint:    roleplay.NarrativePersona{Name: "Mara"},
		OngoingActions: []roleplay.NarrativeOngoingAction{{
			CharacterName: "Mara", Action: "Mara is sorting the flooded ledgers.",
		}},
	}
	authority := roleplay.SimulationNarrativeAuthority{
		SceneID:                   "rps_22222222222222222222222222222222",
		ViewpointID:               characterID,
		ParticipantIDs:            []string{characterID},
		OngoingActionStateIDs:     []string{stateID},
		OngoingActionCharacterIDs: []string{characterID},
	}
	responder := roleplay.SimulationResponderAuthority{
		CharacterID: characterID, NarrativeProjection: projection,
		NarrativeAuthority: authority,
	}

	required, _, rankable, err := roleplayContextRecordGroups(
		roleplay.SimulationTurnAuthority{}, responder, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 2 || required[0].Namespace != "scene_state" {
		t.Fatalf("required records=%+v", required)
	}
	for _, exact := range []string{
		"Active initiative character: Mara", "round 3", "turn 7",
		"fictional time tick 6",
	} {
		if !strings.Contains(required[0].Content, exact) {
			t.Fatalf("required scene state %q lacks %q", required[0].Content, exact)
		}
	}
	if len(rankable) != 1 || rankable[0].Namespace != "ongoing_action" ||
		rankable[0].SourceID != stateID ||
		rankable[0].Content != "Current ongoing action for Mara: Mara is sorting the flooded ledgers." {
		t.Fatalf("rankable ongoing action=%+v", rankable)
	}
}
