package webresearch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestWebProjectionUsesSelectedValuesWithoutDuplicateCallLedger(t *testing.T) {
	for _, subject := range []string{"Inspection schedule", "Library classification"} {
		for _, calls := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", subject, calls), func(t *testing.T) {
				observed := []Evidence{{
					ID: "evidence_1", CandidateID: "candidate_1", URL: "https://source.example/document",
					Title: subject, Content: subject + " is documented here.",
					ObservedAt: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC),
				}}
				machine := evidenceMachine{
					objective: Objective{Question: "What is documented about this subject?"},
					config:    EvidenceConfig{MaxRelevantCandidates: 1, MaxProjectionBytes: 1024, CandidateSummaryBytes: 256},
					relevance: webUsageRelevanceFunc(func(call RelevanceCall) (RelevanceDecision, error) {
						return RelevanceDecision{Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{call.Candidates[0].CandidateID}, SemanticCalls: calls}, nil
					}),
				}
				var result evidenceRun
				projected, relevant, err := machine.selectAndProject(context.Background(), observed, &result)
				if err != nil || !relevant || len(projected) != 1 || projected[0].Content != observed[0].Content || result.SemanticCalls != calls {
					t.Fatalf("projected=%+v relevant=%t calls=%d error=%v", projected, relevant, result.SemanticCalls, err)
				}
			})
		}
	}
}

func TestPortableWebResultNeedsDecodedTextNotPositiveInference(t *testing.T) {
	stations, err := NewPortableStations(PortableRuntime{Resolve: func(_ context.Context, _ assemblyline.PortableJob, validate PortableCandidateValidator) (int, error) {
		return 0, validate("A")
	}})
	if err != nil {
		t.Fatal(err)
	}
	call := portableEvidenceSelectionCall()
	call.MaxSelections = 1
	decision, err := stations.Select(context.Background(), call)
	if err != nil || decision.SemanticCalls != 0 || !reflect.DeepEqual(decision.CandidateIDs, []websearch.CandidateID{call.Candidates[0].CandidateID}) {
		t.Fatalf("decoded zero-call result=%+v error=%v", decision, err)
	}
}

func TestPortableWebResolverCannotSkipRepeatOrIgnoreDecoding(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		invoke func(PortableCandidateValidator)
	}{
		{"missing", func(PortableCandidateValidator) {}},
		{"repeated", func(validate PortableCandidateValidator) { _ = validate("A"); _ = validate("B") }},
		{"ignored error", func(validate PortableCandidateValidator) { _ = validate("invalid") }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			stations, err := NewPortableStations(PortableRuntime{Resolve: func(_ context.Context, _ assemblyline.PortableJob, validate PortableCandidateValidator) (int, error) {
				calls++
				fixture.invoke(validate)
				return 1, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			decision, err := stations.Select(context.Background(), portableEvidenceSelectionCall())
			if err == nil || calls != 1 || decision.CandidateIDs != nil {
				t.Fatalf("invalid resolver consumed a value: decision=%+v calls=%d error=%v", decision, calls, err)
			}
		})
	}
}

func TestPortableWebFailureRetainsActualCallCount(t *testing.T) {
	calls := 0
	stations, err := NewPortableStations(PortableRuntime{Resolve: func(_ context.Context, _ assemblyline.PortableJob, validate PortableCandidateValidator) (int, error) {
		calls++
		if calls == 1 {
			return 1, validate("A")
		}
		return 1, validate("invalid")
	}})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := stations.Select(context.Background(), portableEvidenceSelectionCall())
	if err == nil || calls != 2 || decision.SemanticCalls != 2 || decision.CandidateIDs != nil {
		t.Fatalf("failed selection lost its actual calls: %+v calls=%d error=%v", decision, calls, err)
	}
}

func TestPortableWebCallBoundsRemainEnforced(t *testing.T) {
	for _, reported := range []int{-1, 2} {
		t.Run(fmt.Sprint(reported), func(t *testing.T) {
			stations, err := NewPortableStations(PortableRuntime{Resolve: func(_ context.Context, _ assemblyline.PortableJob, validate PortableCandidateValidator) (int, error) {
				return reported, validate("A")
			}})
			if err != nil {
				t.Fatal(err)
			}
			decision, err := stations.Select(context.Background(), portableEvidenceSelectionCall())
			if err == nil || !strings.Contains(err.Error(), "outside 0..1") || decision.CandidateIDs != nil {
				t.Fatalf("invalid dispatch count was consumed: %+v / %v", decision, err)
			}
		})
	}
}

func TestWebProjectionKeepsFailureUsageAndRejectsUnknownSource(t *testing.T) {
	observed := []Evidence{{ID: "evidence_1", CandidateID: "candidate_1", URL: "https://source.example/document",
		Title: "Timetable", Content: "The train departs at nine.", ObservedAt: time.Now().UTC()}}
	machine := evidenceMachine{objective: Objective{Question: "When does the train depart?"},
		config: EvidenceConfig{MaxRelevantCandidates: 1, MaxProjectionBytes: 1024, CandidateSummaryBytes: 256}}
	failure := errors.New("exact candidate interpretation failed")
	machine.relevance = webUsageRelevanceFunc(func(RelevanceCall) (RelevanceDecision, error) {
		return RelevanceDecision{SemanticCalls: 1}, failure
	})
	var result evidenceRun
	if projected, _, err := machine.selectAndProject(context.Background(), observed, &result); !errors.Is(err, failure) || result.SemanticCalls != 1 || projected != nil {
		t.Fatalf("failure lost actual usage: %+v / %v", result, err)
	}
	machine.relevance = webUsageRelevanceFunc(func(RelevanceCall) (RelevanceDecision, error) {
		return RelevanceDecision{Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{"absent"}}, nil
	})
	result = evidenceRun{}
	if projected, _, err := machine.selectAndProject(context.Background(), observed, &result); !errors.Is(err, ErrInvalidRelevance) || projected != nil {
		t.Fatalf("zero-call selection invented an acquired source: %+v / %v", projected, err)
	}
}

func TestWebCallProofLedgersAndReceiptsAreRemoved(t *testing.T) {
	for _, removed := range []string{"semantic_call_ledger.go", "attempts.go", "fetch_attempt.go",
		"../specialistworkflow/runner.go", "../specialistworkflow/attempt.go", "../specialistworkflow/contract.go",
		"../specialistworkflow/identity.go", "../specialistworkflow/registry.go"} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("obsolete acquisition or call-proof file %s remains: %v", removed, err)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"SemanticCallLedger", "SemanticCallReceipt", "ValidateSemanticCallReceipt", "CallLedger", "RelevanceCalls",
			"specialistworkflow", "acquisitionContracts", "AcquisitionAttempts", "AcquisitionAttemptLimit", "DiscoveryAttempts", "FetchAttempts",
			"StepInitialDiscovery", "StepDocumentsFetched", "StepRelevanceResolved", "StepEvidenceProjected"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains duplicate call-proof machinery %q", entry.Name(), forbidden)
			}
		}
	}
}

type webUsageRelevanceFunc func(RelevanceCall) (RelevanceDecision, error)

func (selectEvidence webUsageRelevanceFunc) Select(_ context.Context, call RelevanceCall) (RelevanceDecision, error) {
	return selectEvidence(call)
}
