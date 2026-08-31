package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestWorkspaceScopeRequiresClientCWDMetadata(t *testing.T) {
	temporary := t.TempDir()
	runtimeRoot := filepath.Join(temporary, "runtime")
	hostRoot := filepath.Join(temporary, "host")
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	service := &Service{workspaceRoot: runtimeRoot, workspaceHostRoot: hostRoot}
	legacy, err := json.Marshal(map[string]string{"host_env_cwd": hostRoot})
	if err != nil {
		t.Fatalf("marshal legacy metadata: %v", err)
	}

	_, err = service.workspaceScopeForV3Job(model.Job{Metadata: legacy})
	if err == nil || !strings.Contains(err.Error(), "authoritative job root") {
		t.Fatalf("legacy duplicate workspace metadata was not rejected: %v", err)
	}
}
