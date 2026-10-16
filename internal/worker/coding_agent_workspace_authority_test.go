package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

type localAgentWorkspaceFixture struct {
	ctx       context.Context
	service   *Service
	job       model.Job
	scope     v3WorkspaceScope
	directory string
	client    *workspacetransport.Local
}

func newLocalAgentWorkspaceFixture(t *testing.T, clientID string) localAgentWorkspaceFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("isolated local installation fixture uses XDG_CONFIG_HOME")
	}
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	if err := os.Mkdir(filepath.Join(configRoot, "omnidex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "omnidex", "client-id"), []byte(strings.Repeat("1", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	hub := workspacetransport.NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err == nil {
			_ = hub.Serve(request.Context(), connection)
		}
	}))
	t.Cleanup(server.Close)
	root := filepath.Join(t.TempDir(), "invoking-directory")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	handle, err := projectroot.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := handle.Close(); err != nil {
			t.Error(err)
		}
	})
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := workspacetransport.Connect(ctx, connection, handle, clientID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	directory, err := handle.Identity()
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "codex", Model: "test-model"}
	metadata, err := json.Marshal(map[string]any{
		"client_cwd": root, "client_workspace_identity": client.Identity(),
		"coding_execution": codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route},
	})
	if err != nil {
		t.Fatal(err)
	}
	return localAgentWorkspaceFixture{ctx: ctx,
		service: &Service{workspaceConnections: hub, hostDirectoryAccess: workspace.NewHostDirectoryAccess(t.TempDir())},
		job:     model.Job{Pipeline: model.PipelineChat, Metadata: metadata},
		scope:   v3WorkspaceScope{Root: root, Identity: client.Identity()}, directory: directory, client: client}
}

func TestLocalCLIAgentWorkspaceUsesExactLiveDirectoryOutsideHostBoundary(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	scope, err := fixture.service.workspaceScopeForV3Job(fixture.ctx, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, scope, fixture.directory); err != nil {
		t.Fatalf("live local CLI directory should authorize its native Agents environment: %v", err)
	}
	if err := fixture.service.requireHostWorkspaceRoot(scope.Root); err == nil {
		t.Fatal("local CLI authority widened the fixed host-directory boundary")
	}
	directMetadata, err := json.Marshal(map[string]string{"client_cwd": scope.Root, "client_workspace_identity": fixture.directory})
	if err != nil {
		t.Fatal(err)
	}
	directJob := model.Job{Pipeline: model.PipelineCoding, Metadata: directMetadata}
	if _, err := fixture.service.workspaceScopeForV3Job(fixture.ctx, directJob); err == nil || !strings.Contains(err.Error(), "HOST_DIRECTORY_ACCESS_ROOT") {
		t.Fatal("unbound direct job inherited local CLI directory authority")
	}
}

func TestNativeCLIAgentWorkspaceUsesPhysicalBindingNotInstallationConfig(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("2", 32))
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, fixture.directory); err != nil {
		t.Fatalf("exact physical job binding was rejected by local installation configuration: %v", err)
	}
}

func TestNativeCLIAgentWorkspaceSurvivesDisconnectedClient(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	fixture.service.hostDirectoryAccess = workspace.NewHostDirectoryAccess(fixture.scope.Root)
	if err := fixture.client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, fixture.directory); err != nil {
		t.Fatalf("approved physical job binding incorrectly depends on a live CLI: %v", err)
	}
}

func TestNativeCLIAgentWorkspaceRejectsReplacedDirectory(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	fixture.service.hostDirectoryAccess = workspace.NewHostDirectoryAccess(fixture.scope.Root)
	if err := os.Rename(fixture.scope.Root, fixture.scope.Root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.scope.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, fixture.directory); err == nil {
		t.Fatal("replacement directory inherited native authority from its former pathname")
	}
}

func TestNativeCLIAgentWorkspaceRejectsIdentityDifferentFromJobBinding(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	fixture.service.hostDirectoryAccess = workspace.NewHostDirectoryAccess(fixture.scope.Root)
	otherDirectory, err := projectroot.DirectoryIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture.scope.Identity, err = projectroot.ClientWorkspaceIdentity(strings.Repeat("1", 32), otherDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, otherDirectory); err == nil {
		t.Fatal("native scope bypassed immutable job and physical-directory identity")
	}
}

func TestNativeCLIAgentWorkspaceRejectsConversationWithoutCodingAuthority(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	metadata, err := json.Marshal(map[string]string{"client_cwd": fixture.scope.Root, "client_workspace_identity": fixture.scope.Identity})
	if err != nil {
		t.Fatal(err)
	}
	fixture.job.Metadata = metadata
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, fixture.directory); err == nil {
		t.Fatal("conversation acquired native Agents authority")
	}
	if scope, err := fixture.service.workspaceScopeForV3Job(fixture.ctx, fixture.job); err != nil || scope != fixture.scope {
		t.Fatalf("ordinary client workspace access changed: %#v: %v", scope, err)
	}
}

func TestNativeCLIAgentWorkspaceDoesNotRequireLocalCLIInstallationFiles(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(configRoot, "omnidex", "client-id")); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.requireNativeAgentWorkspace(fixture.ctx, fixture.job, fixture.scope, fixture.directory); err != nil {
		t.Fatalf("job binding incorrectly depends on a local CLI installation file: %v", err)
	}
}
