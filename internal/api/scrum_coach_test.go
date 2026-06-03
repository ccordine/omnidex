package api

import (
	"encoding/json"
	"testing"
)

func TestDefaultScrumCoachConfigAutoScanOff(t *testing.T) {
	cfg := defaultScrumCoachConfig()
	if cfg.AutoScan {
		t.Fatal("expected auto_scan default false")
	}
	if !cfg.Enabled {
		t.Fatal("expected coach enabled by default")
	}
}

func TestParseScrumCoachConfigEmptyDefaultsAutoScanOff(t *testing.T) {
	cfg := parseScrumCoachConfig(nil)
	if cfg.AutoScan {
		t.Fatal("expected auto_scan false for empty config")
	}
}

func TestParseScrumCoachConfigExplicitAutoScan(t *testing.T) {
	raw := json.RawMessage(`{"auto_scan":true}`)
	cfg := parseScrumCoachConfig(raw)
	if !cfg.AutoScan {
		t.Fatal("expected auto_scan true when set")
	}
}
