package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoadScrumAutoWorkConfigUsesExplicitDefaultOnlyWhenAbsent(t *testing.T) {
	t.Parallel()

	config, err := loadScrumAutoWorkConfig(json.RawMessage(`{"model_config":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || len(config.SourceColumns) != 1 || config.SourceColumns[0] != "assigned" {
		t.Fatalf("config=%+v", config)
	}
}

func TestLoadScrumAutoWorkConfigPreservesOneExactStoredConfig(t *testing.T) {
	t.Parallel()

	config, err := loadScrumAutoWorkConfig(json.RawMessage(
		`{"scrum_auto_work":{"enabled":true,"source_columns":["ready","assigned"]}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || len(config.SourceColumns) != 2 ||
		config.SourceColumns[0] != "ready" || config.SourceColumns[1] != "assigned" {
		t.Fatalf("config=%+v", config)
	}
}

func TestLoadScrumAutoWorkConfigRejectsMalformedPresentAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, settings, want string
	}{
		{name: "null", settings: `{"scrum_auto_work":null}`, want: "must be an object"},
		{name: "missing enabled", settings: `{"scrum_auto_work":{"source_columns":["assigned"]}}`, want: "enabled is required"},
		{name: "missing columns", settings: `{"scrum_auto_work":{"enabled":true}}`, want: "source_columns is required"},
		{name: "unknown field", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":["assigned"],"agent":"x"}}`, want: "unknown field"},
		{name: "duplicate field", settings: `{"scrum_auto_work":{"enabled":true,"enabled":false,"source_columns":["assigned"]}}`, want: "duplicate key"},
		{name: "noncanonical column", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":[" assigned"]}}`, want: "not registered"},
		{name: "unknown column", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":["triage"]}}`, want: "not registered"},
		{name: "duplicate column", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":["assigned","assigned"]}}`, want: "duplicate"},
		{name: "empty columns", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":[]}}`, want: "at least one"},
		{name: "unsupported source", settings: `{"scrum_auto_work":{"enabled":true,"source_columns":["done"]}}`, want: "not registered"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadScrumAutoWorkConfig(json.RawMessage(test.settings))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want containing %q", err, test.want)
			}
		})
	}
}

func TestScrumAutoWorkHasNoSilentColumnNormalizer(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("scrum_auto_play.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "normalizeScrumAutoWorkColumns") {
		t.Fatal("Scrum auto-work retains a silent column normalizer")
	}
}
