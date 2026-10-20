package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Scripted semantic answers verify the real controller, sieve and routing;
// the opt-in native corpus measures live interpretation separately.
type executionScopeProvider struct {
	t              *testing.T
	task           Task
	response, kind string
	roles          []Role
}

func (p *executionScopeProvider) Run(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
	p.roles = append(p.roles, call.Role)
	if call.Role == ExecutionResponse {
		if call.Task == nil || call.Task.Goal != p.task.Goal || call.Route.Effort != "high" {
			p.t.Fatal("execution parser lost frozen task or configured effort")
		}
		return RunParserSieve(ctx, call, event, sieveTestProvider(p.t, func(leaf Assignment) any {
			if leaf.ParserStep.TaskInstructions != p.task.Goal {
				p.t.Fatal("leaf lost frozen instructions")
			}
			prompt, err := RenderParserPrompt(leaf)
			if err != nil || !strings.Contains(prompt, "exact frozen task_instructions") || !strings.Contains(prompt, "Evaluate the EXECUTION assignment only") || !strings.Contains(prompt, "not prerequisites for execution completion") {
				p.t.Fatal("missing execution scope", err)
			}
			background, err := RenderParserContext(leaf)
			if err != nil || !strings.Contains(background, "task_instructions") {
				p.t.Fatal("missing task background", err)
			}
			kind := p.kind
			if p.kind == "done" && leaf.ParserStep.BlockID != "block_1" {
				kind = "ignore"
			}
			return scanOracle(leaf.ParserStep.Source, kind)
		}))
	}
	content, _ := json.Marshal(p.response)
	if call.Role == ReviewResponse {
		content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Independent verification complete."}`)
	}
	if call.Role == Execute || call.Role == Review {
		if call.Session != nil {
			p.t.Fatal("reused native session")
		}
	}
	if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"execution-scope"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "fresh-" + string(call.Role), TurnID: "turn-" + string(call.Role), Content: content}, nil
}
func TestFrozenExecutionScopeAndRouting(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/parser-sieve/job15-execution.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Task     Task
		Response string
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	var job21 struct {
		Task     Task
		Response string
	}
	raw21, err := os.ReadFile("../../testdata/parser-sieve/job21-execution.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw21, &job21); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, response, kind string }{
		{"job15", saved.Response, "done"},
		{"job21", job21.Response, "done"},
		{"later review pending", "Implementation and required checks complete. Required independent review remains pending.", "done"},
		{"review protocol check failed", "Required independent-review protocol verification failed at turn startup.", "error"},
		{"required failure", "Required verification failed.", "error"},
		{"essential question", "Which required endpoint should I verify? I need the answer to finish.", "question"},
		{"permission", "May I access the required verification endpoint?", "permission"},
		{"unresolved meaning", "The required verification is in an unresolved state.", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := saved.Task
			if tc.name == "job21" {
				task = job21.Task
			}
			task.ReadFiles = []string{"input.txt"}
			task.WriteFiles = []string{"output.txt"}
			provider := &executionScopeProvider{t: t, task: task, response: tc.response, kind: tc.kind}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
			store := &durableTaskEvidence{}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			err := controller.RunTask(t.Context(), task)
			switch tc.kind {
			case "done":
				if err != nil {
					t.Fatal(err)
				}
				if len(provider.roles) != 4 || provider.roles[2] != Review || provider.roles[3] != ReviewResponse {
					t.Fatalf("no independent review: %v", provider.roles)
				}
			case "error":
				var incomplete *IncompleteTaskError
				if !errors.As(err, &incomplete) {
					t.Fatalf("missing incomplete route: %v", err)
				}
			case "question", "permission":
				var input *ExecutionInputRequired
				if !errors.As(err, &input) || string(input.Kind) != tc.kind || input.Text != tc.response {
					t.Fatalf("lost input: %v", err)
				}
			case "unknown":
				var failure *ParserFailure
				if !errors.As(err, &failure) {
					t.Fatalf("unresolved meaning accepted: %v", err)
				}
			}
			if tc.kind != "done" && len(provider.roles) != 2 {
				t.Fatalf("review ran before execution resolved: %v", provider.roles)
			}
			found := false
			for _, entry := range store.entries {
				if entry.Phase == "execution_response" {
					found = true
					if entry.Notes[0] != tc.response || string(entry.ResponseKind) != tc.kind {
						t.Fatal("rewrote source or outcome")
					}
				}
			}
			if tc.kind != "unknown" && !found {
				t.Fatal("missing original execution evidence")
			}
		})
	}
}
func TestExecutionParserRequiresFrozenTask(t *testing.T) {
	call := Assignment{Role: ExecutionResponse, Response: "Complete."}
	if err := call.ValidateTaskContext(); err == nil {
		t.Fatal("accepted execution parser without frozen task")
	}
}
