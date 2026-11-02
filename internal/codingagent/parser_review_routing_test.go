package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

type reviewScopeRoutingProvider struct {
	executionScopeProvider
	kind, source, quote string
	reviews             int
}

func (p *reviewScopeRoutingProvider) Run(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
	if call.Role != Review && call.Role != ReviewResponse {
		return p.executionScopeProvider.Run(ctx, call, event)
	}
	p.roles = append(p.roles, call.Role)
	if call.Role == ReviewResponse {
		if call.Task == nil || call.Task.Instructions() != p.task.Instructions() {
			p.t.Fatal("review lost frozen scope")
		}
		findings := []map[string]string{}
		if p.quote != "" && p.reviews == 1 {
			findings = append(findings, map[string]string{"observation": p.quote, "file": "output.txt"})
		}
		kind := p.kind
		if p.reviews > 1 {
			kind = "done"
		}
		content, _ := json.Marshal(map[string]any{"kind": kind, "findings": findings})
		content, err := MapParserResponse(call, content)
		if err != nil {
			return Result{}, err
		}
		if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"review-response"}`)}); err != nil {
			return Result{}, err
		}
		return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, Content: content}, nil
	}
	p.reviews++
	if call.Session != nil {
		p.t.Fatal("review reused execution session")
	}
	source := p.source
	if p.reviews > 1 {
		source = "Independent review complete. Correction verified."
	}
	content, _ := json.Marshal(source)
	if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"review-routing"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: fmt.Sprintf("independent-review-%d", p.reviews), TurnID: "review-turn", Content: content}, nil
}
func TestFrozenReviewOutcomesRetainCorrectionAndInputRoutes(t *testing.T) {
	for _, tc := range []struct{ name, source, kind, quote string }{
		{"finding", "Review complete. output.txt omits the required result.", "done", "output.txt omits the required result."},
		{"qualification finding", "Review complete. output.txt omits the required result; qualification failed with exit 1. Correction and acceptance remain pending.", "done", "output.txt omits the required result; qualification failed with exit 1."},
		{"failure", "Required verification could not finish; the review is unfinished.", "error", ""},
		{"incomplete with findings", "Task incomplete: output.txt omits the required result and verification remains pending.", "error", "output.txt omits the required result."},
		{"missing evidence", "I inspected the source.", "error", ""},
		{"question", "Which required endpoint should I verify?", "question", ""},
		{"permission", "May I access the required verification endpoint?", "permission", ""},
		{"unknown", "The check has unresolved meaning.", "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := Task{ID: "scope-task", Goal: "Write output.txt with the required result and verify it.", WriteFiles: []string{"output.txt"}}
			p := &reviewScopeRoutingProvider{executionScopeProvider: executionScopeProvider{t: t, task: task, response: "Implementation and required checks complete.", kind: "done"}, kind: tc.kind, source: tc.source, quote: tc.quote}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
			store := &durableTaskEvidence{}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			err := controller.RunTask(t.Context(), task)
			switch tc.kind {
			case "done", "error":
				if err != nil {
					t.Fatal(err)
				}
				if fmt.Sprint(p.roles) != "[execute execution_response review review_response execute execution_response review review_response]" {
					t.Fatalf("lost correction sequence: %v", p.roles)
				}
				corrected := false
				observation, file := tc.quote, "output.txt"
				if tc.kind == "error" && tc.quote == "" {
					observation, file = tc.source, ""
				}
				for _, entry := range store.entries {
					if entry.Phase == "call_opened" && entry.Role == Execute && len(entry.Assignment.Findings) > 0 {
						f := entry.Assignment.Findings[0]
						corrected = f.TaskID == task.ID && f.File == file && f.Observation == observation
					}
				}
				if !corrected {
					t.Fatal("exact finding did not reach correction execution")
				}
			case "question", "permission":
				var input *ExecutionInputRequired
				if !errors.As(err, &input) || input.Role != Review || string(input.Kind) != tc.kind || input.Text != tc.source {
					t.Fatalf("lost essential question: %v", err)
				}
			case "unknown":
				var failure *ParserFailure
				if !errors.As(err, &failure) {
					t.Fatalf("lost unresolved meaning: %v", err)
				}
			}
			for _, entry := range store.entries {
				if entry.Phase == "review_response" && entry.Round == 1 && entry.Notes[0] != tc.source {
					t.Fatal("review evidence rewritten")
				}
			}
		})
	}
}
