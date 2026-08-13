package scrum

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnprovenCardTicketIsDisplayOnly(t *testing.T) {
	t.Parallel()
	lines := ContextLinesFromMetadata(json.RawMessage(`{
		"scrum_card_description":"Authoritative description",
		"scrum_card_ticket":"legacy unproven ticket",
		"scrum_card_tags":["legacy-ai-tag"]
	}`))
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Authoritative description") {
		t.Fatalf("description missing from context: %q", joined)
	}
	if strings.Contains(joined, "legacy unproven ticket") || strings.Contains(joined, "Card ticket") {
		t.Fatalf("display-only ticket entered execution context: %q", joined)
	}
	if strings.Contains(joined, "legacy-ai-tag") || strings.Contains(joined, "Tags:") {
		t.Fatalf("unproven tag entered execution context: %q", joined)
	}
}

func TestCardContextHasNoTicketAuthorityField(t *testing.T) {
	t.Parallel()
	lines := AppendCardContextLines(nil, CardContext{Description: "Exact description"})
	if joined := strings.Join(lines, "\n"); joined != "Description:\nExact description" {
		t.Fatalf("context=%q", joined)
	}
}
