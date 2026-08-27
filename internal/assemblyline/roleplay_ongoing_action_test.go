package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayOngoingActionReturnsOneCompleteNullableLeaf(t *testing.T) {
	t.Parallel()
	input := RoleplayOngoingActionInput{
		CharacterName:         "Mara",
		Source:                RoleplayOngoingActionSourceAssistantResponse,
		ExactContribution:     "Mara keeps turning the seized wheel against the current.",
		PreviousOngoingAction: nil,
	}
	action := "Mara is turning the seized wheel against the current."
	actionJSON, _ := json.Marshal(action)
	for _, decision := range []RoleplayOngoingActionDecision{
		{Schema: RoleplayOngoingStateLeafV1, OngoingAction: actionJSON},
		{Schema: RoleplayOngoingStateLeafV1, OngoingAction: json.RawMessage("null")},
	} {
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("decision=%+v error=%v", decision, err)
		}
	}

	if _, err := DecodeRoleplayOngoingActionDecision(
		input, `{"schema":"omnidex.roleplay-ongoing-action.v1"}`,
	); err == nil || !strings.Contains(err.Error(), "ongoing_action") {
		t.Fatalf("missing explicit nullable leaf error=%v", err)
	}
	for _, raw := range []string{
		`{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":""}`,
		`{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":" padded "}`,
		`{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":42}`,
		`{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":null,"status":"clear"}`,
	} {
		if _, err := DecodeRoleplayOngoingActionDecision(input, raw); err == nil {
			t.Fatalf("invalid candidate was accepted: %s", raw)
		}
	}
}

func TestRoleplayOngoingActionPromptHasOnlyTheNecessarySemanticAuthority(t *testing.T) {
	t.Parallel()
	input := RoleplayOngoingActionInput{
		CharacterName:         "Ivo",
		Source:                RoleplayOngoingActionSourceAssistantResponse,
		ExactContribution:     "Ivo finishes tying the last knot and steps away.",
		PreviousOngoingAction: roleplayOngoingActionPointer("Ivo is tying the last knot."),
	}
	prompt, err := BuildRoleplayOngoingActionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		input.CharacterName, input.ExactContribution, string(input.Source),
		*input.PreviousOngoingAction, "previous_ongoing_action", "ongoing_action",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"exact_instruction", "user_turn", "context", "tool", "operation", "workflow", "complete the turn",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("prompt contains unrelated authority %q: %s", forbidden, prompt)
		}
	}
}

func TestRoleplayOngoingActionRejectsResponseBeyondRoleplayOutputBound(t *testing.T) {
	t.Parallel()
	input := RoleplayOngoingActionInput{
		CharacterName:     "Ivo",
		Source:            RoleplayOngoingActionSourceAssistantResponse,
		ExactContribution: strings.Repeat("x", roleplay.MaxNarrativeResponseBytes+1),
	}
	if _, err := NewRoleplayOngoingActionJob(input); err == nil {
		t.Fatal("ongoing-action station accepted a response beyond the roleplay output bound")
	}
}

func TestRoleplayOngoingActionAcceptsExactTypedUserActionAtItsOwnBound(t *testing.T) {
	t.Parallel()
	input := RoleplayOngoingActionInput{
		CharacterName:     "Ivo",
		Source:            RoleplayOngoingActionSourceUserAction,
		ExactContribution: strings.Repeat("x", roleplay.MaxUserTurnBytes),
	}
	if _, err := NewRoleplayOngoingActionJob(input); err != nil {
		t.Fatalf("bounded user action rejected: %v", err)
	}
	input.ExactContribution += "x"
	if _, err := NewRoleplayOngoingActionJob(input); err == nil {
		t.Fatal("over-bound user action was accepted")
	}
}

func roleplayOngoingActionPointer(value string) *string { return &value }
