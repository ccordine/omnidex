package roleplay

import "testing"

func TestNextSceneCharacterNeverTransfersResponderOwnershipToUserPersona(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{ActiveCharacterID: "rpc_11111111111111111111111111111111"},
		Participants: []SceneParticipantProjection{
			{CharacterID: "rpc_11111111111111111111111111111111", TurnPosition: 0},
			{CharacterID: "rpc_22222222222222222222222222222222", TurnPosition: 1},
		},
	}

	got, err := nextSceneCharacterExcept(scene, "rpc_22222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if got != scene.Sheet.ActiveCharacterID {
		t.Fatalf("next responder=%q want current responder %q", got, scene.Sheet.ActiveCharacterID)
	}
}

func TestNextSceneCharacterRotatesAcrossNonUserParticipants(t *testing.T) {
	scene := lockedSimulationScene{
		Sheet: SceneSheet{ActiveCharacterID: "rpc_11111111111111111111111111111111"},
		Participants: []SceneParticipantProjection{
			{CharacterID: "rpc_11111111111111111111111111111111", TurnPosition: 0},
			{CharacterID: "rpc_22222222222222222222222222222222", TurnPosition: 1},
			{CharacterID: "rpc_33333333333333333333333333333333", TurnPosition: 2},
		},
	}

	got, err := nextSceneCharacterExcept(scene, "rpc_22222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if got != "rpc_33333333333333333333333333333333" {
		t.Fatalf("next responder=%q", got)
	}
}
