package worker

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestDatabaseRawLeafConsumesSoleChoiceWithoutCallProof(t *testing.T) {
	for _, relation := range []string{"measurements", "shipments"} {
		t.Run(relation, func(t *testing.T) {
			input := databaseSingleChoiceIntentInput()
			input.SchemaProjection.Relations[0].ID = relation
			state := assemblyline.NewDatabaseQueryIntentLeafState(input)
			job, err := assemblyline.NewDatabaseQueryFromRelationJob(state)
			if err != nil {
				t.Fatal(err)
			}
			resolveModel := func() (string, error) {
				t.Fatal("sole choice prepared a model")
				return "", fmt.Errorf("unexpected model resolution")
			}
			call := (portableObjectiveDatabaseStations{}).rawLeafCall(
				station.DatabaseQueryIntent, resolveModel,
			)
			value, calls, err := callObjectiveDatabaseRawLeaf(
				context.Background(), call, "sole relation", job,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryFromRelationLeaf(state, raw)
				},
			)
			if err != nil || value != relation || calls != 0 {
				t.Fatalf("deterministic relation=%q calls=%d error=%v", value, calls, err)
			}
		})
	}
}

func TestDatabaseCallProofLedgersAreRemoved(t *testing.T) {
	if _, err := os.Stat("objective_database_call_ledger.go"); !os.IsNotExist(err) {
		t.Fatalf("obsolete database call-proof ledger file remains: %v", err)
	}
	for _, name := range []string{
		"objective_database_stations.go", "objective_database_selection.go", "objective_database_compile.go",
		"objective_database_workflow.go", "objective_database_raw_leaf.go", "objective_turn_types.go", "objective_turn_workflow.go",
	} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"objectiveDatabaseRawLeafCallLedger", "objectiveDatabaseBoundedCallLedger",
			"objectiveDatabaseAcquisitionCallLedger", "DatabaseCallLedger", "completeObjectiveDatabaseEvidenceAcquisition",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains duplicate call-proof machinery %q", name, forbidden)
			}
		}
	}
}

func TestDatabaseSchemaReductionAcceptsDeterministicSoleCandidate(t *testing.T) {
	input := databaseSchemaChoiceFixture(1, 1)
	selected, dispatches, err := reduceObjectiveDatabaseCandidates(context.Background(), input.EvidenceNeedID,
		input.ExactNeed, input.Context, input.Candidates, portableObjectiveDatabaseStations{})
	if err != nil || !reflect.DeepEqual(selected, input.Candidates) || dispatches != 0 {
		t.Fatalf("deterministic reduction=%+v usage=%+v error=%v", selected, dispatches, err)
	}
}

// This isolates the response consumer; execution and persisted citation checks
// are exercised separately by the PostgreSQL workflow fixtures.
func TestDatabaseResponseUsesAcquiredEvidenceWithoutDuplicateCallLedger(t *testing.T) {
	for _, calls := range []int{0, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			authority := turnAuthority{
				JobID: 17, Generation: 2, DataSourceID: "source-1",
				ModelInstruction: "Return the measured value.",
			}
			citation, err := newObjectiveEvidence("DB-01", `{"value":12}`, "postgres_query", "database:fixture/query/41")
			if err != nil {
				t.Fatal(err)
			}
			citation.DatabaseEvidenceID, citation.DatabaseRowEnd = 41, 1
			citation.ObservedAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			initial := objectiveTurnResult{
				ObjectiveID: objectiveTurnID(authority), Kind: assemblyline.ObjectiveKindDatabaseRead,
			}
			initial.RequirementID = objectiveRequirementID(initial.ObjectiveID)
			answer := databaseUsageAnswerFunc(func(input assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, error) {
				if !reflect.DeepEqual(input.Evidence, []assemblyline.GroundedEvidenceCapsule{citation.Capsule}) {
					t.Fatalf("answer did not receive the acquired rows: %+v", input.Evidence)
				}
				return assemblyline.AssembleGroundedAnswerDecision(input, []assemblyline.GroundedAnswerParagraph{{
					Text: "The measured value is twelve.", EvidenceIDs: []string{citation.Capsule.ID},
				}})
			})
			resolve := func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error) {
				return objectiveEvidenceAcquisition{Evidence: []objectiveEvidence{citation}, ModelCalls: calls}, nil
			}
			result, err := runObjectiveDatabaseRead(context.Background(), authority, initial, answer, resolve)
			if err != nil || !result.Complete || result.ModelCalls != calls || !reflect.DeepEqual(result.Citations, []objectiveEvidence{citation}) {
				t.Fatalf("database response=%+v error=%v", result, err)
			}
			citation.DatabaseEvidenceID = 0
			if result, err := runObjectiveDatabaseRead(context.Background(), authority, initial, answer, resolve); err == nil || result.Complete {
				t.Fatalf("missing execution reference passed: %+v / %v", result, err)
			}
		})
	}
}

type databaseUsageAnswerFunc func(assemblyline.GroundedAnswerInput) (assemblyline.GroundedAnswerDecision, error)

func (answer databaseUsageAnswerFunc) Answer(_ context.Context, input assemblyline.GroundedAnswerInput) (
	assemblyline.GroundedAnswerDecision, int, error,
) {
	decision, err := answer(input)
	return decision, 0, err
}
