package api

import "testing"

func TestNormalizeScrumColumnDoesNotInventAliases(t *testing.T) {
	t.Parallel()
	if got := normalizeScrumColumn("in_progress"); got != "in_progress" {
		t.Fatalf("canonical column=%q", got)
	}
	for _, value := range []string{" in_progress", "in_progress ", "in-progress", "in progress", "IN_PROGRESS"} {
		if got := normalizeScrumColumn(value); got != "" {
			t.Errorf("normalizeScrumColumn(%q)=%q, want rejection", value, got)
		}
	}
}

func TestCardsByColumnRejectsPersistedColumnDrift(t *testing.T) {
	t.Parallel()
	for name, board := range map[string]ScrumBoard{
		"invalid card":      {Columns: []string{"backlog"}, Cards: []ScrumCard{{ID: "card-1", Column: "unknown"}}},
		"missing inventory": {Columns: []string{"backlog"}, Cards: []ScrumCard{{ID: "card-1", Column: "assigned"}}},
		"invalid board":     {Columns: []string{" Backlog"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := cardsByColumn(board); err == nil {
				t.Fatal("persisted Scrum column drift was silently mapped")
			}
		})
	}
}
