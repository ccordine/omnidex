package codingagent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type reviewOwnerScopeProvider struct {
	*reviewScopeRoutingProvider
	want []TaskClarification
}

func (p *reviewOwnerScopeProvider) Run(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
	if call.Role == ReviewResponse {
		if !reflect.DeepEqual(call.Clarifications, p.want) {
			p.t.Fatal("review parser lost owner clarifications")
		}
		if err := call.ValidateTaskContext(); err != nil {
			p.t.Fatal(err)
		}
		raw, err := json.Marshal(PromptContext(call))
		if err != nil {
			p.t.Fatal(err)
		}
		var rendered struct {
			Clarifications []TaskClarification `json:"clarifications"`
		}
		if err := json.Unmarshal(raw, &rendered); err != nil {
			p.t.Fatal(err)
		}
		if !reflect.DeepEqual(rendered.Clarifications, p.want) {
			p.t.Fatal("rendered parser context lost owner clarifications")
		}
	}
	return p.reviewScopeRoutingProvider.Run(ctx, call, event)
}

func TestReviewParserReceivesOwnerClarifications(t *testing.T) {
	task := Task{ID: "scope-task", Goal: "Complete the item and verify it under current owner directions."}
	want := []TaskClarification{{Question: "Continue fault checks?", Answer: "Move on; no further fault-test rounds for this item."}}
	base := &reviewScopeRoutingProvider{executionScopeProvider: executionScopeProvider{t: t, task: task}, kind: "done", source: "Review complete. No remaining defects within the owner-amended scope. Excluded fault checks remain unverified."}
	provider := &reviewOwnerScopeProvider{reviewScopeRoutingProvider: base, want: want}
	route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
	controller := Controller{JobID: 1, Generation: 1, Root: ".", Evidence: &durableTaskEvidence{}, Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Mode: Agents, Reviewer: route, LedgerParser: route}}
	scope := controller.assignment(Review, &task, route)
	scope.Clarifications = want
	review, _, err := controller.review(t.Context(), scope, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Findings) != 0 {
		t.Fatal("passing review acquired findings")
	}
}
