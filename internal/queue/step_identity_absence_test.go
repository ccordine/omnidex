package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRunningStepIdentityHasNoRequeueFallback(t *testing.T) {
	paths := []string{
		"repository_interrupt.go",
		"repository_step_claim.go",
		"step_attempt_authority.go",
		"../worker/engine.go",
	}
	var source strings.Builder
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
		source.WriteByte('\n')
	}
	for _, forbidden := range []string{
		"RecoverStaleV3Steps",
		"step_interrupted",
		"interrupted and re-queued",
		"StepStatusPending || stepStatus",
	} {
		if strings.Contains(source.String(), forbidden) {
			t.Fatalf("step authority contains forbidden identity-reuse path %q", forbidden)
		}
	}
	setup, err := os.ReadFile("../../database/setup.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(setup), "step attempt identity is immutable") ||
		!strings.Contains(string(setup), "step attempt must increase monotonically by one") {
		t.Fatal("PostgreSQL does not enforce monotonic immutable step attempts")
	}
}
