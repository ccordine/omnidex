package api

import "testing"

func TestLoadScrumCreateTicketConfigDefaultsOff(t *testing.T) {
	cfg, err := loadScrumCreateTicketConfig(nil)
	if err != nil {
		t.Fatalf("loadScrumCreateTicketConfig: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("expected create ticket default disabled")
	}
	if cfg.Column != "backlog" {
		t.Fatalf("column=%q want backlog", cfg.Column)
	}
}

func TestLoadScrumCreateTicketConfigStored(t *testing.T) {
	cfg, err := loadScrumCreateTicketConfig([]byte(`{"scrum_create_ticket":{"enabled":true,"column":"assigned"}}`))
	if err != nil {
		t.Fatalf("loadScrumCreateTicketConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatal("expected create ticket enabled")
	}
	if cfg.Column != "assigned" {
		t.Fatalf("column=%q want assigned", cfg.Column)
	}
}

func TestLoadScrumCreateTicketConfigRejectsInvalidColumn(t *testing.T) {
	if _, err := loadScrumCreateTicketConfig([]byte(`{"scrum_create_ticket":{"enabled":true,"column":"done"}}`)); err == nil {
		t.Fatal("invalid create-ticket column must fail")
	}
	if _, err := loadScrumCreateTicketConfig([]byte(`{"scrum_create_ticket":`)); err == nil {
		t.Fatal("invalid create-ticket settings JSON must fail")
	}
}
