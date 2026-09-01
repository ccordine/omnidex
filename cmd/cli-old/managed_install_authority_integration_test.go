package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestInstalledOmniChatUsesOneExplicitManagedCoreAuthorityAndCallerWorkspace(t *testing.T) {
	installRoot := filepath.Join(t.TempDir(), "managed-install")
	buildInstalledCLIBinaries(t, installRoot)
	workspace := filepath.Join(t.TempDir(), "caller-project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	const session = "managed-authority-integration"
	expected, err := chatChannelForSession(session, workspace)
	if err != nil {
		t.Fatal(err)
	}
	expected.ProjectID = 41
	managedServer, managedCalls := managedChatAuthorityServer(t, expected)
	explicitServer, explicitCalls := managedChatAuthorityServer(t, expected)

	writeManagedCLIEnvironment(t, installRoot, "CORE_URL="+managedServer.URL+"\n")
	if output, err := runInstalledOmniChat(installRoot, workspace, session, nil); err != nil {
		t.Fatalf("managed omni chat failed: %v\n%s", err, output)
	}
	if got := managedCalls.Load(); got != 1 {
		t.Fatalf("managed CORE_URL calls = %d, want 1", got)
	}

	if output, err := runInstalledOmniChat(installRoot, workspace, session, []string{"CORE_URL=" + explicitServer.URL}); err != nil {
		t.Fatalf("explicit omni chat failed: %v\n%s", err, output)
	}
	if got := explicitCalls.Load(); got != 1 {
		t.Fatalf("explicit CORE_URL calls = %d, want 1", got)
	}
	if got := managedCalls.Load(); got != 1 {
		t.Fatalf("explicit process authority leaked to managed endpoint; calls = %d", got)
	}

	output, err := runInstalledOmniChat(installRoot, workspace, session, []string{"CORE_URL="})
	if err == nil || !strings.Contains(output, "CORE_URL is required") {
		t.Fatalf("blank explicit CORE_URL error = %v, output = %q", err, output)
	}
	if got := managedCalls.Load(); got != 1 {
		t.Fatalf("blank explicit CORE_URL fell through to managed endpoint; calls = %d", got)
	}

	writeManagedCLIEnvironment(t, installRoot, "CORE_URL=not-an-absolute-url\n")
	output, err = runInstalledOmniChat(installRoot, workspace, session, nil)
	if err == nil || !strings.Contains(output, "absolute http or https URL") {
		t.Fatalf("malformed managed CORE_URL error = %v, output = %q", err, output)
	}

	if err := os.Remove(filepath.Join(installRoot, ".env")); err != nil {
		t.Fatal(err)
	}
	output, err = runInstalledOmniChat(installRoot, workspace, session, nil)
	if err == nil || !strings.Contains(output, "managed CORE_URL is unavailable") {
		t.Fatalf("missing managed CORE_URL error = %v, output = %q", err, output)
	}
}

func buildInstalledCLIBinaries(t *testing.T, installRoot string) {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	bin := filepath.Join(installRoot, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, packagePath := range map[string]string{
		"agent-cli": "./cmd/cli",
		"omni":      "./cmd/omni",
	} {
		command := exec.Command("go", "build", "-o", filepath.Join(bin, name), packagePath)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build installed %s: %v: %s", name, err, output)
		}
	}
}

func managedChatAuthorityServer(t *testing.T, expected model.Channel) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/v1/channels/"+string(expected.ID) {
			http.Error(response, "unexpected managed chat request", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{"channel": expected}); err != nil {
			t.Errorf("encode channel response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func writeManagedCLIEnvironment(t *testing.T, installRoot, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(installRoot, ".env"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runInstalledOmniChat(
	installRoot string,
	workspace string,
	session string,
	overrides []string,
) (string, error) {
	command := exec.Command(
		filepath.Join(installRoot, "bin", "omni"),
		"chat", "--session", session, "--progress=false",
	)
	command.Dir = workspace
	command.Stdin = strings.NewReader("")
	command.Env = managedCLIIntegrationEnvironment(os.Environ(), overrides)
	output, err := command.CombinedOutput()
	return string(output), err
}

func managedCLIIntegrationEnvironment(base, overrides []string) []string {
	blocked := map[string]struct{}{
		"CORE_URL":           {},
		"OMNIDEX_DIR":        {},
		"OMNI_AGENT_CLI_BIN": {},
		"OMNI_INVOKE_CWD":    {},
	}
	for _, override := range overrides {
		if key, _, found := strings.Cut(override, "="); found {
			blocked[key] = struct{}{}
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if _, remove := blocked[key]; found && remove {
			continue
		}
		result = append(result, entry)
	}
	return append(result, overrides...)
}
