package api

import "testing"

func TestLoadScrumCreateTicketConfigDefaultsOff(t *testing.T) {
	cfg := loadScrumCreateTicketConfig(nil)
	if cfg.Enabled {
		t.Fatal("expected create ticket default disabled")
	}
	if cfg.Column != "backlog" {
		t.Fatalf("column=%q want backlog", cfg.Column)
	}
}

func TestLoadScrumCreateTicketConfigStored(t *testing.T) {
	cfg := loadScrumCreateTicketConfig([]byte(`{"scrum_create_ticket":{"enabled":true,"column":"assigned"}}`))
	if !cfg.Enabled {
		t.Fatal("expected create ticket enabled")
	}
	if cfg.Column != "assigned" {
		t.Fatalf("column=%q want assigned", cfg.Column)
	}
}

func TestLoadScrumCreateTicketConfigRejectsInvalidColumn(t *testing.T) {
	cfg := loadScrumCreateTicketConfig([]byte(`{"scrum_create_ticket":{"enabled":true,"column":"done"}}`))
	if cfg.Column != "backlog" {
		t.Fatalf("column=%q want backlog", cfg.Column)
	}
}
