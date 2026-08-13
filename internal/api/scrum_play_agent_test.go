package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestValidateScrumPlayAgentCodexPreflightFailsLoudly(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "codex-key")
	t.Setenv("HOST_AGENT_URL", "")
	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "true")
	missing := filepath.Join(t.TempDir(), "missing-tool")
	t.Setenv("OMNI_CODEX_NODE_BIN", missing)
	t.Setenv("OMNI_CODEX_NPM_BIN", missing)
	t.Setenv("OMNI_CODEX_BIN", missing)

	project := model.Project{
		Settings: json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`),
	}
	err := (&Server{}).validateScrumPlayAgent(context.Background(), project, ScrumCard{}, nil)
	if err == nil {
		t.Fatal("expected codex preflight failure")
	}
	if !strings.Contains(err.Error(), "codex host preflight failed") {
		t.Fatalf("expected codex preflight error, got %v", err)
	}
}
