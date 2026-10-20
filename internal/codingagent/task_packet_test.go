package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		if _, err := mergePlanningPatch(ledger, "packet", json.RawMessage(raw)); err == nil {
			t.Fatal("invalid patch accepted", raw)
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
	if task.ValidatePacket() == nil {
		t.Fatal("unbound acceptance was accepted")
	}
}

func TestTaskPacketParserKeepsSupportingBlocksOnOneTask(t *testing.T) {
	for _, role := range []Role{PlanningInitial, PlanningMutation} {
		t.Run(string(role), func(t *testing.T) {
			source := "**One task: restore routing.**\n\nInspect input.go. Create output.go.\n\nAcceptance: Research is separate and read-only.\n\nVerify with routing tests.\n"
			call := Assignment{Role: role, JobID: 1, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningResponse: source}
			base := EngineeringLedger{ID: "ledger", Revision: 1, Tasks: []LedgerTask{}}
			call.PlanningLedger = &base
			projections := 0
			provider := sieveTestProvider(t, func(call Assignment) any {
				step := call.ParserStep
				if step.Stage == "scan" {
					kind := "task_support"
					if step.BlockID == "block_1" {
						kind = "task"
					}
					return scanOracle(step.Source, kind)
				}
				projections++
				if step.Category == "task" {
					return map[string]any{"operations": []any{map[string]any{"kind": "add", "rationale": "One requested task.", "item": map[string]any{"task": map[string]any{"id": step.NewIDs["task"], "goal": "restore routing", "read_files": []string{}, "write_files": []string{}, "delete_files": []string{}}}}}}
				}
				if len(step.Ledger.Tasks) != 1 {
					t.Fatal("support became queued work")
				}
				patch := map[string]any{}
				if strings.Contains(step.Source, "input.go") {
					patch["read_files"] = []string{"input.go"}
					patch["write_files"] = []string{"output.go"}
					patch["file_responsibilities"] = []model.TaskFileResponsibility{{Path: "output.go", Action: "create", Responsibility: "Restore routing."}}
				} else if strings.Contains(step.Source, "Acceptance:") {
					patch["acceptance"] = []model.TaskAcceptance{{ID: "separate", Text: "Research is separate and read-only."}}
				} else {
					patch["acceptance"] = []model.TaskAcceptance{{ID: "separate", Text: "Research is separate and read-only.", Verification: []string{"Run routing tests."}}}
				}
				return map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": step.Ledger.Tasks[0].ID, "rationale": "Supporting details.", "item": map[string]any{"task": patch}}}}
			})
			result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, provider)
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Ledger     EngineeringLedger               `json:"ledger"`
				Operations []model.LedgerMutationOperation `json:"operations"`
			}
			if err = json.Unmarshal(result.Content, &out); err != nil {
				t.Fatal(err)
			}
			if role == PlanningMutation {
				mutation := LedgerMutation{OperationID: "packet", OwnerTurnID: "owner", BaseRevision: 1, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{out.Operations[0].Item.Task.ID}), Operations: out.Operations}
				out.Ledger, err = ApplyPlanningMutation(base, mutation)
				if err != nil {
					t.Fatal("complete parser result rejected at mutation boundary", err)
				}
				if len(out.Operations) != 1 {
					t.Fatalf("support remained intermediate mutations: %d", len(out.Operations))
				}
			}
			if len(out.Ledger.Tasks) != 1 || projections != 4 {
				t.Fatalf("one task split: %+v", out)
			}
			task := out.Ledger.Tasks[0]
			if len(task.ReadFiles) != 1 || len(task.WriteFiles) != 1 || len(task.FileResponsibilities) != 1 || len(task.Acceptance) != 1 || len(task.Acceptance[0].Verification) != 1 {
				t.Fatalf("support lost: %+v", task)
			}
			for _, block := range strings.Split(strings.TrimSpace(source), "\n\n") {
				if !strings.Contains(task.Goal, block) {
					t.Fatal("lost original source", block)
				}
			}

		})
	}
}

