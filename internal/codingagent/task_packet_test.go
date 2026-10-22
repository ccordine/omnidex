package codingagent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func packetFixture() LedgerTask {
	return LedgerTask{LedgerItem: LedgerItem{ID: "packet", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner-17", Revision: 1}, Rank: 1, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed},
		Goal: "Preserve the exact original owner instruction.", Objective: "Provide a separate Research destination.",
		OwnerContext: []model.TaskContext{{Text: "Read-only research is approved.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner-16", Revision: 1}}},
		Constraints:  []string{"Preserve accepted work."}, ReadFiles: []string{"input.go"}, WriteFiles: []string{"output.go"}, DeleteFiles: []string{},
		FileResponsibilities: []model.TaskFileResponsibility{{Path: "input.go", Action: "inspect", Responsibility: "Inspect routing."}, {Path: "output.go", Action: "create", Responsibility: "Implement the approved destination."}},
		Acceptance:           []model.TaskAcceptance{{ID: "research", Text: "Separate Research destination retains read-only authorization.", Verification: []string{"Run routing and read-only regression tests."}}}}
}

func TestTaskPacketSparseAmendmentsPreserveCompleteObligation(t *testing.T) {
	task := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 1, Tasks: []LedgerTask{task}}
	for _, patch := range []string{`{"goal":"Amended instruction."}`, `{"goal":"Amended instruction.","objective":null,"owner_context":null,"constraints":null,"file_responsibilities":null,"acceptance":null}`} {
		operations, err := decodePlanningOperations(json.RawMessage(`{"operations":[{"kind":"amend","target_id":"packet","item":{"task":`+patch+`},"rationale":"Owner amendment."}]}`), ledger)
		if err != nil {
			t.Fatal(err)
		}
		next := *operations[0].Item.Task
		expected := task
		expected.Goal = "Amended instruction."
		if !reflect.DeepEqual(next, expected) {
			t.Fatalf("sparse patch lost frozen fields: %+v", next)
		}
	}
	raw, err := mergePlanningPatch(ledger, "packet", json.RawMessage(`{"task":{"constraints":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var item model.LedgerMutationItem
	if err = json.Unmarshal(raw, &item); err != nil || len(item.Task.Constraints) != 0 || !reflect.DeepEqual(item.Task.Acceptance, task.Acceptance) {
		t.Fatal("explicit clear changed unrelated acceptance", err)
	}
	for _, raw := range []string{`{"task":{"provenance":{}}}`, `{"task":{"id":"other"}}`, `{"task":{"not_a_field":[]}}`} {
		normalized, err := mergePlanningPatch(ledger, "packet", json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		var unchanged model.LedgerMutationItem
		if err := json.Unmarshal(normalized, &unchanged); err != nil || !reflect.DeepEqual(*unchanged.Task, task) {
			t.Fatal("model metadata changed the retained task", err)
		}
	}
}

func TestTaskPacketPlanningProjectionAndSchemaRetainFields(t *testing.T) {
	task := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 1, Tasks: []LedgerTask{task}}
	for _, role := range []Role{Plan, PlanningMutation} {
		raw, err := json.Marshal(PromptContext(Assignment{Role: role, PlanningLedger: &ledger}))
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{task.Objective, task.OwnerContext[0].Text, "owner-16", task.Constraints[0], task.FileResponsibilities[0].Responsibility, task.Acceptance[0].Text, task.Acceptance[0].Verification[0]} {
			if !strings.Contains(string(raw), text) {
				t.Fatal("planning context lost", text, string(raw))
			}
		}
	}
	raw, _ := json.Marshal(ParserOutputSchema(Assignment{Role: PlanningInitial}))
	for _, field := range []string{"objective", "owner_context", "constraints", "file_responsibilities", "acceptance", "verification"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatal("schema lost", field)
		}
	}
	task.Acceptance[0].Verification = nil
	if err := (EngineeringLedger{ID: "details", Revision: 1, Tasks: []LedgerTask{task}}).ValidateDraft(); err != nil {
		t.Fatal("task details required an invented verification grouping", err)
	}
}

func TestTaskPacketConsecutiveSparseAmendmentsCompose(t *testing.T) {
	task := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 1, Tasks: []LedgerTask{task}}
	operations, err := decodePlanningOperations(json.RawMessage(`{"operations":[{"kind":"amend","target_id":"packet","item":{"task":{"objective":"Updated objective."}},"rationale":"Objective."},{"kind":"amend","target_id":"packet","item":{"task":{"constraints":["Updated constraint."]}},"rationale":"Constraint."}]}`), ledger)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 {
		t.Fatalf("operations: %+v", operations)
	}
	for _, op := range operations {
		if err := applyPlanningOperation(&ledger, op); err != nil {
			t.Fatal(err)
		}
	}
	expected := task
	expected.Objective = "Updated objective."
	expected.Constraints = []string{"Updated constraint."}
	if !reflect.DeepEqual(ledger.Tasks[0], expected) {
		t.Fatalf("consecutive amendments lost fields: %+v", ledger.Tasks[0])
	}
	if !reflect.DeepEqual(task, packetFixture()) {
		t.Fatal("caller task mutated")
	}
}

func TestTaskPacketRecoveryAndResumeClarifications(t *testing.T) {
	entries := []Entry{
		{ID: 12, TaskID: "packet", Phase: "response_recovery", Result: &Result{CallID: 10}, Notes: []string{"Use approved runtime facts."}},
		{ID: 13, TaskID: "other", Phase: "response_recovery", Result: &Result{CallID: 11}, Notes: []string{"Unrelated."}},
		{ID: 14, TaskID: "packet", Phase: "task_resume", Session: &ExecutionSession{ThreadID: "original"}, Notes: []string{"Missing runtime context.", "The read-only endpoint is available."}},
		{ID: 15, TaskID: "packet", Phase: "task_resume", Notes: []string{"Malformed."}},
	}
	got := ExtractTaskClarifications(entries, "packet")
	want := []TaskClarification{
		{Question: "Resume the interrupted task response.", Answer: entries[0].Notes[0], QuestionCallID: 10, AnswerEvidenceID: 12},
		{Question: entries[2].Notes[0], Answer: entries[2].Notes[1], AnswerEvidenceID: 14},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lost durable answers/sources: %+v", got)
	}
}
