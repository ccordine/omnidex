package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeProgressUsesFreshContextWithRetainedReport(t *testing.T) {
	writer := &protocolWriter{}
	responses := "{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"model\":\"fixture\",\"thread\":{\"id\":\"retained-thread\"}}}\n{\"id\":3,\"result\":{}}\n{\"id\":4,\"result\":{\"turn\":{\"id\":\"next-turn\"}}}\n"
	conn := &connection{input: writer, output: bufio.NewReader(bytes.NewBufferString(responses))}
	call := codingagent.Assignment{Role: codingagent.Execute, Root: t.TempDir(), Route: codingagent.Route{Model: "fixture", Effort: "low"},
		Task:            &codingagent.Task{ID: "retained-task", Goal: "Finish and verify both outputs."},
		ExecutionReport: &codingagent.ExecutionReport{CallID: 21, Round: 1, Response: "First output is complete. The second output and verification remain unfinished."}}
	if err := call.ValidateTaskContext(); err != nil {
		t.Fatal(err)
	}
	thread, _, err := conn.startAgentThread(call, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.injectAgentContext(call, thread.Thread.ID); err != nil {
		t.Fatal(err)
	}
	prompt, err := renderAgentAssignment(call)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _, err := conn.startAgentTurn(call, thread, prompt)
	if err != nil || turn != "next-turn" {
		t.Fatalf("continuation dispatch: %s %v", turn, err)
	}
	methods := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(writer.String()), "\n") {
		var frame struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatal(err)
		}
		methods[frame.Method]++
		if frame.Method == "thread/inject_items" {
			var params struct {
				ThreadID string                `json:"threadId"`
				Items    []agentHistoryMessage `json:"items"`
			}
			if err := json.Unmarshal(frame.Params, &params); err != nil || params.ThreadID != "retained-thread" || len(params.Items) != 1 ||
				!strings.Contains(params.Items[0].Content[0].Text, call.ExecutionReport.Response) {
				t.Fatalf("persisted progress missing from continued context: %s %v", frame.Params, err)
			}
		}
		if frame.Method == "turn/start" {
			var params struct {
				ThreadID string `json:"threadId"`
				Model    string `json:"model"`
			}
			if err := json.Unmarshal(frame.Params, &params); err != nil || params.ThreadID != "retained-thread" || params.Model != "fixture" {
				t.Fatalf("continuation changed thread or model: %s", frame.Params)
			}
		}
	}
	if methods["thread/resume"] != 0 || methods["turn/start"] != 1 || methods["thread/start"] != 1 || methods["thread/turns/list"] != 0 {
		t.Fatalf("continuation did not use a fresh context with durable progress: %v", methods)
	}
	// An owner interruption remains distinct from ordinary progress. Its actual
	// continuation instruction takes precedence and is never manufactured.
	call.Reply = "Resume, preserving the completed first output."
	prompt, err = renderAgentContext(call)
	if err != nil || !strings.Contains(prompt, call.Reply) {
		t.Fatal("owner continuation was hidden by the progress report", err)
	}
}
