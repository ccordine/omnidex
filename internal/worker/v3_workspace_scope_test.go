package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestWorkspaceScopeRejectsJobWithoutAuthoritativeBinding(t *testing.T) {
	configuredRoot := t.TempDir()
	service := &Service{workspaceRoot: configuredRoot}

	_, err := service.workspaceScopeForV3Job(model.Job{})
	if err == nil || !strings.Contains(err.Error(), "authoritative job root") {
		t.Fatalf("unbound job error=%v", err)
	}
}

func TestWorkspaceScopeUsesExactJobBinding(t *testing.T) {
	configuredRoot := t.TempDir()
	boundRoot := filepath.Join(configuredRoot, "bound-project")
	if err := os.Mkdir(boundRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	service := &Service{workspaceRoot: configuredRoot}

	scope, err := service.workspaceScopeForV3Job(model.Job{Metadata: []byte(`{"client_cwd":"` + boundRoot + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if scope.Root != boundRoot || scope.Source != "job_metadata" {
		t.Fatalf("scope=%+v", scope)
	}
}
