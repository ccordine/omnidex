package codingagent

import (
	"strings"
	"testing"
)

func TestCompletionRecapReportsObservedWorkAndCurrentChecks(t *testing.T) {
	entries := []Entry{
		{Phase: "plan_frozen", Workload: &Workload{Tasks: []Task{{ID: "update", WriteFiles: []string{"changed.py", "untouched.py"}}}}},
		{Phase: "files_written", WrittenFiles: []string{"changed.py", "changed.py"}},
		{Phase: "task_accepted", TaskID: "update", Checks: []CheckResult{{ID: "old"}}},
		{Phase: "task_reopened", TaskID: "update"},
		{Phase: "task_accepted", TaskID: "update", Checks: []CheckResult{{ID: "unit"}, {ID: "behavior"}}},
		{Phase: "overall_accepted", Checks: []CheckResult{{ID: "integration"}}},
	}
	recap, err := CompletionRecap(entries, 1024)
	if err != nil || !strings.Contains(recap, "Tasks completed: 1. Checks passed: 3") || strings.Count(recap, "changed.py") != 1 || strings.Contains(recap, "untouched.py") {
		t.Fatalf("recap invented or lost observed work: %q %v", recap, err)
	}
	for _, incomplete := range [][]Entry{entries[:5], append(entries[:4:4], entries[5])} {
		if _, err := CompletionRecap(incomplete, 1024); err == nil {
			t.Fatal("recap claimed completion before all accepted work")
		}
	}
}

func TestCompletionRecapBoundsPresentationWithoutLosingTotal(t *testing.T) {
	entries := []Entry{
		{Phase: "plan_frozen", Workload: &Workload{Tasks: []Task{{ID: "update"}}}},
		{Phase: "files_written", WrittenFiles: []string{strings.Repeat("a", 200), strings.Repeat("b", 200)}},
		{Phase: "task_accepted", TaskID: "update"}, {Phase: "overall_accepted"},
	}
	recap, err := CompletionRecap(entries, 256)
	if err != nil || len(recap) > 256 || !strings.Contains(recap, "2 additional written files") {
		t.Fatalf("recap presentation changed job completion: %q %v", recap, err)
	}
}
