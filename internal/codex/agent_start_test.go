package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

type agentStartInput struct{ bytes.Buffer }

func (*agentStartInput) Close() error { return nil }

func TestNativeAgentRolesUsePlanAndReviewProtocol(t *testing.T) {
	effort := "high"
	thread := threadResult{Model: "fixture-model", ReasoningEffort: &effort}
	thread.Thread.ID = "thread-1"
	for _, test := range []struct {
		name       string
		role       codingagent.Role
		response   string
		method     string
		wantNotice bool
	}{
		{
			name: "plan", role: codingagent.Plan, method: "turn/start", wantNotice: true,
			response: `{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"plan-1","type":"plan"}}}` + "\n" +
				`{"id":1,"result":{"turn":{"id":"turn-1"}}}` + "\n",
		},
		{
			name: "review", role: codingagent.Review, method: "review/start",
			response: `{"id":1,"result":{"turn":{"id":"turn-2"},"reviewThreadId":"thread-1"}}` + "\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &agentStartInput{}
			connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(test.response))}
			call := codingagent.Assignment{Role: test.role, Route: codingagent.Route{Model: "fixture-model", Effort: "high"}}
			turnID, _, pending, err := connection.startAgentTurn(call, thread, "exact assignment")
			if err != nil {
				t.Fatal(err)
			}
			if (len(pending) > 0) != test.wantNotice || turnID == "" {
				t.Fatalf("pending=%d turn=%q", len(pending), turnID)
			}
			var request struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(bytes.NewReader(input.Bytes())).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Method != test.method || request.Params["threadId"] != "thread-1" {
				t.Fatalf("wrong native request: %+v", request)
			}
			if test.role == codingagent.Plan {
				mode := request.Params["collaborationMode"].(map[string]any)
				settings := mode["settings"].(map[string]any)
				if mode["mode"] != "plan" || settings["model"] != "fixture-model" || settings["reasoning_effort"] != "high" || request.Params["outputSchema"] == nil {
					t.Fatalf("native plan settings changed: %+v", request.Params)
				}
			} else {
				target := request.Params["target"].(map[string]any)
				if request.Params["delivery"] != "inline" || target["type"] != "custom" || target["instructions"] != "exact assignment" || request.Params["outputSchema"] != nil || request.Params["input"] != nil {
					t.Fatalf("native review request changed: %+v", request.Params)
				}
			}
		})
	}
}

func TestNativeReviewRejectsDifferentInlineThread(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(`{"id":1,"result":{"turn":{"id":"turn"},"reviewThreadId":"foreign"}}` + "\n"))}
	thread := threadResult{Model: "fixture-model"}
	thread.Thread.ID = "thread"
	_, _, _, err := connection.startAgentTurn(codingagent.Assignment{Role: codingagent.Review, Route: codingagent.Route{Model: "fixture-model"}}, thread, "review")
	if err == nil || !strings.Contains(err.Error(), "different thread") {
		t.Fatalf("foreign native review accepted: %v", err)
	}
}

var _ io.WriteCloser = (*agentStartInput)(nil)
