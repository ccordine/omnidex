package hostbridge

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalAgentHostMutationRoutesAreAbsent(t *testing.T) {
	server := (&Server{}).Handler()
	for _, path := range []string{"/v1/cursor/run", "/v1/codex/run"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("retired host mutation route %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"cursor_handlers.go", "codex_handlers.go", "external_agent_client.go", "external_agent_stream.go"} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired host mutation source remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestHostAgentEnvironmentNamesAreBoundedToHostBridgeTransport(t *testing.T) {
	allowedGoFiles := map[string]struct{}{
		"cmd/cli/host_command.go":               {},
		"cmd/cli/host_serve.go":                 {},
		"cmd/cli/host_service_command.go":       {},
		"internal/api/core_dependencies.go":     {},
		"internal/api/host_handlers.go":         {},
		"internal/api/host_status.go":           {},
		"internal/api/screen_handlers.go":       {},
		"internal/api/terminal_bridge.go":       {},
		"internal/api/terminal_handlers.go":     {},
		"internal/config/config.go":             {},
		"internal/hostbridge/serve.go":          {},
		"internal/hostbridge/systemd.go":        {},
		"internal/omni/host_command.go":         {},
		"internal/omni/host_service_command.go": {},
	}
	inspectProductionRoot := func(root string) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(raw), "HOST_AGENT_URL") &&
				!strings.Contains(string(raw), "HOST_AGENT_TOKEN") &&
				!strings.Contains(string(raw), "HOST_BRIDGE_PUBLIC_WS_URL") {
				return nil
			}
			relative, err := filepath.Rel("../..", path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if _, ok := allowedGoFiles[relative]; !ok {
				t.Errorf("host-bridge transport environment key escaped into %s", relative)
			}
			return nil
		})
	}
	for _, root := range []string{"../../cmd", "../../internal"} {
		if err := inspectProductionRoot(root); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{"../../docker-compose.yml", "../../default.env"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"OMNI_ENABLE_CURSOR_ARCHITECT", "CURSOR_API_KEY",
			"OMNI_ENABLE_CODEX_ARCHITECT", "CODEX_API_KEY", "OMNI_AGENT_STRICT",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s retains external-agent environment authority %q", path, forbidden)
			}
		}
	}
}
