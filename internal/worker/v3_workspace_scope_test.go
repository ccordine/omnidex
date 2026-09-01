package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestWorkspaceScopeRequiresClientCWDMetadata(t *testing.T) {
	legacy, err := json.Marshal(map[string]string{"host_env_cwd": "/workspace"})
	if err != nil {
		t.Fatalf("marshal legacy metadata: %v", err)
	}

	_, err = codingWorkspaceForJob(model.Job{Metadata: legacy})
	if err == nil || !strings.Contains(err.Error(), "requires client_cwd") {
		t.Fatalf("legacy duplicate workspace metadata was not rejected: %v", err)
	}
}
