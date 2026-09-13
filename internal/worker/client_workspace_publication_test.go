package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

func clientPublicationFixture(t *testing.T) *directCodingSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	hub := workspacetransport.NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = hub.Serve(r.Context(), conn)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := projectroot.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := directory.Close(); err != nil {
			t.Error(err)
		}
	})
	local, err := workspacetransport.Connect(ctx, conn, directory, strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	access, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := access.Release(); err != nil {
			t.Error(err)
		}
	})
	metadata, err := json.Marshal(map[string]string{"client_cwd": root, "client_workspace_identity": local.Identity()})
	if err != nil {
		t.Fatal(err)
	}
	return &directCodingSession{
		root: root, program: &directCodingProgram{Project: directCodingProjectSelection{Stack: directCodingProjectStack{ID: genericTypeScriptBrowserAdapter}}},
		runtime: &nativeRuntimeV3{
			ctx: ctx, claim: &model.ClaimedStep{Job: model.Job{Pipeline: model.PipelineChat, Metadata: metadata}},
			svc:            &Service{workspaceConnections: hub, hostDirectoryAccess: workspace.NewHostDirectoryAccess(filepath.Join(t.TempDir(), "unavailable"))},
			workspaceFence: access, workspaceFenceRoot: root,
		},
	}
}

func TestClientPublicationRetainsVerifiedFilesAcrossLaterFailure(t *testing.T) {
	session := clientPublicationFixture(t)
	assembly := publicationWorkspaceAssembly()
	prepared, err := session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(session.root, "b.txt")
	if err := os.WriteFile(conflict, []byte("user bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err == nil {
		t.Fatal("published over an unowned client file")
	}
	if len(session.publishedAssembly.Files) != 1 || session.publishedAssembly.Files[0].Path != "a.json" || len(session.mutationJournal) != 1 {
		t.Fatalf("lost accepted client publication: %#v", session.publishedAssembly)
	}
	if err := os.Remove(conflict); err != nil {
		t.Fatal(err)
	}
	prepared, err = session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil {
		t.Fatal(err)
	}
	if len(prepared.result.Changes) != 1 || prepared.result.Changes[0].Path != "b.txt" {
		t.Fatalf("revisited accepted files: %#v", prepared.result)
	}
	prepared, err = session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil || len(prepared.result.Changes) != 0 {
		t.Fatalf("zero delta: %#v %v", prepared.result, err)
	}
}

func TestClientPublicationRejectsChangesToItsAcceptedBase(t *testing.T) {
	session := clientPublicationFixture(t)
	assembly := publicationWorkspaceAssembly()
	prepared, err := session.prepareWorkspaceReconciliation(directCodingAssembly{Files: assembly.Files[:1]})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil {
		t.Fatal(err)
	}
	prepared, err = session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session.root, "a.json"), []byte("edited by user"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err == nil {
		t.Fatal("client edit did not stop dependent publication")
	}
	if _, err := os.Stat(filepath.Join(session.root, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("dependent file was written: %v", err)
	}
}

func TestCodingWorkspaceObservationsCannotOpenServerPaths(t *testing.T) {
	for _, file := range []string{
		"v3_coding_assembly_observation.go", "v3_target_tree_existing_browser.go", "v3_coding_browser_greenfield_authority.go",
		"v3_coding_driver_files.go", "v3_coding_driver_workspace_execute.go", "v3_coding_typescript_host_verification.go",
		"v3_coding_go_host_verification.go", "v3_compiled_language_workspace.go",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"os.ReadFile(", "os.Lstat(", "os.Stat(", "os.ReadDir(", "os.Open(", "os.OpenRoot(", "validateDirectCodingAssemblyAtRoot("} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s bypasses acquired workspace access through %s", file, forbidden)
			}
		}
	}
}