func TestTaskPacketSupportingListsAccumulateWithoutReplacingEarlierDetails(t *testing.T) {
	original := packetFixture()
	projected := original
	projected.ReadFiles = []string{"another.go"}
	projected.WriteFiles = []string{"another_test.go"}
	projected.Acceptance = []model.TaskAcceptance{{ID: "second", Text: "Second criterion.", Verification: []string{"Run second check."}}}
	merged, err := mergePlanningSupport(original, projected)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.ReadFiles) != 2 || len(merged.WriteFiles) != 2 || len(merged.Acceptance) != 2 || !reflect.DeepEqual(merged.Acceptance[0], original.Acceptance[0]) {
		t.Fatalf("support erased earlier details: %+v", merged)
	}
	projected.Acceptance = []model.TaskAcceptance{{ID: original.Acceptance[0].ID, Text: "A different obligation.", Verification: []string{"Another check."}}}
	if _, err = mergePlanningSupport(original, projected); err == nil {
		t.Fatal("support replaced acceptance by reusing its identity")
	}
}

func TestTaskPacketSharedInstructionsRetainSourcesWithoutQueueItems(t *testing.T) {
	source := "Implement the requested task.\n\nPreserve accepted work; do not commit.\n"
	result, err := RunParserSieve(t.Context(), Assignment{JobID: 1, Role: PlanningInitial, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningResponse: source}, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(call Assignment) any {
		step := call.ParserStep
		if step.Stage == "scan" {
			kind := "task"
			if step.BlockID == "block_2" {
				kind = "project_instruction"
			}
			return scanOracle(step.Source, kind)
		}
		return map[string]any{"operations": []any{map[string]any{"kind": "add", "rationale": "One task.", "item": map[string]any{"task": map[string]any{"id": step.NewIDs["task"], "goal": "Requested task.", "read_files": []string{}, "write_files": []string{}, "delete_files": []string{}}}}}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Ledger EngineeringLedger `json:"ledger"`
	}
	if err = json.Unmarshal(result.Content, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Ledger.Tasks) != 1 || len(out.Ledger.Tasks[0].OwnerContext) != 1 {
		t.Fatal("shared instruction became queued work or lost context")
	}
	context := out.Ledger.Tasks[0].OwnerContext[0]
	if context.Text != "Preserve accepted work; do not commit." || !strings.Contains(context.Provenance.Reference, "block_2") || context.Provenance.Kind != "final_response_statement" {
		t.Fatalf("shared instruction lost source: %+v", context)
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

func TestTaskPacketParserRejectsUnboundVerificationAtFinalBoundary(t *testing.T) {
	source := "Implement the task.\n\nAcceptance: The task works.\n"
	_, err := RunParserSieve(t.Context(), Assignment{JobID: 1, Role: PlanningInitial, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningResponse: source}, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(call Assignment) any {
		step := call.ParserStep
		if step.Stage == "scan" {
			kind := "task"
			if step.BlockID == "block_2" {
				kind = "task_support"
			}
			return scanOracle(step.Source, kind)
		}
		if step.Category == "task" {
			return map[string]any{"operations": []any{map[string]any{"kind": "add", "rationale": "One task.", "item": map[string]any{"task": map[string]any{"id": step.NewIDs["task"], "goal": source, "read_files": []string{}, "write_files": []string{}, "delete_files": []string{}}}}}}
		}
		return map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": step.Ledger.Tasks[0].ID, "rationale": "Criterion.", "item": map[string]any{"task": map[string]any{"acceptance": []model.TaskAcceptance{{ID: "works", Text: "The task works."}}}}}}}
	}))
	if err == nil || !strings.Contains(err.Error(), "bound verification") {
		t.Fatalf("unbound final packet accepted: %v", err)
	}
}

func TestTaskPacketParserContextProjectsOnlyRelevantRecords(t *testing.T) {
	owner := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 57, Tasks: []LedgerTask{owner}}
	for i := 0; i < 35; i++ {
		retired := packetFixture()
		retired.ID = model.LedgerItemID(fmt.Sprintf("retired-%d", i))
		retired.Disposition = model.LedgerDispositionTombstoned
		retired.Goal = strings.Repeat("Preserved retired source. ", 100)
		ledger.Tasks = append(ledger.Tasks, retired)
	}
	other := packetFixture()
	other.ID = "other"
	other.Goal = strings.Repeat("Unrelated active obligation. ", 1000)
	ledger.Tasks = append(ledger.Tasks, other)
	before, _ := json.Marshal(ledger)
	for _, part := range []ParserPart{{Kind: "change", Text: "Amend only packet."}, {Kind: "task_support", Text: "Verify the owning task."}} {
		projected := planningParserContext(ledger, part, owner.ID, nil)
		if len(projected.Tasks) != 1 || !reflect.DeepEqual(projected.Tasks[0], owner) {
			t.Fatal("projection lost owner or included unrelated background")
		}
		step := ParserStep{Stage: "planning_unit", Source: part.Text, Ledger: &projected}
		if _, err := renderParserStepContext(Assignment{Role: PlanningMutation, ParserStep: &step}); err != nil {
			t.Fatal(err)
		}
	}
	retired := ledger.Tasks[2]
	projected := planningParserContext(ledger, ParserPart{Kind: "change", Text: "Restore " + string(retired.ID)}, "", nil)
	if len(projected.Tasks) != 1 || !reflect.DeepEqual(projected.Tasks[0], retired) {
		t.Fatal("explicit retired target lost source")
	}
	after, _ := json.Marshal(ledger)
	if !bytes.Equal(before, after) {
		t.Fatal("projection changed durable records")
	}
}

func TestTaskPacketImplicitAmendmentUsesSourceProjectionAndFullMaterialization(t *testing.T) {
	owner := packetFixture()
	owner.Objective = "Restore durable task packets as the prerequisite."
	ledger := EngineeringLedger{ID: "ledger", Revision: 57, Tasks: []LedgerTask{owner}}
	for i := 0; i < 7; i++ {
		other := packetFixture()
		other.ID = model.LedgerItemID(fmt.Sprintf("original-%d", i))
		other.Rank = int64(i + 2)
		other.OwnerContext = []model.TaskContext{{Text: strings.Repeat("Approved supporting context. ", 180), Provenance: other.Provenance}}
		ledger.Tasks = append(ledger.Tasks, other)
	}
	before, _ := json.Marshal(ledger)
	source := "Amend the prerequisite objective."
	full := ParserStep{Stage: "planning_unit", Source: source, Ledger: &ledger}
	if _, err := renderParserStepContext(Assignment{Role: PlanningMutation, ParserStep: &full}); err == nil {
		t.Fatal("fixture must exceed the full-packet background limit")
	}
	call := Assignment{JobID: 1, Role: PlanningMutation, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningLedger: &ledger, PlanningResponse: source}
	result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(call Assignment) any {
		step := call.ParserStep
		if step.Stage == "scan" {
			return scanOracle(step.Source, "change")
		}
		if _, err := renderParserStepContext(call); err != nil {
			t.Fatal(err)
		}
		if step.Stage == "planning_targets" {
			if step.Ledger != nil {
				t.Fatal("reference leaf received full packets")
			}
			if step.TaskIndex[0].ID == "choice_0_0" {
				if len(step.TaskIndex) != 9 || step.TaskIndex[0].Goal != owner.Goal || step.TaskIndex[0].Objective != owner.Objective {
					t.Fatal("reference options lost exact source or objective")
				}
				return "choice_0_0"
			}
			if len(step.TaskIndex) != 8 {
				t.Fatal("selected candidate was not removed")
			}
			return "choice_1_end"
		}
		if len(step.Ledger.Tasks) != 1 || !reflect.DeepEqual(step.Ledger.Tasks[0], owner) {
			t.Fatal("amendment extraction did not receive the full selected packet")
		}
		return map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": owner.ID, "rationale": "Requested objective amendment.", "item": map[string]any{"task": map[string]any{"objective": "Updated prerequisite objective."}}}}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out planningSieveMutationResult
	if err := json.Unmarshal(result.Content, &out); err != nil {
		t.Fatal(err)
	}
	expected := owner
	expected.Objective = "Updated prerequisite objective."
	if len(out.Operations) != 1 || !reflect.DeepEqual(*out.Operations[0].Item.Task, expected) {
		t.Fatalf("implicit sparse amendment lost frozen packet: %+v", out)
	}
	after, _ := json.Marshal(ledger)
	if !bytes.Equal(before, after) {
		t.Fatal("implicit amendment mutated authoritative source")
	}
}

func TestTaskPacketImplicitConstraintAdditionPreservesApprovedConstraints(t *testing.T) {
	owner := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 57, Tasks: []LedgerTask{owner}}
	before, _ := json.Marshal(ledger)
	result, err := RunParserSieve(t.Context(), Assignment{JobID: 1, Role: PlanningMutation, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningLedger: &ledger, PlanningResponse: "Add a constraint to the prerequisite: preserve original sources."}, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(call Assignment) any {
		step := call.ParserStep
		if step.Stage == "scan" {
			return scanOracle(step.Source, "change")
		}
		if len(step.Ledger.Tasks) != 1 {
			t.Fatal("implicit reference lost its candidate")
		}
		projected := step.Ledger.Tasks[0]
		if !reflect.DeepEqual(projected.Constraints, owner.Constraints) || !reflect.DeepEqual(projected.FileResponsibilities, owner.FileResponsibilities) || !reflect.DeepEqual(projected.Acceptance, owner.Acceptance) || !reflect.DeepEqual(projected.ReadFiles, owner.ReadFiles) || !reflect.DeepEqual(projected.WriteFiles, owner.WriteFiles) || !reflect.DeepEqual(projected.DeleteFiles, owner.DeleteFiles) {
			t.Fatal("implicit amendment hid existing structured obligations")
		}
		constraints := append(append([]string(nil), projected.Constraints...), "Preserve original sources.")
		return map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": projected.ID, "rationale": "Requested additional constraint.", "item": map[string]any{"task": map[string]any{"constraints": constraints}}}}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out planningSieveMutationResult
	if err := json.Unmarshal(result.Content, &out); err != nil {
		t.Fatal(err)
	}
	expected := owner
	expected.Constraints = append(append([]string(nil), owner.Constraints...), "Preserve original sources.")
	if len(out.Operations) != 1 || !reflect.DeepEqual(*out.Operations[0].Item.Task, expected) {
		t.Fatalf("implicit addition lost approved obligation: %+v", out)
	}
	after, _ := json.Marshal(ledger)
	if !bytes.Equal(before, after) {
		t.Fatal("implicit addition mutated authoritative source")
	}
}

func TestTaskPacketImplicitOwnerContextAdditionPreservesSources(t *testing.T) {
	owner := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 57, Tasks: []LedgerTask{owner}}
	additional := model.TaskContext{Text: "Keep the approved navigation.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner-18", Revision: 57}}
	result, err := RunParserSieve(t.Context(), Assignment{JobID: 1, Role: PlanningMutation, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningLedger: &ledger, PlanningResponse: "Add context to the prerequisite: keep approved navigation."}, func(NativeEvent) error { return nil }, sieveTestProvider(t, func(call Assignment) any {
		if call.ParserStep.Stage == "scan" {
			return scanOracle(call.ParserStep.Source, "change")
		}
		projected := call.ParserStep.Ledger.Tasks[0]
		if !reflect.DeepEqual(projected.OwnerContext, owner.OwnerContext) {
			t.Fatal("implicit amendment hid approved owner context or provenance")
		}
		contexts := append(append([]model.TaskContext(nil), projected.OwnerContext...), additional)
		return map[string]any{"operations": []any{map[string]any{"kind": "amend", "target_id": owner.ID, "rationale": "Additional approved context.", "item": map[string]any{"task": map[string]any{"owner_context": contexts}}}}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out planningSieveMutationResult
	if err := json.Unmarshal(result.Content, &out); err != nil {
		t.Fatal(err)
	}
	expected := owner
	expected.OwnerContext = append(append([]model.TaskContext(nil), owner.OwnerContext...), additional)
	if len(out.Operations) != 1 || !reflect.DeepEqual(*out.Operations[0].Item.Task, expected) || !reflect.DeepEqual(ledger.Tasks[0], owner) {
		t.Fatal("context addition lost original task or provenance")
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

func TestTaskPacketOpaqueReferenceRounds(t *testing.T) {
	ledger := EngineeringLedger{ID: "ledger", Revision: 57, Tasks: []LedgerTask{packetFixture()}}
	other := packetFixture()
	other.ID = "other"
	ledger.Tasks = append(ledger.Tasks, other)
	for _, test := range []struct {
		name    string
		tasks   int
		answers []string
		want    int
		invalid bool
	}{
		{"empty", 0, nil, 0, false},
		{"sole", 1, nil, 1, false},
		{"none", 2, []string{"choice_0_end"}, 0, false},
		{"multiple", 2, []string{"choice_0_1", "choice_1_0"}, 2, false},
		{"remaining may decline", 2, []string{"choice_0_0", "choice_1_end"}, 1, false},
		{"invalid", 2, []string{"unknown"}, 0, true},
		{"duplicate", 2, []string{"choice_0_0", "choice_0_0"}, 0, true},
		{"task IDs forbidden", 2, []string{"packet"}, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledger
			input.Tasks = input.Tasks[:test.tasks]
			before, _ := json.Marshal(input)
			calls := 0
			s := parserSieve{call: Assignment{Role: PlanningMutation, Route: Route{Provider: "fixture", Model: "fixture"}}, event: func(NativeEvent) error { return nil }}
			s.run = sieveTestProvider(t, func(call Assignment) any {
				if calls >= len(test.answers) {
					t.Fatal("unexpected inference")
				}
				answer := test.answers[calls]
				calls++
				if calls == 2 && len(call.ParserStep.TaskIndex) != 2 {
					t.Fatal("accepted candidate still offered")
				}
				return answer
			})
			out, err := s.resolvePlanningTargets(t.Context(), ParserStep{Source: "Amend referenced tasks."}, input)
			if (err != nil) != test.invalid {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.invalid && len(out.Tasks) != test.want {
				t.Fatalf("selected %d", len(out.Tasks))
			}
			if calls != len(test.answers) {
				t.Fatalf("calls %d", calls)
			}
			for _, task := range out.Tasks {
				if !reflect.DeepEqual(task, ledger.Tasks[0]) && !reflect.DeepEqual(task, ledger.Tasks[1]) {
					t.Fatal("selected packet changed")
				}
			}
			after, _ := json.Marshal(input)
			if !bytes.Equal(before, after) {
				t.Fatal("accepted source changed")
			}
		})
	}
}

func TestTaskPacketReferenceChoiceContractRejectsLists(t *testing.T) {
	call := Assignment{ParserStep: &ParserStep{Stage: "planning_targets", Source: "Amend a task.", TaskIndex: []planningTaskReference{{ID: "choice_0_0"}, {ID: "choice_0_end"}}}}
	prompt, err := renderParserStepPrompt(call)
	if err != nil {
		t.Fatal(err)
	}
	context, err := renderParserStepContext(call)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt+context, "target_ids") {
		t.Fatal("ID-list contract remains")
	}
	for _, raw := range []string{`["choice_0_0"]`, `{"target_ids":["packet"]}`, `"unknown"`, `null`} {
		if err := ValidateParserLeafResponse(call, json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid choice accepted: %s", raw)
		}
	}
	if err := ValidateParserLeafResponse(call, json.RawMessage(`"choice_0_0"`)); err != nil {
		t.Fatal(err)
	}
}
