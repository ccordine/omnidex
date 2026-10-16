package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestChunkedContextArtifactBindingsAreNotLimitedToOneStationEnvelope(t *testing.T) {
	var source strings.Builder
	for index := range 100 {
		fmt.Fprintf(&source, "Required file /opt/project/item-%d.go\n", index)
	}
	encoded, _ := json.Marshal(source.String())
	redacted, bindings, err := RedactContextArtifactJSON(encoded)
	if err != nil || len(bindings) != 100 {
		t.Fatalf("context inherited a single-envelope artifact ceiling: %d %v", len(bindings), err)
	}
	restored, err := RestoreContextArtifactJSONOrText(redacted, bindings)
	var text string
	decodeErr := json.Unmarshal([]byte(restored), &text)
	if err != nil || decodeErr != nil || text != source.String() {
		t.Fatalf("context identities were not restored exactly: %v", err)
	}
	if _, _, err := RedactArtifactIdentities(source.String(), ArtifactIdentityProvenance{}); err == nil {
		t.Fatal("context batching weakened the ordinary per-envelope safeguard")
	}
}
