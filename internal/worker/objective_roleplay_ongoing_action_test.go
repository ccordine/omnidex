package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestExtractRoleplayOngoingActionConsumesOnlyOneResponseLeaf(t *testing.T) {
	t.Parallel()
	action := "Mara is hauling the anchor line toward the rail."
	replacement := "Mara is coiling the anchor line beside the rail."
	station := &scriptedRoleplayOngoingActionStation{
		actions: []*string{&action, &replacement, nil},
	}

	first, calls, err := extractRoleplayOngoingAction(
		t.Context(), station, assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mara", "Mara glances toward the dark horizon.", &action,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || first == nil || *first != action {
		t.Fatalf("action=%v calls=%d", first, calls)
	}
	second, calls, err := extractRoleplayOngoingAction(
		t.Context(), station, assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mara", "Mara begins coiling the recovered line.", &action,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || second == nil || *second != replacement {
		t.Fatalf("replacement action=%v calls=%d", second, calls)
	}
	third, calls, err := extractRoleplayOngoingAction(
		t.Context(), station, assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mara", "Mara finishes the coil and stands.", &replacement,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || third != nil {
		t.Fatalf("cleared action=%v calls=%d", third, calls)
	}
	if len(station.inputs) != 3 || station.inputs[0].CharacterName != "Mara" ||
		station.inputs[0].Source != assemblyline.RoleplayOngoingActionSourceAssistantResponse ||
		station.inputs[0].ExactContribution != "Mara glances toward the dark horizon." ||
		station.inputs[0].PreviousOngoingAction == nil ||
		*station.inputs[0].PreviousOngoingAction != action {
		t.Fatalf("station inputs=%+v", station.inputs)
	}
	if station.inputs[1].CharacterName != "Mara" ||
		station.inputs[1].Source != assemblyline.RoleplayOngoingActionSourceAssistantResponse ||
		station.inputs[1].ExactContribution != "Mara begins coiling the recovered line." ||
		station.inputs[1].PreviousOngoingAction == nil ||
		*station.inputs[1].PreviousOngoingAction != action {
		t.Fatalf("station inputs=%+v", station.inputs)
	}
}

func TestResolveRoleplayUserOngoingActionUsesOnlyExactTypedActionParts(t *testing.T) {
	t.Parallel()
	const actorID = "rpc_11111111111111111111111111111111"
	previous := "Mara is holding the hatch against the wind."
	resolved := "Mara is bracing the hatch with her shoulder."
	station := &scriptedRoleplayOngoingActionStation{actions: []*string{&resolved}}
	preparation := roleplay.SimulationTurnAuthority{
		UserTurn: roleplay.UserTurnAuthority{
			PersonaKind: roleplay.UserPersonaCharacter, CharacterID: actorID,
			PersonaName: "Mara", PersonaSummary: "A deck officer.",
			ContributionKind: roleplay.UserContributionActionDialogue,
			Parts: []roleplay.UserTurnPart{
				{Kind: roleplay.UserTurnPartAction, Text: "I brace the hatch with my shoulder."},
				{Kind: roleplay.UserTurnPartMessage, Text: "Get below!"},
			},
			ExactText: "[Action]\nI brace the hatch with my shoulder.\n\n[Message]\nGet below!",
		},
		NarrativeProjection: roleplay.NarrativeSimulationProjection{
			Participants: []string{"Mara", "Ivo"},
			OngoingActions: []roleplay.NarrativeOngoingAction{{
				CharacterName: "Mara", Action: previous,
			}},
		},
		NarrativeAuthority: roleplay.SimulationNarrativeAuthority{
			ParticipantIDs:            []string{actorID, "rpc_22222222222222222222222222222222"},
			OngoingActionStateIDs:     []string{"rpo_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			OngoingActionCharacterIDs: []string{actorID},
		},
	}
	completion, calls, err := resolveRoleplayUserOngoingAction(
		t.Context(), station, preparation, preparation.UserTurn,
		turnAuthority{ModelArtifactPaths: []string{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || completion == nil || completion.PreviousOngoingAction == nil ||
		*completion.PreviousOngoingAction != previous || completion.OngoingAction == nil ||
		*completion.OngoingAction != resolved {
		t.Fatalf("completion=%+v calls=%d", completion, calls)
	}
	if len(station.inputs) != 1 ||
		station.inputs[0].Source != assemblyline.RoleplayOngoingActionSourceUserAction ||
		station.inputs[0].ExactContribution != "[Action]\nI brace the hatch with my shoulder." {
		t.Fatalf("user action station input=%+v", station.inputs)
	}
}

func TestResolveRoleplayUserOngoingActionSkipsDialogueWithoutStationCall(t *testing.T) {
	t.Parallel()
	preparation := roleplay.SimulationTurnAuthority{UserTurn: roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaCharacter,
		CharacterID: "rpc_11111111111111111111111111111111",
		PersonaName: "Mara", PersonaSummary: "A deck officer.",
		ContributionKind: roleplay.UserContributionDialogue,
		Parts:            []roleplay.UserTurnPart{{Kind: roleplay.UserTurnPartMessage, Text: "Get below!"}},
		ExactText:        "[Message]\nGet below!",
	}}
	completion, calls, err := resolveRoleplayUserOngoingAction(
		t.Context(), nil, preparation, preparation.UserTurn,
		turnAuthority{ModelArtifactPaths: []string{}},
	)
	if err != nil || completion != nil || calls != 0 {
		t.Fatalf("dialogue completion=%+v calls=%d err=%v", completion, calls, err)
	}
}

type scriptedRoleplayOngoingActionStation struct {
	calls   int
	actions []*string
	inputs  []assemblyline.RoleplayOngoingActionInput
}

func (station *scriptedRoleplayOngoingActionStation) ResolveOngoingAction(
	_ context.Context,
	input assemblyline.RoleplayOngoingActionInput,
) (assemblyline.RoleplayOngoingActionDecision, objectiveStationReceipt, error) {
	station.inputs = append(station.inputs, input)
	var action *string
	if station.calls < len(station.actions) {
		action = station.actions[station.calls]
	}
	station.calls++
	raw := json.RawMessage("null")
	if action != nil {
		raw, _ = json.Marshal(*action)
	}
	return assemblyline.RoleplayOngoingActionDecision{
		Schema: assemblyline.RoleplayOngoingStateLeafV1, OngoingAction: raw,
	}, objectiveStationReceipt{Calls: 1}, nil
}
