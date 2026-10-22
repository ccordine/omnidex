package codingagent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

type directPlanningProvider struct {
	t      *testing.T
	source string
	task   LedgerTask
	roles  []Role
}

func (p *directPlanningProvider) Run(_ context.Context, call Assignment, emit func(NativeEvent) error) (Result, error) {
	p.roles = append(p.roles, call.Role)
	var content json.RawMessage
	var err error
	switch call.Role {
	case Plan:
		content, err = json.Marshal(p.source)
	case PlanningInitial:
		if call.PlanningResponse != p.source {
			p.t.Fatal("extraction did not receive the complete original response")
		}
		prompt, renderErr := RenderParserPrompt(call)
		if renderErr != nil || !strings.HasSuffix(prompt, p.source) {
			p.t.Fatal("renderer changed the source", renderErr)
		}
		for _, removed := range []string{"group_start", "source unit", "task_support", "planning_unit"} {
			if strings.Contains(prompt, removed) {
				p.t.Fatal("removed parser stage returned", removed)
			}
		}
		content, err = json.Marshal(map[string]any{"ledger": map[string]any{"tasks": []LedgerTask{p.task}}, "commentary": "The task and its details belong together."})
		if err == nil {
			content, err = MapParserResponse(call, content)
		}
	default:
		p.t.Fatalf("unexpected extra inference stage: %s", call.Role)
	}
	if err != nil {
		return Result{}, err
	}
	if err := emit(NativeEvent{Frame: json.RawMessage(`{"fixture":"complete-response"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true,
		ThreadID: "native-plan", TurnID: "native-turn", Content: content}, nil
}

func TestCompleteTaskDetailsReachLedgerInOneExtraction(t *testing.T) {
	for _, source := range []string{
		"1. Format the diagnostic text. Preserve the original timestamp.\n\nFiles:\n- input.go: read timestamps\n- output.go: render text\n\nAcceptance: output retains the timestamp.\nVerification: run the timestamp fixture.",
		"Export the selected records.\n\nKeep their order.\n\nUse output.go for serialization and input.go for input. Verify empty and populated exports.\n\nDo not change the input records.",
	} {
		t.Run(source[:20], func(t *testing.T) {
			task := packetFixture()
			task.Goal = source
			provider := &directPlanningProvider{t: t, source: source, task: task}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(),
				Evidence: &durableTaskEvidence{}, Environment: durableTaskEnvironment{},
				Providers:     map[string]Provider{"fixture": provider},
				Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			proposal, err := controller.ProposePlanningLedger(t.Context(), PlanningTurn{OperationID: "direct", OwnerTurnID: "owner", Text: "Implement the requested task."})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(provider.roles, []Role{Plan, PlanningInitial}) || len(proposal.Ledger.Tasks) != 1 {
				t.Fatalf("extra stages or task splitting: %v %+v", provider.roles, proposal.Ledger)
			}
			actual := proposal.Ledger.Tasks[0]
			if actual.ID == task.ID || actual.ID == "" || actual.ReviewState != model.LedgerReviewProposed {
				t.Fatal("model supplied ledger identity or lifecycle", actual.LedgerItem)
			}
			if actual.Goal != source || actual.Objective != task.Objective || !reflect.DeepEqual(actual.Constraints, task.Constraints) ||
				!reflect.DeepEqual(actual.FileResponsibilities, task.FileResponsibilities) || !reflect.DeepEqual(actual.WriteFiles, task.WriteFiles) ||
				actual.OwnerContext[0].Text != task.OwnerContext[0].Text || actual.Acceptance[0].Text != task.Acceptance[0].Text ||
				!reflect.DeepEqual(actual.Acceptance[0].Verification, task.Acceptance[0].Verification) {
				t.Fatalf("task details changed or disappeared: %+v", actual)
			}
		})
	}
}

func TestDirectTaskAmendmentRetainsDetailsAndCodeMetadata(t *testing.T) {
	task := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 3, Tasks: []LedgerTask{task}}
	call := Assignment{Role: PlanningMutation, PlanningLedger: &ledger, PlanningResponse: "Change only the objective."}
	raw, err := MapParserResponse(call, json.RawMessage(`{"operations":[{"kind":"amend","target_id":"packet","item":{"task":{"objective":"Updated objective.","id":17,"review_state":"agreed","acceptance":null,"extra_description":"irrelevant metadata"},"commentary":"Only the objective changed."},"rationale":"Requested edit."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Operations []model.LedgerMutationOperation `json:"operations"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	want := task
	want.Objective = "Updated objective."
	if len(result.Operations) != 1 || !reflect.DeepEqual(*result.Operations[0].Item.Task, want) {
		t.Fatalf("amendment changed retained details or metadata: %s", raw)
	}
	if !reflect.DeepEqual(ledger.Tasks[0], task) {
		t.Fatal("mapping mutated the retained ledger")
	}
}

func TestDirectReviewExtractionAllowsPresentationDifferences(t *testing.T) {
	call := Assignment{Role: ReviewResponse, Task: &Task{ID: "task"}, Response: "The review found a missing result in the implementation."}
	raw, err := MapParserResponse(call, json.RawMessage(`{"kind":"done","findings":[{"observation":"The required result is missing.","extra":"context"}],"verification":"A shorter summary.","commentary":"review complete"}`))
	if err != nil {
		t.Fatal(err)
	}
	var review ReviewResponseResult
	if err := json.Unmarshal(raw, &review); err != nil {
		t.Fatal(err)
	}
	if review.Verification != call.Response || len(review.Findings) != 1 || review.Findings[0].TaskID != "task" || review.Findings[0].Observation != "The required result is missing." {
		t.Fatalf("review lost source evidence or findings: %+v", review)
	}
}

func TestTaskDetailsDoNotRequireArtificialCrossReferences(t *testing.T) {
	task := packetFixture()
	task.OwnerContext = []model.TaskContext{{Text: "Keep the existing interaction."}}
	task.FileResponsibilities = []model.TaskFileResponsibility{{Path: "mentioned-only.go", Responsibility: "Describe the implementation here."}}
	task.Acceptance = []model.TaskAcceptance{{Text: "Keep the existing behavior."}}
	ledger := EngineeringLedger{ID: "ledger", Revision: 1, Tasks: []LedgerTask{task}}
	if err := ledger.ValidateDraft(); err != nil {
		t.Fatal("task details were rejected for their structure", err)
	}
	if err := (Task{ID: string(task.ID), Goal: task.Goal, OwnerContext: task.OwnerContext,
		FileResponsibilities: task.FileResponsibilities, Acceptance: task.Acceptance}).Validate(); err != nil {
		t.Fatal("execution rejected the same task details", err)
	}
	for _, detail := range []string{"Keep the existing interaction.", "mentioned-only.go", "Keep the existing behavior."} {
		if !strings.Contains(task.Instructions(), detail) {
			t.Fatal("task lost its original detail", detail)
		}
	}
}
