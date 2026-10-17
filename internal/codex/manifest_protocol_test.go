package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/manifest"
)

type protocolWriter struct{ bytes.Buffer }

func (*protocolWriter) Close() error { return nil }

func TestManifestRoleSettingsReachNativeProtocol(t *testing.T) {
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
			connection := &connection{input: writer, output: bufio.NewScanner(bytes.NewBufferString(responses))}
			call := codingagent.Assignment{Role: fixture.role, Root: t.TempDir(), Route: fixture.route,
				Objective: &codingagent.Objective{Instruction: "Exact current owner request."}, Task: &codingagent.Task{Goal: "Exact frozen task."}}
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
					if request.Params["developerInstructions"] != agentRoleInstructions(fixture.role) || request.Params["developerInstructions"] == "" {
						t.Fatal("role instructions were not dispatched at the developer boundary")
					}
					if fixture.role == codingagent.Execute && (request.Params["sandbox"] != "danger-full-access" || request.Params["approvalPolicy"] != "never") {
						t.Fatal("execution did not receive the granted full permissions")
					}
				case "turn/start":
					seenTurn = true
					if request.Params["effort"] != fixture.route.Effort || request.Params["model"] != fixture.route.Model {
						t.Fatalf("manifest route changed at dispatch: %+v", request.Params)
					}
					if fixture.role == codingagent.Execute && request.Params["sandboxPolicy"].(map[string]any)["type"] != "dangerFullAccess" {
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

func TestExecutionPromptRetainsTaskWithDurableReply(t *testing.T) {
	call := codingagent.Assignment{Role: codingagent.Execute, Task: &codingagent.Task{Goal: "Complete only this task."},
		Reply: "Persisted answer.", Session: &codingagent.ExecutionSession{ThreadID: "old-thread", State: "idle"}}
	prompt, err := renderAgentAssignment(call)
	if err != nil || prompt != call.Task.Goal {
		t.Fatalf("execution relied on native history for its task: %q %v", prompt, err)
	}
}
