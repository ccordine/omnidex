package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// A process stop after a completed report must reconstitute identical evidence
// for review without repeating native execution or changing the report's claims.
func TestExecutionEvidenceSurvivesReplay(t *testing.T) {
	task := Task{ID: "task", Goal: "Implement the converter and prove its checksum."}
	report := "Execution report concluded. Implementation is incomplete and required checksum proof is missing."
	store := &durableTaskEvidence{}
	route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
	p := &evidenceReviewProvider{t: t, report: report}
	c := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Environment: durableTaskEnvironment{}, Evidence: store, Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
	// Persist precisely the boundaries a stopped process would leave behind.
	original, _ := json.Marshal(report)
	store.Append(t.Context(), Entry{Phase: "call_completed", Role: Execute, TaskID: task.ID, Round: 1, Result: &Result{CallID: 8, Content: original, ThreadID: "execution", TurnID: "turn"}})
	if err := c.RunTask(t.Context(), task); err == nil {
		t.Fatal("review evidence probe must stop before acceptance")
	}
	if p.reviews != 1 {
		t.Fatal(p.reviews)
	}
	// Restart again: the execution parser receipt is durable, native work stays intact.
	if err := c.RunTask(t.Context(), task); err == nil || p.reviews != 2 {
		t.Fatalf("review replay: %v reviews=%d", err, p.reviews)
	}
	for _, e := range store.entries {
		if e.Phase == "execution_response" && (!reflect.DeepEqual(e.Notes, []string{report}) || e.ResponseKind != ExecutionDone) {
			t.Fatal("changed original report")
		}
	}
}

type evidenceReviewProvider struct {
	t              *testing.T
	report         string
	calls, reviews int
}

func (p *evidenceReviewProvider) Run(_ context.Context, a Assignment, emit func(NativeEvent) error) (Result, error) {
	p.calls++
	var content json.RawMessage
	switch a.Role {
	case Execute:
		return Result{}, errors.New("replayed execution")
	case ExecutionResponse:
		content = json.RawMessage(`{"kind":"done"}`)
	case Review:
		p.reviews++
		if a.ExecutionReport == nil || a.ExecutionReport.Response != p.report || a.ExecutionReport.CallID != 8 {
			p.t.Fatalf("review lost original evidence: %+v", a)
		}
		raw, _ := json.Marshal(PromptContext(a))
		var ctx map[string]json.RawMessage
		json.Unmarshal(raw, &ctx)
		if len(ctx["execution_report"]) == 0 {
			p.t.Fatal("rendered context lost original evidence")
		}
		return Result{}, fmt.Errorf("intentional stop after inspecting review context; no acceptance claimed")
	case ReviewResponse:
		p.t.Fatal("probe must not reach acceptance parsing")
	}
	if err := emit(NativeEvent{Frame: json.RawMessage(`{"fixture":"replay-only"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: a.Route.Provider, Model: a.Route.Model, Completed: true, ThreadID: "review", TurnID: "turn", Content: content}, nil
}
