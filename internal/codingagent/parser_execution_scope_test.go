package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Scripted semantic answers verify the real controller and routing;
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
		prompt, err := RenderParserPrompt(call)
		if err != nil || !strings.Contains(prompt, "frozen task instructions") {
			p.t.Fatal("missing execution scope", err)
		}
		background, err := RenderParserContext(call)
		instructions, _ := json.Marshal(p.task.Instructions())
		if err != nil || !strings.Contains(background, string(instructions)) {
			p.t.Fatal("missing frozen task background", err)
		}
		content, _ := json.Marshal(map[string]string{"kind": p.kind})
		if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"execution-response"}`)}); err != nil {
			return Result{}, err
		}
		return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, Content: content}, nil
	}
	content, _ := json.Marshal(p.response)
	if call.Role == ReviewResponse {
		content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Independent verification complete."}`)
	}
	if call.Role == Execute || call.Role == Review {
		if call.Role == Execute && call.ReviewFeedback != "" {
			if call.Session == nil || call.Session.ThreadID != "fresh-execute" || call.Session.TurnID != "turn-execute" {
				p.t.Fatal("review feedback lost its executor session")
			}
		} else if call.Session != nil {
			p.t.Fatal("initial execution or independent review reused a session")
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
		{"later ledger items pending", "This ledger item's implementation and required verification are complete. Independent review and the separate subsequent ledger items remain outstanding.", "done"},
		{"review protocol check failed", "Required independent-review protocol verification cannot start because its configured provider is unavailable.", "error"},
		{"required failure", "Required verification cannot run because its configured runtime is unavailable.", "error"},
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
