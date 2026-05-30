package api

import (
	"strings"
	"testing"
)

func TestScrumAgentConfigErrorNote(t *testing.T) {
	output := "strict external agent required: Cursor SDK agent is not enabled (set OMNI_ENABLE_CURSOR_ARCHITECT=true and CURSOR_API_KEY)"
	note := scrumAgentConfigErrorNote(output)
	if !strings.Contains(note, "Cursor SDK not configured") {
		t.Fatalf("note=%q", note)
	}
	if scrumAgentConfigErrorNote("all good") != "" {
		t.Fatal("expected empty note for normal output")
	}
}
