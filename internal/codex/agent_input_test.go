package codex

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeRequestsAreSurfacedAndOnlyOwnerResponsesReachCodex(t *testing.T) {
	for _, test := range []struct{ method, params, reply, want string }{
		{"item/commandExecution/requestApproval", `"command":"ssh host uptime","availableDecisions":["accept","decline"]`, "approve", `"decision":"accept"`},
		{"item/fileChange/requestApproval", `"reason":"edit requested file"`, "deny", `"decision":"decline"`},
		{"item/permissions/requestApproval", `"permissions":{"network":{"enabled":true}}`, "approve", `"scope":"turn"`},
		{"item/tool/requestUserInput", `"questions":[{"id":"target","question":"Which target?"}]`, "chosen-host", `"answers":{"target":{"answers":["chosen-host"]}}`},
		{"mcpServer/elicitation/request", `"mode":"form","requestedSchema":{"type":"object"}`, `{"field":"owner-value"}`, `"content":{"field":"owner-value"}`},
		{"mcpServer/elicitation/request", `"mode":"url","url":"https://example.invalid/authorize"`, "done", `"action":"accept"`},
	} {
		t.Run(test.method+test.params, func(t *testing.T) {
			raw := `{"id":"native-request","method":"` + test.method + `","params":{"threadId":"thread","turnId":"turn",` + test.params + `}}`
			input := &agentStartInput{}
			connection := &connection{input: input, lastFrame: json.RawMessage(raw)}
			frame := decodeAgentTestFrame(t, raw)
			asked := 0
			err := connection.handleNativeInput(frame, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, func(event codingagent.NativeEvent) error {
				if event.Input != nil {
					asked++
					var err error
					event.Input.Response, err = event.Input.Request.Reply(test.reply)
					return err
				}
				return nil
			})
			if err != nil || asked != 1 || !strings.Contains(input.String(), test.want) || !strings.Contains(input.String(), `"id":"native-request"`) {
				t.Fatalf("native request lost owner response: count=%d wire=%s error=%v", asked, input.String(), err)
			}
		})
	}
}

func TestEarlyNativeRequestDoesNotBreakPendingTurnStartRPC(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
		`{"id":90,"method":"item/tool/requestUserInput","params":{"threadId":"thread","turnId":"turn","questions":[{"id":"target","question":"Where?"}]}}` + "\n" +
			`{"id":1,"result":{"turn":{"id":"turn"}}}` + "\n"))}
	result := codingagent.Result{ThreadID: "thread"}
	connection.bindAgentRequests(t.Context(), codingagent.Assignment{Role: codingagent.Execute}, &result, func(event codingagent.NativeEvent) error {
		if event.Input != nil {
			event.Input.Response, _ = event.Input.Request.Reply("owner-target")
		}
		return nil
	})
	thread := threadResult{}
	thread.Thread.ID = "thread"
	turn, _, _, err := connection.startAgentTurn(codingagent.Assignment{Role: codingagent.Execute}, thread, "task")
	if err != nil || turn != "turn" || result.TurnID != turn {
		t.Fatalf("early request interrupted turn startup: turn=%s error=%v", turn, err)
	}
}

func TestForeignNativeInputCannotReachOwnerOrProcess(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input}
	frame := decodeAgentTestFrame(t, `{"id":1,"method":"item/fileChange/requestApproval","params":{"threadId":"foreign","turnId":"turn"}}`)
	called := false
	if err := connection.handleNativeInput(frame, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, func(codingagent.NativeEvent) error { called = true; return nil }); err == nil || called || input.Len() != 0 {
		t.Fatal("foreign native request crossed its task boundary")
	}
}

func TestPlanningCannotEscalateItsReadOnlyBoundary(t *testing.T) {
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval"} {
		t.Run(method, func(t *testing.T) {
			input := &agentStartInput{}
			connection := &connection{input: input}
			asked := false
			connection.bindAgentRequests(t.Context(), codingagent.Assignment{Role: codingagent.Plan}, &codingagent.Result{ThreadID: "thread"}, func(codingagent.NativeEvent) error { asked = true; return nil })
			frame := decodeAgentTestFrame(t, `{"id":1,"method":"`+method+`","params":{"threadId":"thread","turnId":"turn"}}`)
			if err := connection.serverRequest(frame); err == nil || asked || input.Len() != 0 {
				t.Fatal("planning acquired execution authority through an approval request")
			}
		})
	}
}
