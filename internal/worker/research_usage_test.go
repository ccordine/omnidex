package worker

import (
	"os"
	"strings"
	"testing"
)

func TestResearchDoesNotRequireWebCallProofLedgers(t *testing.T) {
	for _, path := range []string{"objective_roleplay_research.go", "objective_turn_types.go", "objective_turn_workflow.go", "objective_web_workflow.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"SemanticCallLedger", "SemanticCallReceipt", "CallLedger", "RelevanceCalls"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains call-proof machinery %q", path, forbidden)
			}
		}
	}
}
