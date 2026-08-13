package artifacts

import (
	"os"
	"strings"
	"testing"
)

func TestEnvelopeValidatesExactGenericPersistenceBoundary(t *testing.T) {
	t.Parallel()

	envelope, err := MarshalPayload("bounded_result", "1", map[string]string{"value": "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayload[map[string]string](envelope)
	if err != nil {
		t.Fatal(err)
	}
	if decoded["value"] != "accepted" {
		t.Fatalf("decoded=%v", decoded)
	}
	for _, invalid := range []Envelope{
		{Kind: " broad", Version: "1", Payload: []byte(`{}`)},
		{Kind: "broad", Version: " 1", Payload: []byte(`{}`)},
		{Kind: "broad", Version: "1", Payload: []byte(`null`)},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid envelope accepted: %#v", invalid)
		}
	}
}

func TestRejectedRuntimeArtifactDTOsAreAbsent(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"intent_validation.go"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("rejected artifact file %s still exists or cannot be checked: %v", name, err)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
	}
	for _, rejected := range []string{
		"IntentArtifact", "PlanArtifact", "Subtask", "ToolCallArtifact",
		"ToolResultArtifact", "AnalysisArtifact", "VerificationArtifact",
		"tool_call", "tool_result", "allowed_tools", "recommended_actions",
	} {
		if strings.Contains(source.String(), rejected) {
			t.Fatalf("rejected runtime artifact %q remains", rejected)
		}
	}
}
