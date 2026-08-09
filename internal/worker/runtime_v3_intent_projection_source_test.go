package worker

import (
	"os"
	"strings"
	"testing"
)

func TestIntentParseHasNoGenericArtifactWriteFallback(t *testing.T) {
	raw, err := os.ReadFile("runtime_v3_native.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "writeArtifact(artifacts.KindIntent") {
		t.Fatal("intent parsing still writes through the generic artifact path")
	}
	if !strings.Contains(source, "writeAcceptedIntentArtifact(intent)") {
		t.Fatal("intent parsing does not use the transactional accepted-intent writer")
	}
}
