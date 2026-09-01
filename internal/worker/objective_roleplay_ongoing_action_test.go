package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type recordingRoleplayOngoingActionStation struct {
	relation        assemblyline.RoleplayOngoingActionRelation
	value           string
	relationCalls   int
	valueCalls      int
	relationReceipt objectiveStationReceipt
	valueReceipt    objectiveStationReceipt
	valueInput      assemblyline.RoleplayOngoingActionValueInput
}

func (station *recordingRoleplayOngoingActionStation) ResolveOngoingActionRelation(
	_ context.Context,
	_ assemblyline.RoleplayOngoingActionRelationInput,
) (assemblyline.RoleplayOngoingActionRelation, objectiveStationReceipt, error) {
	station.relationCalls++
	return station.relation, station.relationReceipt, nil
}

func (station *recordingRoleplayOngoingActionStation) GenerateOngoingActionValue(
	_ context.Context,
	input assemblyline.RoleplayOngoingActionValueInput,
) (string, objectiveStationReceipt, error) {
	station.valueCalls++
	station.valueInput = input
	return station.value, station.valueReceipt, nil
}

func TestExtractRoleplayOngoingActionClearsWithoutValueCall(t *testing.T) {
	station := &recordingRoleplayOngoingActionStation{
		relation:        assemblyline.RoleplayOngoingActionAbsent,
		relationReceipt: objectiveStationReceipt{Calls: 1},
	}
	previous := "Crossing the bridge."
	result, calls, err := extractRoleplayOngoingAction(
		context.Background(), station,
		assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mira", "Mira reaches the far bank and stops.", &previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != nil || result.RequiresRestoration || calls != 1 {
		t.Fatalf("result = %#v, calls = %d; want nil and one relation call", result, calls)
	}
	if station.relationCalls != 1 || station.valueCalls != 0 {
		t.Fatalf("relation calls = %d, value calls = %d", station.relationCalls, station.valueCalls)
	}
}

func TestExtractRoleplayOngoingActionPreservesPreviousInCode(t *testing.T) {
	station := &recordingRoleplayOngoingActionStation{
		relation:        assemblyline.RoleplayOngoingActionUnchanged,
		relationReceipt: objectiveStationReceipt{Calls: 1},
	}
	previous := "Crossing the bridge."
	result, calls, err := extractRoleplayOngoingAction(
		context.Background(), station,
		assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mira", "Mira keeps moving toward the far bank.", &previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action == nil || *result.Action != previous ||
		result.RequiresRestoration || calls != 1 {
		t.Fatalf("result = %#v, calls = %d; want code-owned previous value and one relation call", result, calls)
	}
	if station.relationCalls != 1 || station.valueCalls != 0 {
		t.Fatalf("relation calls = %d, value calls = %d", station.relationCalls, station.valueCalls)
	}
	*result.Action = "changed by test"
	if previous != "Crossing the bridge." {
		t.Fatal("returned ongoing action aliases the caller's previous-state value")
	}
}

func TestExtractRoleplayOngoingActionGeneratesOnlyForReplacement(t *testing.T) {
	station := &recordingRoleplayOngoingActionStation{
		relation:        assemblyline.RoleplayOngoingActionReplacement,
		value:           "Opening the gate.",
		relationReceipt: objectiveStationReceipt{Calls: 1},
		valueReceipt:    objectiveStationReceipt{Calls: 1},
	}
	previous := "Crossing the bridge."
	const contribution = "Mira steps off the bridge and begins opening the gate."
	result, calls, err := extractRoleplayOngoingAction(
		context.Background(), station,
		assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mira", contribution, &previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action == nil || *result.Action != station.value ||
		!result.RequiresRestoration || calls != 2 {
		t.Fatalf("result = %#v, calls = %d; want replacement and two bounded calls", result, calls)
	}
	if station.relationCalls != 1 || station.valueCalls != 1 {
		t.Fatalf("relation calls = %d, value calls = %d", station.relationCalls, station.valueCalls)
	}
	if station.valueInput.CharacterName != "Mira" ||
		station.valueInput.Source != assemblyline.RoleplayOngoingActionSourceAssistantResponse ||
		station.valueInput.ExactContribution != contribution {
		t.Fatalf("unexpected value input: %#v", station.valueInput)
	}
}

func TestExtractRoleplayOngoingActionKeepsIdenticalValueCodeOwned(t *testing.T) {
	const previous = "Crossing the bridge."
	station := &recordingRoleplayOngoingActionStation{
		relation:        assemblyline.RoleplayOngoingActionReplacement,
		value:           previous,
		relationReceipt: objectiveStationReceipt{Calls: 1},
		valueReceipt:    objectiveStationReceipt{Calls: 1},
	}
	result, calls, err := extractRoleplayOngoingAction(
		context.Background(), station,
		assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		"Mira", "Mira begins crossing the bridge again.", ongoingActionPointer(previous),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action == nil || *result.Action != previous ||
		result.RequiresRestoration || calls != 2 {
		t.Fatalf("result = %#v, calls = %d; identical bytes must remain code-owned", result, calls)
	}
}

func ongoingActionPointer(value string) *string {
	return &value
}
