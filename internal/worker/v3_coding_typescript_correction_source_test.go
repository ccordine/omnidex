package worker

import (
	"os"
	"strings"
	"testing"
)

func TestTypeScriptCorrectionEnvelopeHasNoPathOrWorkflowAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_typescript_fragment_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "return assemblyline.NewFragmentCorrectionJob(")
	if start < 0 {
		t.Fatal("TypeScript correction envelope constructor is absent")
	}
	envelope := source[start:]
	if end := strings.Index(envelope, "\n\t})"); end >= 0 {
		envelope = envelope[:end]
	}
	for _, forbidden := range []string{
		"Path:", "File:", "Workspace:", "RequirementQuote:", "TestSource:", "TestCode:",
		"job.failure",
	} {
		if strings.Contains(envelope, forbidden) {
			t.Fatalf("TypeScript correction envelope contains forbidden authority %q", forbidden)
		}
	}
	if !strings.Contains(source, "directCodingTypeScriptModelFailure(job.failure)") ||
		!strings.Contains(envelope, "Diagnostic:         diagnostic") {
		t.Fatal("TypeScript correction diagnostic bypasses the path-blind sanitizer")
	}
}
