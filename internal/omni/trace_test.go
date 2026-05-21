package omni

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRunTraceSummarizesSessionEvents(t *testing.T) {
	session := &Session{
		WorkspacePath: "/tmp/project",
		WorkspaceHash: "trace-workspace",
		Turns: []Turn{{
			ID:        "turn_000001",
			CreatedAt: "2026-05-20T12:00:00Z",
			Events: []Event{
				{Type: "prompt_interpreter_completed", CreatedAt: "2026-05-20T12:00:00Z"},
				{Type: "structured_llm_request_started", CreatedAt: "2026-05-20T12:00:01Z"},
				{Type: "prep_workspace_scan_completed", CreatedAt: "2026-05-20T12:00:01Z"},
				{Type: "structured_command_finished", CreatedAt: "2026-05-20T12:00:02Z"},
				{Type: "structured_command_rejected", CreatedAt: "2026-05-20T12:00:03Z"},
				{Type: "structured_done_rejected", CreatedAt: "2026-05-20T12:00:04Z"},
				{Type: "completion_check_completed", CreatedAt: "2026-05-20T12:00:05Z"},
			},
		}},
	}

	trace := BuildRunTrace(session)
	if trace.ModelCalls != 3 {
		t.Fatalf("model calls = %d, want 3", trace.ModelCalls)
	}
	if trace.Commands != 1 || trace.RejectedCommands != 1 || trace.DoneRejections != 1 {
		t.Fatalf("unexpected trace counts: %#v", trace)
	}
	if trace.PrepEvents != 1 || trace.Turns[0].PrepEvents != 1 {
		t.Fatalf("prep events not counted: %#v", trace)
	}
	if trace.EstimatedDuration != "5s" {
		t.Fatalf("duration = %q", trace.EstimatedDuration)
	}
}

func TestOmniRunTraceLatestCommandPrintsJSON(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	sessionRoot := filepath.Join(tmp, "sessions")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(sessionRoot)
	session, _, err := store.LoadOrCreate(workspace)
	if err != nil {
		t.Fatal(err)
	}
	session.Turns = append(session.Turns, Turn{
		ID: "turn_000001",
		Events: []Event{{
			Type:      "structured_llm_request_started",
			CreatedAt: "2026-05-20T12:00:00Z",
		}},
	})
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err = app.Run([]string{"run:trace", "latest", "--workspace", workspace, "--session-root", sessionRoot, "--json"})
	if err != nil {
		t.Fatalf("run:trace failed: %v\nstderr=%s", err, errOut.String())
	}
	var trace RunTrace
	if err := json.Unmarshal(out.Bytes(), &trace); err != nil {
		t.Fatal(err)
	}
	if trace.ModelCalls != 1 || trace.TurnCount != 1 {
		t.Fatalf("unexpected trace: %#v", trace)
	}
}
