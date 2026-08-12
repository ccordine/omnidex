package cognitiongauntlet

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func TestProductionSemanticEvidencePreflightRejectsCombinedContainerOverflow(t *testing.T) {
	sidecars := ProductionSemanticReplaySidecars{
		RuntimeBrainBootstrapEvidence:     []byte{'b'},
		RuntimeProviderActivationEvidence: []byte{'a'},
	}
	inventory := semanticReplayEvidenceInventory{
		policy: make(map[string]semanticPolicyEvidence),
	}
	remaining := cognitionreplay.MaxContainerBytes - 2
	for index := 0; remaining > 0; index++ {
		bytes := cognitionpolicy.MaxModelResponseEvidenceBytes
		if bytes > remaining {
			bytes = remaining
		}
		id := fmt.Sprintf("evidence-%d", index)
		inventory.policy[id] = semanticPolicyEvidence{
			EvidenceKind: "model_response", EvidenceID: id, Bytes: bytes,
		}
		remaining -= bytes
	}
	if err := preflightProductionSemanticEvidence(
		productionTrace{}, inventory, sidecars,
	); err != nil {
		t.Fatalf("exact evidence container cap rejected: %v", err)
	}
	inventory.policy["over-cap"] = semanticPolicyEvidence{
		EvidenceKind: "model_response", EvidenceID: "over-cap", Bytes: 1,
	}
	if err := preflightProductionSemanticEvidence(
		productionTrace{}, inventory, sidecars,
	); err == nil {
		t.Fatal("combined evidence above the replay container cap was accepted")
	}
}
