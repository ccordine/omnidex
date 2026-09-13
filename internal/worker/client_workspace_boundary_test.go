package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestClientWorkspaceCannotBindMatchingServerDirectoryForMutation(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "retained.txt")
	if err := os.WriteFile(marker, []byte("accepted bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	directoryID, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity(strings.Repeat("1", 32), directoryID)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]string{"client_cwd": root, "client_workspace_identity": identity})
	if err != nil {
		t.Fatal(err)
	}
	job := model.Job{Pipeline: model.PipelineChat, Metadata: metadata}
	service := &Service{hostDirectoryAccess: workspace.NewHostDirectoryAccess(os.TempDir())}
	if _, err := service.workspaceScopeForV3Job(context.Background(), job); err == nil {
		t.Fatal("client job acquired a same-named server workspace")
	}
	if actual, err := os.ReadFile(marker); err != nil || string(actual) != "accepted bytes" {
		t.Fatalf("client job changed server bytes: %q: %v", actual, err)
	}
	if scope, err := workspaceAuthorityForV3Job(job); err != nil || scope.Root != root || scope.Identity != identity {
		t.Fatalf("pure semantic intake lost exact client binding: %#v: %v", scope, err)
	}
}
