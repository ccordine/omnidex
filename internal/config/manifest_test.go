package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/manifest"
)

func TestManifestOverridesInstallationRoutingWithoutChangingDatabase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DATABASE_URL", "postgresql:///retained")
	t.Setenv("DATABASE_SCHEMA", "retained_schema")
	t.Setenv("CORE_URL", "http://old-listener")
	t.Setenv("CODING_PLANNER_MODEL", "old-planner")
	t.Setenv("CODING_PLANNER_EFFORT", "low")
	t.Setenv("CODING_LEDGER_PARSER_PROVIDER", "ollama")
	t.Setenv("LLM_PROVIDER", "ollama")
	path, _ := manifest.HomePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(manifest.Defaults())
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ManifestConfigured || cfg.CoreURL != manifest.DefaultCoreURL || cfg.CodingAgents != manifest.Defaults().Coding() || cfg.LLMProvider != "codex" {
		t.Fatalf("manifest defaults were shadowed: %+v", cfg.CodingAgents)
	}
	if cfg.DatabaseURL != "postgresql:///retained" || cfg.DatabaseSchema != "retained_schema" {
		t.Fatal("manifest changed retained database authority")
	}
	if cfg.ModelAuthority.Config().Get("coding_surface_model") != "gpt-6.1-sol" {
		t.Fatal("semantic model did not use manifest default")
	}
}
