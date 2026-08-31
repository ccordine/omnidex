package worker

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingWorkspaceForJobPreservesExactClientCWD(t *testing.T) {
	const exactRoot = "/home/example/Projects/calculator"
	metadata, err := json.Marshal(map[string]string{"client_cwd": exactRoot})
	if err != nil {
		t.Fatalf("marshal exact workspace metadata: %v", err)
	}
	resolved, err := codingWorkspaceForJob(model.Job{Metadata: metadata})
	if err != nil {
		t.Fatalf("resolve exact workspace metadata: %v", err)
	}
	if resolved != exactRoot {
		t.Fatalf("resolved workspace=%q, want exact client_cwd %q", resolved, exactRoot)
	}
}
