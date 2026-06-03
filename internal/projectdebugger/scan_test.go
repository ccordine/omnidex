package projectdebugger

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMetadata(t *testing.T) {
	raw, err := JobMetadata(42, "cursor", "qwen3:4b", "planner:7b")
	if err != nil {
		t.Fatal(err)
	}
	if !IsJobMetadata(raw) {
		t.Fatal("expected debugger metadata")
	}
	meta, err := ParseMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProjectID != 42 || meta.AgentSystem != "cursor" || meta.AnalyzerModel != "qwen3:4b" || meta.TicketModel != "planner:7b" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestCardPlanningPrompt(t *testing.T) {
	got := CardPlanningPrompt("Fix auth", "Login flow lacks tests")
	if !strings.Contains(got, "Fix auth") || !strings.Contains(got, "Login flow") {
		t.Fatalf("prompt=%q", got)
	}
}

func TestParseScanResponse(t *testing.T) {
	raw := `Here are findings:
{"summary":"Found test gaps","bug_tickets":[{"title":"Missing auth tests","description":"No coverage for login flow","severity":"high","column":"backlog","checklist":["Add integration test"],"ref_files":["internal/auth/login.go"],"tags":["security"]}],"suggestions":["Run tests in CI"]}`
	parsed := ParseScanResponse(raw)
	if parsed.Summary != "Found test gaps" {
		t.Fatalf("summary=%q", parsed.Summary)
	}
	if len(parsed.BugTickets) != 1 {
		t.Fatalf("tickets=%d", len(parsed.BugTickets))
	}
	ticket := parsed.BugTickets[0]
	if ticket.Severity != "high" {
		t.Fatalf("severity=%q", ticket.Severity)
	}
	if ticket.Tags[0] != "security" || ticket.Tags[len(ticket.Tags)-1] != "analysis" {
		t.Fatalf("tags=%v", ticket.Tags)
	}
}

func TestChecklistJSON(t *testing.T) {
	raw := ChecklistJSON([]string{"Verify fix", ""})
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
}
