package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectedIntentAndDelegationAuthorityIsAbsent(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"repository_accepted_intent.go",
		"repository_delegated_steps.go",
		"task_artifact_projection_store.go",
		"task_artifact_projection_types.go",
		"task_ledger_intent_projection.go",
		"task_ledger_intent_terminal.go",
	} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("rejected queue authority %s still exists or cannot be checked: %v", name, err)
		}
	}
}

func TestQueueProductionHasNoRejectedRuntimeActions(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, rejected := range []string{
			"v3_intent_parse", "v3_planning", "v3_subtask",
			"acceptedIntent", "AcceptedIntent", "delegatedSubtask",
		} {
			if strings.Contains(string(source), rejected) {
				t.Fatalf("rejected queue authority %q remains in %s", rejected, path)
			}
		}
	}
}
