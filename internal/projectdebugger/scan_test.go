package projectdebugger

import (
	"encoding/json"
	"testing"
)

func TestParseMetadata(t *testing.T) {
	raw, err := JobMetadata(42, "cursor", "qwen3:4b")
	if err != nil {
		t.Fatal(err)
	}
	if !IsJobMetadata(raw) {
		t.Fatal("expected debugger metadata")
	}
	projectID, agent, modelName, err := ParseMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if projectID != 42 || agent != "cursor" || modelName != "qwen3:4b" {
		t.Fatalf("unexpected metadata: %d %q %q", projectID, agent, modelName)
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
