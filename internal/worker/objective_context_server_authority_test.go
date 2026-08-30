package worker

import (
	"os"
	"strings"
	"testing"
)

func TestContextRelevanceHasOnlyTheDurableExactStationExecutionPath(t *testing.T) {
	stationSource, err := os.ReadFile("objective_context_sieve_stations.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stationSource), "runObjectiveReusablePortableRawLeafCall(") ||
		strings.Contains(string(stationSource), "runObjectivePortableRawLeafCall(") {
		t.Fatal("context relevance bypasses durable accepted-result reuse")
	}
	for _, forbidden := range []string{
		"ExecuteContextRelevance", "browserContextRelevance", "ContextRelevanceExecutor",
	} {
		if strings.Contains(string(stationSource), forbidden) {
			t.Errorf("context relevance station retains alternate executor %q", forbidden)
		}
	}

	reuseSource, err := os.ReadFile("objective_reusable_raw_leaf_call.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reuseSource), "runObjectivePortableRawLeafCall(") {
		t.Fatal("accepted-result reuse omitted the durable exact station fallback")
	}

	engineSource, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"BrowserContextRelevance", "browserContextRelevance", "ContextRelevanceExecutor",
	} {
		if strings.Contains(string(engineSource), forbidden) {
			t.Errorf("worker engine retains alternate executor authority %q", forbidden)
		}
	}
}
