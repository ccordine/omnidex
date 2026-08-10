package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCognitionEvidencePacketHasNoHistoryOrReadySiblingFallback(t *testing.T) {
	raw, err := os.ReadFile("cognition_runtime_evidence_packet.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"appendRecentCognitionEvidence", "ORDER BY transitions.revision DESC",
		"ObligationReady", "LIMIT $2",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("clean-desk evidence source contains forbidden fallback %q", forbidden)
		}
	}
}
