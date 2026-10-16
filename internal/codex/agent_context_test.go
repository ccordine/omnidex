package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentSharedContextIsInjectedBeforeTheLiteralUserTurn(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review,
		codingagent.PlanningInitial, codingagent.PlanningMutation, codingagent.ExecutionResponse, codingagent.ReviewResponse} {
		t.Run(string(role), func(t *testing.T) {
			input := &agentStartInput{}
			connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
				`{"id":1,"result":{}}` + "\n" + `{"id":2,"result":{"turn":{"id":"turn"}}}` + "\n"))}
			call := codingagent.Assignment{Role: role, Context: "Only the compiled shared context.",
				Objective: &codingagent.Objective{Instruction: "  Reprioritize this item.\nKeep my wording.  "},
				Task:      &codingagent.Task{Goal: "Raw context must not be duplicated."}}
			prompt, err := renderAgentAssignment(call)
			if err != nil {
				t.Fatal(err)
			}
			if role == codingagent.Plan && prompt != call.Objective.Instruction {
				t.Fatal("planning rewrote the current owner message")
			}
			if _, err := connection.injectAgentContext(call, "thread"); err != nil {
				t.Fatal(err)
			}
			thread := threadResult{}
			thread.Thread.ID = "thread"
			if _, _, _, err := connection.startAgentTurn(call, thread, prompt); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(input.Bytes()))
			var injection struct {
				Method string `json:"method"`
				Params struct {
					ThreadID string                `json:"threadId"`
					Items    []agentHistoryMessage `json:"items"`
				} `json:"params"`
			}
			if err := decoder.Decode(&injection); err != nil {
				t.Fatal(err)
			}
			if injection.Method != "thread/inject_items" || injection.Params.ThreadID != "thread" || len(injection.Params.Items) != 1 {
				t.Fatalf("shared context was not injected as history: %+v", injection)
			}
			item := injection.Params.Items[0]
			if item.Type != "message" || item.Role != "assistant" || len(item.Content) != 1 || item.Content[0].Type != "output_text" {
				t.Fatalf("context acquired instruction authority: %+v", item)
			}
			if strings.Count(item.Content[0].Text, call.Context) != 1 || strings.Contains(item.Content[0].Text, call.Task.Goal) || strings.Contains(item.Content[0].Text, call.Objective.Instruction) {
				t.Fatal("history duplicated raw context or the current request")
			}
			var turn struct {
				Method string `json:"method"`
				Params struct {
					ThreadID string `json:"threadId"`
					Input    []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"input"`
				} `json:"params"`
			}
			if err := decoder.Decode(&turn); err != nil {
				t.Fatal(err)
			}
			if turn.Method != "turn/start" || turn.Params.ThreadID != "thread" || len(turn.Params.Input) != 1 || turn.Params.Input[0].Type != "text" || turn.Params.Input[0].Text != prompt || strings.Contains(prompt, call.Context) {
				t.Fatalf("literal user message contains shared context: %+v", turn)
			}
		})
	}
}

func TestParserCurrentSourceIsExactAndSeparateFromSharedHistory(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.PlanningInitial, codingagent.PlanningMutation, codingagent.ExecutionResponse, codingagent.ReviewResponse} {
		source := "  Exact new source:\n" + strings.Repeat("Unmodified current response. ", 6000) + "\n  "
		call := codingagent.Assignment{Role: role, PlanningResponse: source, Response: source, Context: "Only existing ledger/context."}
		prompt, err := renderAgentAssignment(call)
		if err != nil || !strings.HasSuffix(prompt, source) || strings.Count(prompt, source) != 1 || strings.Contains(prompt, call.Context) {
			t.Fatalf("%s changed the current parser source or stuffed shared context: %v", role, err)
		}
		history, err := renderAgentContext(call)
		if err != nil || !strings.Contains(history, call.Context) || strings.Contains(history, source) {
			t.Fatalf("%s duplicated the current source in history: %v", role, err)
		}
		contract, err := json.Marshal(agentOutputSchema(call))
		if err != nil || strings.Count(history, string(contract)) != 1 || strings.Contains(prompt, string(contract)) {
			t.Fatalf("%s parser API contract is absent, duplicated, or stuffed into the current prompt: %v", role, err)
		}
	}
}

func TestContextInjectionFailureHasNoPromptFallback(t *testing.T) {
	input := &agentStartInput{}
	connection := &connection{input: input, output: bufio.NewScanner(strings.NewReader(
		`{"id":1,"error":{"code":-32601,"message":"unsupported"}}` + "\n"))}
	if _, err := connection.injectAgentContext(codingagent.Assignment{Context: "background"}, "thread"); err == nil {
		t.Fatal("failed history injection was silently accepted")
	}
	if strings.Contains(input.String(), "turn/start") {
		t.Fatal("failed injection fell back to a literal context prompt")
	}
}

func TestInstalledCodexPlanningUsesInjectedHistory(t *testing.T) {
	client, model := installedAgentClient(t)
	call := codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.Plan, Root: t.TempDir(),
		Route:     codingagent.Route{Provider: "codex", Model: model, Effort: "low", ServiceTier: "default"},
		Objective: &codingagent.Objective{Instruction: "What name did we agree on for the pilot application? Answer only; do not implement anything."},
		Context:   "Earlier planning conversation:\nuser: Name the pilot application Copper Finch.\nassistant: Agreed. The pilot application is named Copper Finch."}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := client.Run(ctx, call, nil)
	if err != nil {
		t.Fatal(err)
	}
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil || !result.Completed || !strings.Contains(response, "Copper Finch") {
		t.Fatalf("native planning did not receive separate history: completed=%v content=%s error=%v", result.Completed, result.Content, err)
	}
	t.Logf("native planning answered using injected history: thread=%s turn=%s", result.ThreadID, result.TurnID)
}
