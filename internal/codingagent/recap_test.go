package codingagent

import (
	"strings"
	"testing"
)

func TestTaskLedgerCompletionRecapUsesAcceptedTaskEvidence(t *testing.T) {
	entries := []Entry{
		{Phase: "files_written", WrittenFiles: []string{"first.go", "first.go"}},
		{Phase: "task_accepted", TaskID: "first", Checks: []CheckResult{{ID: "first-check"}}},
		{Phase: "task_accepted", TaskID: "second"},
	}
	recap, err := TaskLedgerCompletionRecap(entries, 2, 1, 1024)
	if err != nil || !strings.Contains(recap, "Tasks completed: 2. Checks passed: 1. Independent task reviews passed.") || strings.Count(recap, "first.go") != 1 {
		t.Fatalf("task-ledger recap = %q, %v", recap, err)
	}
}

func TestTaskLedgerCompletionRecapRequiresAuthoritativeCounts(t *testing.T) {
	if _, err := TaskLedgerCompletionRecap(nil, 0, 0, 1024); err == nil {
		t.Fatal("recap claimed completion without authoritative task counts")
	}
	recap, err := TaskLedgerCompletionRecap(nil, 1, 1, 1024)
	if err != nil || !strings.Contains(recap, "Tasks completed: 1. Checks passed: 1.") {
		t.Fatalf("carried task recap = %q, %v", recap, err)
	}
}
