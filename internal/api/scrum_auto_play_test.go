package api

import "testing"

func TestNextAutoWorkScrumCardUsesConfiguredColumns(t *testing.T) {
	s := &Server{}
	board := ScrumBoard{Cards: []ScrumCard{
		{ID: "ready-b", Column: "ready", BoardOrder: 2},
		{ID: "ready-a", Column: "ready", BoardOrder: 1},
		{ID: "assigned-a", Column: "assigned", BoardOrder: 1},
	}}
	got := s.nextAutoWorkScrumCard(board, ScrumAutoWorkConfig{Enabled: true, SourceColumns: []string{"ready", "assigned"}})
	if got == nil || got.ID != "ready-a" {
		t.Fatalf("expected ready-a by board_order, got %#v", got)
	}
}

func TestScrumAutoWorkComplete(t *testing.T) {
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "a", Column: "review"},
			{ID: "b", Column: "done"},
		},
	}
	if !scrumAutoWorkComplete(board) {
		t.Fatal("expected complete when all cards are review/done")
	}
	board.Cards = append(board.Cards, ScrumCard{ID: "c", Column: "assigned"})
	if scrumAutoWorkComplete(board) {
		t.Fatal("expected incomplete when assigned cards remain")
	}
}

func TestLoadScrumAutoWorkConfig(t *testing.T) {
	cfg, err := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_work":{"enabled":true,"source_columns":["ready","assigned"]}}`))
	if err != nil {
		t.Fatalf("loadScrumAutoWorkConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if len(cfg.SourceColumns) != 2 || cfg.SourceColumns[0] != "ready" || cfg.SourceColumns[1] != "assigned" {
		t.Fatalf("unexpected source columns: %#v", cfg.SourceColumns)
	}
	if _, err := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_play_through":true}`)); err == nil {
		t.Fatal("legacy scrum_auto_play_through must fail after migration")
	}
	if _, err := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_review":{"enabled":true}}`)); err == nil {
		t.Fatal("removed scrum_auto_review must fail without a compatibility path")
	}
	if _, err := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_work":{"enabled":true,"source_columns":["review"]}}`)); err == nil {
		t.Fatal("invalid auto-work source column must fail")
	}
	if _, err := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_work":`)); err == nil {
		t.Fatal("invalid project settings JSON must fail")
	}
}

func TestNormalizeScrumAutoWorkConfigRejectsDuplicates(t *testing.T) {
	if _, err := validateScrumAutoWorkConfig(ScrumAutoWorkConfig{Enabled: true, SourceColumns: []string{"ready", "ready"}}); err == nil {
		t.Fatal("duplicate auto-work source columns must fail")
	}
}
