package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRunningStepIdentityHasNoRequeueFallback(t *testing.T) {
	paths := []string{
		"repository_interrupt.go",
		"repository_stale_leases.go",
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
	migration, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), "job step execution identity cannot return to pending") {
		t.Fatal("PostgreSQL does not reject running-step identity reuse")
	}
}
