package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestObjectiveCallCountsRetainOnlyActualBounds(t *testing.T) {
	for _, limit := range []int{0, 1, 7} {
		for _, calls := range []int{0, limit} {
			if err := validateObjectiveCallCount("fixture", calls, limit); err != nil {
				t.Fatalf("valid count %d of %d: %v", calls, limit, err)
			}
		}
		for _, calls := range []int{-1, limit + 1} {
			if err := validateObjectiveCallCount("fixture", calls, limit); err == nil {
				t.Fatalf("invalid count %d of %d accepted", calls, limit)
			}
		}
	}
	if err := validateObjectiveCallCount("fixture", 0, -1); err == nil {
		t.Fatal("negative call bound accepted")
	}
}

func TestObjectiveZeroCallsDoNotAuthorizeInvalidSemanticValues(t *testing.T) {
	conversation := usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 0, nil
	})
	result, err := runObjectiveConversationResponse(context.Background(),
		turnAuthority{ModelInstruction: "Answer the current question."}, objectiveTurnResult{Kind: assemblyline.ObjectiveKindAnswer}, conversation, "fixture")
	if err == nil || !strings.Contains(err.Error(), "schema") || result.Complete || result.Output != "" {
		t.Fatalf("invalid response bypassed semantic validation: %+v / %v", result, err)
	}
	canon := usageCanonFunc(func(assemblyline.RoleplayCanonExtractionInput) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
		return assemblyline.RoleplayCanonExtractionDecision{}, 0, nil
	})
	if facts, calls, err := extractRoleplayCanonSource(context.Background(), canon, roleplayCanonWorkerTestInput()); err == nil || !strings.Contains(err.Error(), "schema") || facts != nil || calls != 0 {
		t.Fatalf("invalid canon bypassed semantic validation: facts=%v calls=%d / %v", facts, calls, err)
	}
}

func TestObjectiveStationReceiptAndReuseGatesAreRemoved(t *testing.T) {
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
		for _, forbidden := range []string{"objectiveStationReceipt", "validateObjectiveStationReceipt", "validateObjectiveBoundedStationReceipt", "validateRoleplayCanonExtractionReceipt", "validateObjectiveGroundedAnswerReceipt", ".Reused", "allReused"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains removed station-call proof machinery %q", entry.Name(), forbidden)
			}
		}
	}
}
