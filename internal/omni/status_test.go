package omni

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintStatusShowsExecutionStackAndLastTurn(t *testing.T) {
	var out bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &bytes.Buffer{})
	app.ollama = NewOllamaClient("http://127.0.0.1:11434/api/chat", "test-model")
	app.ollama.ConfigureRuntime("30s", 2048)
	session := &Session{
		WorkspacePath: "/tmp/workspace",
		WorkspaceHash: "workspace_hash",
		Permission:    PermissionFull,
		Turns: []Turn{{
			ID:          "turn_000001",
			UserInput:   "what is the date",
			Response:    "Mon May 18 12:53:03 EDT 2026",
			ReasonCodes: []string{"execution_first_command_loop"},
			CreatedAt:   "2026-05-18T16:53:03Z",
			Events: []Event{
				{Type: "agent_loop_started", Summary: "Execution-first command loop started"},
				{Type: "command_success", Summary: "Command success", Details: map[string]string{"command": "date", "stdout": "Mon May 18 12:53:03 EDT 2026"}},
				{Type: "execution_completed", Summary: "Execution-first command loop completed"},
			},
		}},
	}

	app.printStatus(session)
	got := out.String()
	for _, want := range []string{
		"Execution stack:",
		"Ollama request defaults: keep_alive=30s num_ctx=2048",
		"normal prompts: execution-first command loop",
		"/manage, /job: manager-worker orchestration",
		"/micro, /queue: project-profiled manager-manager micro job queue",
		"document search: chunked manager-worker needle finding",
		"web docs: fetch, normalize, chunk, search, and cite documentation",
		"memory: Postgres-backed tags + query retrieval",
		"relay service: exact JSON handoff with checksum validation",
		"structured command loop: max_steps=40 task_budget=6h0m0s ollama_request_timeout=10m0s",
		"command loop: max_steps=",
		"manager: max_workers=",
		"document chunks: chunk_chars=",
		"Tools: implemented=",
		"Last turn: turn_000001",
		"commands_success=1",
		"command=date",
		"stdout=Mon May 18 12:53:03 EDT 2026",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestCountEventTypes(t *testing.T) {
	counts := countEventTypes([]Event{{Type: "a"}, {Type: "b"}, {Type: "a"}})
	if counts["a"] != 2 || counts["b"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}
