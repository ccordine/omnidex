package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestExecutionResumesExactNativeThreadAndSendsOnlyNewOwnerReply(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
		`{"id":1,"result":{}}` + "\n" + `{"id":2,"result":{"model":"fixture-model","thread":{"id":"task-thread"}}}` + "\n"))}
	call := codingagent.Assignment{Role: codingagent.Execute, Root: "/workspace", Reply: "Use blue.",
		Route: codingagent.Route{Model: "fixture-model"}, Session: &codingagent.ExecutionSession{ThreadID: "task-thread", State: "idle"}}
	if _, _, err := connection.startAgentThread(call, ""); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(input.Bytes()))
	resumed := false
	for {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if decoder.Decode(&request) != nil {
			break
		}
		if request.Method == "thread/start" {
			t.Fatal("owner reply opened a fresh thread")
		}
		if request.Method == "thread/resume" {
			resumed = request.Params["threadId"] == "task-thread" && request.Params["ephemeral"] == nil
		}
	}
	prompt, err := renderAgentAssignment(call)
	if !resumed || err != nil || prompt != "Use blue." {
		t.Fatalf("native continuation changed: resumed=%v prompt=%q error=%v", resumed, prompt, err)
	}
	before := input.String()
	call.Context = "Never duplicate the existing task context."
	if receipt, err := connection.injectAgentContext(call, "task-thread"); err != nil || len(receipt) != 0 || input.String() != before {
		t.Fatal("resumed execution reinjected task history")
	}
}

func TestNativeRecoveryReadsCompletedTurnWithoutDispatchingAgain(t *testing.T) {
	for _, status := range []string{"completed", "interrupted", "inProgress"} {
		input := &agentStartInput{}
		response := `{"id":1,"result":{"data":[{"id":"original-turn","status":"` + status + `","items":[{"type":"agentMessage","phase":"final_answer","text":"Done."}]}]}}` + "\n"
		connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(response))}
		call := codingagent.Assignment{Session: &codingagent.ExecutionSession{ThreadID: "thread", TurnID: "original-turn", State: "running"}}
		result, err := connection.recoverExecutionResult(call, codingagent.Result{ThreadID: "thread"}, nil)
		if (err == nil) != (status == "completed") || result.Completed != (status == "completed") {
			t.Fatalf("status=%s result=%+v error=%v", status, result, err)
		}
		if strings.Contains(input.String(), "turn/start") {
			t.Fatal("recovery replayed an effectful prompt")
		}
	}
}

func TestUnacknowledgedDispatchCannotRecoverPreviousTurnAsNewWork(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(`{"id":1,"result":{"data":[{"id":"old-turn","status":"completed","items":[]}]}}` + "\n"))}
	call := codingagent.Assignment{Session: &codingagent.ExecutionSession{ThreadID: "thread", PreviousTurnID: "old-turn", State: "dispatching"}}
	if _, err := connection.recoverExecutionResult(call, codingagent.Result{ThreadID: "thread"}, nil); err == nil {
		t.Fatal("old native completion was accepted as new work")
	}
}
