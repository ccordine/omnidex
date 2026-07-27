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
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"source":"project_debugger","project_id":"42","agent_system":"cursor","model":"qwen","ticket_model":"planner"}`),
		json.RawMessage(`{"source":"project_debugger","project_id":42,"agent_system":"cursor","model":"qwen","ticket_model":"planner","legacy":true}`),
		json.RawMessage(`{"source":"project_debugger","project_id":42,"agent_system":"","model":"qwen","ticket_model":"planner"}`),
	} {
		if _, err := ParseMetadata(raw); err == nil {
			t.Fatalf("metadata %s must fail", raw)
		}
	}
	if _, err := JobMetadata(42, "", "qwen", "planner"); err == nil {
		t.Fatal("empty agent system must fail")
	}
}

func TestCardPlanningPrompt(t *testing.T) {
	got, err := CardPlanningPrompt("Fix auth", "Login flow lacks tests")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Fix auth") || !strings.Contains(got, "Login flow") {
		t.Fatalf("prompt=%q", got)
	}
	if _, err := CardPlanningPrompt("", "missing title"); err == nil {
		t.Fatal("empty title must fail")
	}
}

func TestParseScanResponseStrict(t *testing.T) {
	raw := `{"summary":"Found test gaps","bug_tickets":[{"title":"Missing auth tests","description":"No coverage for login flow","severity":"high","column":"backlog","checklist":["Add integration test"],"ref_files":["internal/auth/login.go"],"tags":["security"]}],"suggestions":["Run tests in CI"]}`
	parsed, err := ParseScanResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
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
	for _, invalid := range []string{
		`plain text`,
		`{"summary":"ok","unknown":true}`,
		`{"summary":"","bug_tickets":[]}`,
		`{"summary":"ok","bug_tickets":[{"title":"x","description":"body","severity":"maybe","column":"backlog","checklist":["test"]}]}`,
		`{"summary":"ok","bug_tickets":[{"title":"x","description":"body","severity":"high","column":"ready","checklist":["test"]}]}`,
		`{"summary":"ok","bug_tickets":[{"title":"x","description":"body","severity":"high","column":"backlog","checklist":[]}]}`,
	} {
		if _, err := ParseScanResponse(invalid); err == nil {
			t.Fatalf("response %s must fail", invalid)
		}
	}
}

func TestChecklistJSON(t *testing.T) {
	raw, err := ChecklistJSON([]string{"Verify fix"})
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	if _, err := ChecklistJSON([]string{""}); err == nil {
		t.Fatal("empty checklist item must fail")
	}
}
