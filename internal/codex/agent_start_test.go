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

func TestNativeAgentRolesUseOnlyIsolatedSchemaBoundTurns(t *testing.T) {
	effort := "high"
	thread := threadResult{Model: "fixture-model", ReasoningEffort: &effort}
	thread.Thread.ID = "thread-1"
	for _, test := range []struct {
		name       string
		role       codingagent.Role
		response   string
		wantNotice bool
	}{
		{
			name: "planning", role: codingagent.PlanningInitial, wantNotice: true,
			response: `{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"message-1","type":"agentMessage"}}}` + "\n" +
				`{"id":1,"result":{"turn":{"id":"turn-1"}}}` + "\n",
		},
		{
			name: "normal planning", role: codingagent.Plan,
			response: `{"id":1,"result":{"turn":{"id":"turn-plan"}}}` + "\n",
		},
		{
			name: "planning mutation parser", role: codingagent.PlanningMutation,
			response: `{"id":1,"result":{"turn":{"id":"turn-mutation"}}}` + "\n",
		},
		{
			name: "review", role: codingagent.Review,
			response: `{"id":1,"result":{"turn":{"id":"turn-2"}}}` + "\n",
		},
		{
			name: "normal execution", role: codingagent.Execute,
			response: `{"id":1,"result":{"turn":{"id":"turn-3"}}}` + "\n",
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
			if request.Method != "turn/start" || request.Params["threadId"] != "thread-1" {
				t.Fatalf("wrong native request: %+v", request)
			}
			if test.role.IsNative() {
				policy := request.Params["sandboxPolicy"].(map[string]any)
				if request.Params["outputSchema"] != nil || policy["type"] != "dangerFullAccess" || request.Params["approvalPolicy"] != "never" {
					t.Fatalf("execution is not an ordinary full-access turn: %+v", request.Params)
				}
			} else if request.Params["collaborationMode"] != nil || (request.Params["outputSchema"] != nil) != (test.role != codingagent.PlanningMutation) || request.Params["target"] != nil || request.Params["delivery"] != nil {
				t.Fatalf("isolated schema-bound settings changed: %+v", request.Params)
			}
		})
	}
}

func TestNativePlanningStationsUseTypedSchemasWithoutLegacyPlanMode(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.PlanningInitial, codingagent.PlanningMutation} {
		t.Run(string(role), func(t *testing.T) {
			schema := agentOutputSchema(codingagent.Assignment{Role: role})
			properties := schema["properties"].(map[string]any)
			if properties["tasks"] != nil || properties["questions"] != nil {
				t.Fatalf("planning station inherited legacy workload schema: %#v", properties)
			}
			switch role {
			case codingagent.PlanningInitial:
				if properties["ledger"] == nil {
					t.Fatal("initial station omitted ledger contract")
				}
			case codingagent.PlanningMutation:
				if properties["operations"] == nil {
					t.Fatal("mutation station omitted operations contract")
				}
			}
		})
	}
}

var _ io.WriteCloser = (*agentStartInput)(nil)

func TestNativeThreadsKeepNormalCodexToolsAndInstructions(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			input := &agentStartInput{}
			connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
				`{"id":1,"result":{}}` + "\n" +
					`{"id":2,"result":{"model":"fixture-model","thread":{"id":"execution-thread"}}}` + "\n"))}
			call := codingagent.Assignment{Role: role, Root: "/workspace", Route: codingagent.Route{Model: "fixture-model"}}
			if _, _, err := connection.startAgentThread(call, ""); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(input.Bytes()))
			var request struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			for decoder.Decode(&request) == nil {
				if request.Method != "thread/start" {
					continue
				}
				if request.Params["sandbox"] != "danger-full-access" || request.Params["approvalPolicy"] != "never" || request.Params["cwd"] != "/workspace" {
					t.Fatalf("executor thread lost normal full-access settings: %#v", request.Params)
				}
				if request.Params["ephemeral"] != (role != codingagent.Execute) {
					t.Fatal("native role lost its owned thread lifecycle")
				}
				for _, field := range []string{"dynamicTools", "config", "baseInstructions", "outputSchema"} {
					if _, present := request.Params[field]; present {
						t.Fatalf("executor retained a special %s override", field)
					}
				}
				return
			}
			t.Fatal("executor never opened a normal Codex thread")
		})
	}
}
