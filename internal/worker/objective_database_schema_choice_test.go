package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDatabaseSchemaChoiceRemovesAcceptedRelationsAndRetainsStopChoice(t *testing.T) {
	input := databaseSchemaChoiceFixture(3, 3)
	rawChoices := []string{"B", "A", "B"}
	wantDescriptions := [][]string{
		{"relation one", "relation two", "relation three"},
		{"relation one", "relation three"},
		{"relation three"},
	}
	callIndex := 0
	call := func(
		_ context.Context,
		subject string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		if callIndex >= len(rawChoices) {
			return nil, 1, fmt.Errorf("unexpected extra relation choice %q", subject)
		}
		prompt, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			return nil, 1, err
		}
		for _, description := range []string{"relation one", "relation two", "relation three"} {
			want := containsDatabaseSchemaDescription(wantDescriptions[callIndex], description)
			if strings.Contains(prompt, description) != want {
				return nil, 1, fmt.Errorf(
					"round %d description %q presence = %t, want %t",
					callIndex+1, description, strings.Contains(prompt, description), want,
				)
			}
		}
		if !strings.Contains(prompt, "No additional remaining relation is necessary") {
			return nil, 1, fmt.Errorf("round %d omitted the no-additional choice", callIndex+1)
		}
		for index := 1; index <= 3; index++ {
			if strings.Contains(prompt, fmt.Sprintf("relation-id-%d", index)) {
				return nil, 1, fmt.Errorf("round %d exposed relation ID %d", callIndex+1, index)
			}
		}
		raw := rawChoices[callIndex]
		callIndex++
		value, err := decode(raw)
		return value, 1, err
	}

	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatalf("resolve iterative relation choices: %v", err)
	}
	if calls != 3 || callIndex != 3 {
		t.Fatalf("relation choice calls = reported %d actual %d, want 3", calls, callIndex)
	}
	wantIDs := []string{"relation-id-2", "relation-id-1"}
	if fmt.Sprint(decision.RelationIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("selected relations = %v, want %v", decision.RelationIDs, wantIDs)
	}
}

func TestDatabaseSchemaChoiceStopsAtCodeOwnedSelectionBoundWithoutAnotherCall(t *testing.T) {
	input := databaseSchemaChoiceFixture(2, 1)
	callIndex := 0
	call := func(
		_ context.Context,
		_ string,
		_ assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		callIndex++
		value, err := decode("A")
		return value, 1, err
	}

	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(context.Background(), input, call)
	if err != nil {
		t.Fatalf("bounded selection: %v", err)
	}
	if calls != 1 || callIndex != 1 {
		t.Fatalf("bounded selection calls = reported %d actual %d, want 1", calls, callIndex)
	}
	if len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "relation-id-1" {
		t.Fatalf("bounded selected relations = %v, want relation-id-1", decision.RelationIDs)
	}
}

func TestDatabaseSchemaChoiceComparesSoleRemainingRelationAfterAcceptedSelection(t *testing.T) {
	input := databaseSchemaChoiceFixture(1, 1)
	input.HasAcceptedRelations = true
	callIndex := 0
	call := func(
		_ context.Context,
		_ string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		callIndex++
		prompt, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			return nil, 1, err
		}
		if !strings.Contains(prompt, "A. relation one") ||
			!strings.Contains(prompt, "B. No additional remaining relation") {
			return nil, 1, fmt.Errorf("sole remaining round did not contain relation and stop choices:\n%s", prompt)
		}
		value, err := decode("B")
		return value, 1, err
	}

	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatalf("resolve sole remaining relation comparison: %v", err)
	}
	if calls != 1 || callIndex != 1 {
		t.Fatalf("sole remaining comparison calls = reported %d actual %d, want 1", calls, callIndex)
	}
	if len(decision.RelationIDs) != 0 {
		t.Fatalf("sole remaining stop selected relations = %v, want none", decision.RelationIDs)
	}
}

func databaseSchemaChoiceFixture(candidateCount, maximum int) assemblyline.DatabaseSchemaSelectionInput {
	names := []string{"one", "two", "three"}
	candidates := make([]assemblyline.DatabaseSchemaCandidate, candidateCount)
	for index := range candidates {
		candidates[index] = assemblyline.DatabaseSchemaCandidate{
			RelationID: fmt.Sprintf("relation-id-%d", index+1),
			Descriptor: fmt.Sprintf("relation %s", names[index]),
		}
	}
	return assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Answer the database objective.",
		Context:        assemblyline.ObjectiveContext{},
		Candidates:     candidates,
		MaxSelections:  maximum,
	}
}

func containsDatabaseSchemaDescription(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
