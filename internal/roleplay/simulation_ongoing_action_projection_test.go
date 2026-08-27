package roleplay

import (
	"strings"
	"testing"
)

func TestCurrentOngoingActionResolvesByOpaqueCharacterIdentity(t *testing.T) {
	maraID := "rpc_11111111111111111111111111111111"
	ivoID := "rpc_22222222222222222222222222222222"
	projection := NarrativeSimulationProjection{
		Participants: []string{"Mara", "Ivo"},
		OngoingActions: []NarrativeOngoingAction{
			{CharacterName: "Ivo", Action: "Ivo is securing the eastern window."},
			{CharacterName: "Mara", Action: "Mara is carrying the ledger upstairs."},
		},
	}
	authority := SimulationNarrativeAuthority{
		ParticipantIDs:            []string{maraID, ivoID},
		OngoingActionStateIDs:     []string{"rpo_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "rpo_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		OngoingActionCharacterIDs: []string{ivoID, maraID},
	}

	action, err := CurrentOngoingActionForCharacter(projection, authority, maraID)
	if err != nil {
		t.Fatal(err)
	}
	if action == nil || *action != projection.OngoingActions[1].Action {
		t.Fatalf("Mara action=%v", action)
	}
	*action = "mutated caller copy"
	if projection.OngoingActions[1].Action != "Mara is carrying the ledger upstairs." {
		t.Fatal("opaque action lookup did not clone the frozen leaf")
	}

	missing, err := CurrentOngoingActionForCharacter(
		projection, authority, "rpc_33333333333333333333333333333333",
	)
	if err != nil || missing != nil {
		t.Fatalf("absent character action=%v err=%v", missing, err)
	}
}

func TestNarrativeOngoingActionRejectsMisalignedIdentityAuthority(t *testing.T) {
	projection := NarrativeSimulationProjection{
		Participants:   []string{"Mara", "Ivo"},
		OngoingActions: []NarrativeOngoingAction{{CharacterName: "Ivo", Action: "Ivo is keeping watch."}},
	}
	authority := SimulationNarrativeAuthority{
		ParticipantIDs:            []string{"rpc_11111111111111111111111111111111", "rpc_22222222222222222222222222222222"},
		OngoingActionStateIDs:     []string{"rpo_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		OngoingActionCharacterIDs: []string{"rpc_11111111111111111111111111111111"},
	}
	_, err := CurrentOngoingActionForCharacter(
		projection, authority, authority.ParticipantIDs[0],
	)
	if err == nil || !strings.Contains(err.Error(), "exact character identity") {
		t.Fatalf("misaligned identity error=%v", err)
	}

	authority.OngoingActionCharacterIDs = nil
	_, err = CurrentOngoingActionForCharacter(
		projection, authority, authority.ParticipantIDs[0],
	)
	if err == nil || !strings.Contains(err.Error(), "exact source identities") {
		t.Fatalf("misaligned length error=%v", err)
	}
}
