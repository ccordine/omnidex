package worker

import (
	"os"
	"strings"
	"testing"
)

func TestProviderContractValidationFollowsPersistedNamedGap(t *testing.T) {
	engine, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(engine), ".RequireExactPreparedContract()") {
		t.Fatal("worker construction still executes provider contract validation")
	}

	generation, err := os.ReadFile("llm_generation.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(generation)
	persisted := strings.Index(text, "s.repo.OpenStationGapDiscovery(")
	contract := strings.Index(text, "s.stationClient.RequireExactPreparedContract()")
	discovery := strings.Index(text, "s.stationClient.DiscoverProviderIdentityEvidence(")
	if persisted < 0 || contract < 0 || discovery < 0 ||
		!(persisted < contract && contract < discovery) {
		t.Fatalf("provider order persisted=%d contract=%d discovery=%d", persisted, contract, discovery)
	}
}
