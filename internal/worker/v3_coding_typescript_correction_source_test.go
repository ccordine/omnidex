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
	start := strings.Index(source, "if guidance != \"\" {")
	if start < 0 {
		t.Fatal("guided TypeScript correction boundary is absent")
	}
	envelope := source[start:]
	if end := strings.Index(envelope, "\n\tdiagnostic :="); end >= 0 {
		envelope = envelope[:end]
	}
	for _, forbidden := range []string{
		"Path:", "File:", "Workspace:", "RequirementQuote:", "TestSource:", "TestCode:",
		"job.failure", "Capabilities:", "PermittedSymbols:", "RequiredChange:", "Diagnostic:",
	} {
		if strings.Contains(envelope, forbidden) {
			t.Fatalf("TypeScript correction envelope contains forbidden authority %q", forbidden)
		}
	}
	if !strings.Contains(envelope, "RepairGuidance: guidance") {
		t.Fatal("guided TypeScript executor omitted its sole semantic repair authority")
	}
	if strings.Contains(source, "directCodingTypeScriptTestModelFailure(job.failure)") ||
		!strings.Contains(source, "diagnostic := strings.TrimSpace(job.failure)") ||
		!strings.Contains(source, "directCodingTypeScriptCompilerContainsPathIdentity(diagnostic)") ||
		!strings.Contains(source, "Diagnostic:         diagnostic") {
		t.Fatal("TypeScript correction diagnostic bypasses exact path-free authority")
	}
}
