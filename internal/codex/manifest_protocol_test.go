package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/manifest"
)

type protocolWriter struct{ bytes.Buffer }

func (*protocolWriter) Close() error { return nil }

func TestManifestRoleSettingsReachNativeProtocol(t *testing.T) {
	instructions := agentRoleInstructions(codingagent.Review)
	for _, required := range []string{"completely fulfilled", "actual software", "existing tools and dependencies", "Do not modify the implementation", "original goal"} {
		if !strings.Contains(instructions, required) {
			t.Fatalf("review instructions lack %q", required)
		}
	}
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		if !strings.Contains(agentRoleInstructions(role), codingagent.FrozenItemPhaseInstructions) {
			t.Fatal("native phase was left to historical task context")
		}
	}
	defaults := manifest.Defaults().Coding()
	for _, fixture := range []struct {
		role  codingagent.Role
		route codingagent.Route
	}{
		{codingagent.Plan, defaults.Planner},
		{codingagent.Execute, defaults.Executor},
		{codingagent.Review, defaults.Reviewer},
	} {
		t.Run(string(fixture.role), func(t *testing.T) {
			writer := &protocolWriter{}
			responses := fmt.Sprintf("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"model\":%q,\"thread\":{\"id\":\"exact-thread\"}}}\n{\"id\":3,\"result\":{\"turn\":{\"id\":\"exact-turn\"}}}\n", fixture.route.Model)
			connection := &connection{input: writer, output: bufio.NewReader(bytes.NewBufferString(responses))}
			call := codingagent.Assignment{Role: fixture.role, Root: t.TempDir(), Route: fixture.route,
				Task: &codingagent.Task{Goal: "Exact frozen task.", Payload: json.RawMessage(`{"goal":"Exact frozen task."}`)}}
			if fixture.role == codingagent.Plan {
				call.Task = nil
				call.Objective = &codingagent.Objective{Instruction: "Exact current owner request."}
			}
			thread, _, err := connection.startAgentThread(call, "")
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := renderAgentAssignment(call)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := connection.startAgentTurn(call, thread, prompt); err != nil {
				t.Fatal(err)
			}
			messages := bufio.NewScanner(bytes.NewReader(writer.Bytes()))
			seenThread, seenTurn := false, false
			for messages.Scan() {
				var request struct {
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.Unmarshal(messages.Bytes(), &request); err != nil {
					t.Fatal(err)
				}
				switch request.Method {
				case "thread/start":
					seenThread = true
					if fixture.role == codingagent.Review && (request.Params["ephemeral"] != false || request.Params["threadId"] != nil) {
						t.Fatal("review must start a fresh independent thread")
					}
					if fixture.role == codingagent.Plan && (request.Params["sandbox"] != "read-only" || request.Params["approvalPolicy"] != "never") {
						t.Fatal("planning permissions changed")
					}
					if request.Params["developerInstructions"] != agentRoleInstructions(fixture.role) {
						t.Fatal("role instructions were not dispatched at the developer boundary")
					}
					if (fixture.role == codingagent.Execute || fixture.role == codingagent.Review) && (request.Params["sandbox"] != "danger-full-access" || request.Params["approvalPolicy"] != "never") {
						t.Fatal("task role did not receive the granted full permissions")
					}
				case "turn/start":
					seenTurn = true
					if request.Params["effort"] != fixture.route.Effort || request.Params["model"] != fixture.route.Model {
						t.Fatalf("manifest route changed at dispatch: %+v", request.Params)
					}
					if (fixture.role == codingagent.Execute || fixture.role == codingagent.Review) && (request.Params["sandboxPolicy"].(map[string]any)["type"] != "dangerFullAccess" || request.Params["approvalPolicy"] != "never") {
						t.Fatal("turn permissions disagree with thread permissions")
					}
				}
			}
			if !seenThread || !seenTurn {
				t.Fatal("native protocol was not dispatched")
			}
		})
	}
}

func TestExecutionPromptUsesEntireStoredItemUnchanged(t *testing.T) {
	payload := json.RawMessage(`{ "id":"task", "goal":"Complete only this task.", "rank":5, "acceptance":[{"text":"Actually works"}] }`)
	call := codingagent.Assignment{Role: codingagent.Execute, Task: &codingagent.Task{Goal: "Complete only this task.", Payload: payload},
		Reply: "Persisted answer.", Session: &codingagent.ExecutionSession{ThreadID: "old-thread", State: "idle"}}
	prompt, err := renderAgentAssignment(call)
	if err != nil || prompt != string(payload) {
		t.Fatalf("execution relied on native history for its task: %q %v", prompt, err)
	}
}
