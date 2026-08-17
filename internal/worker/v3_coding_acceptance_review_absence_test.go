package worker

import (
	"os"
	"strings"
	"testing"
)

func TestStagedVerificationDoesNotInvokeAcceptanceReviewBeforeReality(t *testing.T) {
	source, err := os.ReadFile("v3_coding_typescript_stage_workspace.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "ensureDirectCodingAcceptanceGrounding(program)") {
		t.Fatal("staged verification must run deterministic staging and reality, not a ceremonial acceptance review")
	}
}
