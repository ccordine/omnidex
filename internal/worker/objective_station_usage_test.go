package worker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestObjectiveStationZeroCallsNeedNoReuseReceipt(t *testing.T) {
	input, _ := repositoryGroundedSelectionFixture()
	for _, validate := range []struct {
		name string
		run  func() error
	}{
		{"single leaf", func() error { return validateObjectiveLeafCallCount("fixture leaf", 0) }},
		{"bounded leaves", func() error { return validateObjectiveCallCount("fixture leaves", 0, 3) }},
		{"grounded answer", func() error { return validateObjectiveGroundedAnswerCalls(0, input) }},
		{"canon", func() error { return validateRoleplayCanonExtractionCalls(0) }},
	} {
		t.Run(validate.name, func(t *testing.T) {
			if err := validate.run(); err != nil {
				t.Fatalf("zero dispatch required a separate reuse assertion: %v", err)
			}
		})
	}
}

func TestObjectiveConversationConsumesValuesAndCountsFailedCalls(t *testing.T) {
	for _, answer := range []string{"The inspection is Tuesday.", "Use French for display text."} {
		for _, calls := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", answer, calls), func(t *testing.T) {
				authority := turnAuthority{ModelInstruction: "Answer using the retained context."}
				initial := objectiveTurnResult{Kind: assemblyline.ObjectiveKindAnswer, ModelCalls: 2}
				station := usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
					value, err := assemblyline.DecodeConversationResponseDecision(input, answer)
					return value, calls, err
				})
				result, err := runObjectiveConversationResponse(context.Background(), authority, initial, station, "fixture-model")
				if err != nil || !result.Complete || result.Output != answer || result.ModelCalls != 2+calls {
					t.Fatalf("valid response blocked by usage metadata: %+v / %v", result, err)
				}
				failure := errors.New("observed provider failure")
				station = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
					return assemblyline.ConversationResponseDecision{}, calls, failure
				})
				result, err = runObjectiveConversationResponse(context.Background(), authority, initial, station, "fixture-model")
				if !errors.Is(err, failure) || result.Complete || result.Output != "" || result.ModelCalls != 2+calls {
					t.Fatalf("failed response lost actual usage or published output: %+v / %v", result, err)
				}
			})
		}
	}
}

func TestObjectiveCanonConsumesValuesAndCountsFailedCalls(t *testing.T) {
	input := roleplayCanonWorkerTestInput()
	for _, calls := range []int{0, 1} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			station := usageCanonFunc(func(input assemblyline.RoleplayCanonExtractionInput) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
				value, err := assemblyline.AssembleRoleplayCanonExtractionDecision(input, []string{})
				return value, calls, err
			})
			facts, observed, err := extractRoleplayCanonSource(context.Background(), station, input)
			if err != nil || facts == nil || len(facts) != 0 || observed != calls {
				t.Fatalf("valid absence required inference metadata: facts=%v calls=%d / %v", facts, observed, err)
			}
			failure := errors.New("observed canon provider failure")
			station = usageCanonFunc(func(assemblyline.RoleplayCanonExtractionInput) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
				return assemblyline.RoleplayCanonExtractionDecision{}, calls, failure
			})
			facts, observed, err = extractRoleplayCanonSource(context.Background(), station, input)
			if !errors.Is(err, failure) || facts != nil || observed != calls {
				t.Fatalf("failed canon lost actual usage: facts=%v calls=%d / %v", facts, observed, err)
			}
		})
	}
}

func TestObjectiveOngoingActionConsumesValuesAndCountsInvalidOutput(t *testing.T) {
	for _, action := range []string{"Crossing the bridge.", "Reading the catalogue."} {
		for _, calls := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", action, calls), func(t *testing.T) {
				station := &recordingRoleplayOngoingActionStation{
					relation: assemblyline.RoleplayOngoingActionReplacement, value: action,
					relationDispatches: calls, valueDispatches: calls,
				}
				result, observed, err := extractRoleplayOngoingAction(context.Background(), station,
					assemblyline.RoleplayOngoingActionSourceAssistantResponse, "Mira", "Mira continues her activity.", nil)
				if err != nil || result.Action == nil || *result.Action != action || observed != 2*calls {
					t.Fatalf("valid action blocked by usage metadata: %+v calls=%d / %v", result, observed, err)
				}
				station.value = ""
				result, observed, err = extractRoleplayOngoingAction(context.Background(), station,
					assemblyline.RoleplayOngoingActionSourceAssistantResponse, "Mira", "Mira continues her activity.", nil)
				if err == nil || result.Action != nil || observed != 2*calls {
					t.Fatalf("invalid action lost usage or became accepted: %+v calls=%d / %v", result, observed, err)
				}
			})
		}
	}
}

func TestObjectiveDatabaseReadKeepsAcquisitionAndAnswerFailureUsage(t *testing.T) {
	for _, phase := range []string{"acquisition", "answer"} {
		t.Run(phase, func(t *testing.T) {
			failure := errors.New("observed database workflow failure")
			citation, err := newObjectiveEvidence("DB-01", `{"value":12}`, "postgres_query", "database:fixture/query/41")
			if err != nil {
				t.Fatal(err)
			}
			citation.DatabaseEvidenceID, citation.DatabaseRowEnd = 41, 1
			citation.ObservedAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			authority := turnAuthority{JobID: 17, DataSourceID: "source-1", ModelInstruction: "Return the measured value."}
			initial := objectiveTurnResult{ObjectiveID: "objective-17-1", RequirementID: "objective-17-1-requirement", Kind: assemblyline.ObjectiveKindDatabaseRead, ModelCalls: 1}
			resolve := func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error) {
				result := objectiveEvidenceAcquisition{ModelCalls: 2}
				if phase == "acquisition" {
					return result, failure
				}
				result.Evidence = []objectiveEvidence{citation}
				return result, nil
			}
			answer := usageGroundedAnswerFunc(func(assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, int, error) {
				if phase != "answer" {
					t.Fatal("acquisition failure invoked an answer station")
				}
				return assemblyline.GroundedAnswerDecision{}, 3, failure
			})
			result, err := runObjectiveDatabaseRead(context.Background(), authority, initial, answer, resolve)
			wantCalls := 3
			if phase == "answer" {
				wantCalls = 6
			}
			if !errors.Is(err, failure) || result.ModelCalls != wantCalls || result.Complete || result.Output != "" || len(result.Citations) != 0 {
				t.Fatalf("failed %s lost usage or published an answer: %+v / %v", phase, result, err)
			}
		})
	}
}

type usageConversationFunc func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error)

func (respond usageConversationFunc) Respond(_ context.Context, input assemblyline.ConversationResponseInput, _ string) (assemblyline.ConversationResponseDecision, int, error) {
	return respond(input)
}

type usageCanonFunc func(assemblyline.RoleplayCanonExtractionInput) (assemblyline.RoleplayCanonExtractionDecision, int, error)

func (extract usageCanonFunc) ExtractCanon(_ context.Context, input assemblyline.RoleplayCanonExtractionInput) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
	return extract(input)
}

type usageGroundedAnswerFunc func(assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, int, error)

func (answer usageGroundedAnswerFunc) Answer(_ context.Context, input assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, int, error) {
	return answer(input)
}
