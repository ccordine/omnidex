package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildEvidenceLedgerFromSessionTurns(t *testing.T) {
	session := &Session{
		WorkspacePath: "/tmp/project",
		WorkspaceHash: "abc123",
		Turns: []Turn{{
			ID:        "turn_000001",
			UserInput: "build app",
			Response:  "partial progress",
			CreatedAt: "2026-05-20T00:00:00Z",
			Events: []Event{
				{Type: "structured_llm_request_started", Details: map[string]string{"pending_objectives": "install_deps,build_bundle"}},
				{Type: "prep_workspace_scan_completed", Summary: "Codebase route prepared", Details: map[string]string{"likely_files": "package.json"}},
				{Type: "structured_command_finished", Details: map[string]string{"step": "1", "command": "npm install", "exit_code": "0", "stdout": "ok"}},
				{Type: "structured_command_rejected", Details: map[string]string{"step": "2", "command": "npm install", "reason": "repeat"}},
				{Type: "structured_loop_exhausted"},
			},
		}},
	}

	ledger := BuildEvidenceLedger(session)
	if ledger.Summary.TurnCount != 1 || ledger.Summary.CommandCount != 1 || ledger.Summary.RejectedCommandCount != 1 || ledger.Summary.FailedTurnCount != 1 {
		t.Fatalf("unexpected summary: %#v", ledger.Summary)
	}
	if got := ledger.Turns[0].Pending; len(got) != 2 || got[0] != "install_deps" || got[1] != "build_bundle" {
		t.Fatalf("pending = %#v", got)
	}
	if ledger.Turns[0].RejectedCommands[0].Command != "npm install" {
		t.Fatalf("rejected command not exported: %#v", ledger.Turns[0].RejectedCommands[0])
	}
	if ledger.Summary.PrepEventCount != 1 || len(ledger.Turns[0].Prep) != 1 {
		t.Fatalf("prep evidence not exported: summary=%#v prep=%#v", ledger.Summary, ledger.Turns[0].Prep)
	}
}

func TestExportEvidenceLedgerWritesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	session := &Session{WorkspacePath: "/tmp/project", WorkspaceHash: "abc123"}
	if err := ExportEvidenceLedger(session, path); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ledger EvidenceLedger
	if err := json.Unmarshal(blob, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.WorkspaceID != "abc123" {
		t.Fatalf("workspace id = %q", ledger.WorkspaceID)
	}
}
