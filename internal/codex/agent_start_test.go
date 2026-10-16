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
			name: "review", role: codingagent.Review,
			response: `{"id":1,"result":{"turn":{"id":"turn-2"}}}` + "\n",
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
			if request.Params["collaborationMode"] != nil || request.Params["outputSchema"] == nil || request.Params["target"] != nil || request.Params["delivery"] != nil {
				t.Fatalf("isolated schema-bound settings changed: %+v", request.Params)
			}
		})
	}
}

func TestNativePlanningStationsUseTypedSchemasWithoutLegacyPlanMode(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.PlanningInitial, codingagent.PlanningMutation, codingagent.PlanningCoverage, codingagent.PlanningAudit} {
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
			default:
				if properties["covered"] == nil || properties["suggestions"] == nil || properties["mutation"] != nil {
					t.Fatal("judgment station omitted coverage contract")
				}
			}
		})
	}
}

var _ io.WriteCloser = (*agentStartInput)(nil)
