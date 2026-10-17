package codingagent

import (
	"encoding/json"
	"strings"
	"testing"
)

func taskProjection(step *ParserStep) map[string]any {
	id := step.NewIDs
	item := func(kind string, values map[string]any) any {
		values["id"], values["priority"] = id[kind], "normal"
		return map[string]any{"kind": "add", "item": map[string]any{kind: values}, "rationale": "Represent the one admitted statement."}
	}
	return map[string]any{"operations": []any{
		item("requirement", map[string]any{"outcome": "lossy replacement"}),
		item("task", map[string]any{"goal": "lossy replacement", "group_id": "", "requirement_ids": []string{id["requirement"]}, "constraint_ids": []string{}, "acceptance_criterion_ids": []string{id["acceptance_criterion"]}, "delivery_obligation": "", "known_affected_systems": []string{}, "read_files": []string{}, "write_files": []string{}, "delete_files": []string{}}),
		item("acceptance_criterion", map[string]any{"outcome": "lossy replacement", "requirement_ids": []string{id["requirement"]}, "task_ids": []string{id["task"]}}),
		item("check", map[string]any{"argv": []string{}, "observation": "lossy replacement", "requirement_ids": []string{id["requirement"]}, "acceptance_criterion_ids": []string{id["acceptance_criterion"]}}),
	}}
}

func TestPlanningSieveMaterializesEveryAdmittedStatementAndExcludesIgnoredText(t *testing.T) {
	for _, constraintFirst := range []bool{false, true} {
		task, rule, optional := "Create the archive and verify restored bytes.\n\n", "Keep archives local; do not upload them.\n\n", "An optional cloud dashboard is not proposed."
		source := task + rule + optional
		if constraintFirst {
			source = rule + task + optional
		}
		call := Assignment{JobID: 1, Role: PlanningInitial, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningResponse: source}
		projections := 0
		provider := sieveTestProvider(t, func(leaf Assignment) any {
			step := leaf.ParserStep
			if step.Stage == "scan" {
				kind := "ignore"
				if step.Source == task {
					kind = "task"
				}
				if step.Source == rule {
					kind = "project_constraint"
				}
				return scanOracle(step.Source, kind)
			}
			projections++
			if step.Category == "task" {
				return taskProjection(step)
			}
			return map[string]any{"operations": []any{map[string]any{"kind": "add", "rationale": "source policy", "item": map[string]any{"constraint": map[string]any{"id": step.NewIDs["constraint"], "priority": "normal", "rule": "lossy replacement"}}}}}
		})
		result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, provider)
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct{ Ledger EngineeringLedger }
		if err := json.Unmarshal(result.Content, &parsed); err != nil {
			t.Fatal(err)
		}
		ledger := parsed.Ledger
		if err := ledger.Validate(); err != nil {
			t.Fatal(err)
		}
		if len(ledger.Tasks) != 1 || len(ledger.Requirements) != 1 || len(ledger.AcceptanceCriteria) != 1 || len(ledger.Checks) != 1 || len(ledger.Constraints) != 1 || projections != 2 {
			t.Fatalf("wrong ledger effects: %s", result.Content)
		}
		if ledger.Tasks[0].Goal != strings.TrimSpace(task) || ledger.Requirements[0].Outcome != strings.TrimSpace(task) || ledger.AcceptanceCriteria[0].Outcome != strings.TrimSpace(task) || ledger.Checks[0].Observation != strings.TrimSpace(task) {
			t.Fatal("code allowed the projection to erase an admitted obligation")
		}
		if ledger.Constraints[0].Rule != strings.TrimSpace(rule) || len(ledger.Tasks[0].ConstraintIDs) != 1 || ledger.Tasks[0].ConstraintIDs[0] != ledger.Constraints[0].ID {
			t.Fatal("global policy lost its consumer")
		}
		if strings.Contains(string(result.Content), "cloud dashboard") || strings.Contains(string(result.Content), "lossy replacement") {
			t.Fatal("ignored or invented content entered ledger")
		}
	}
}

func TestPlanningSieveRejectsUnsupportedFieldsCommandsAndIdentities(t *testing.T) {
	step := ParserStep{Stage: "planning_unit", BlockID: "block", Source: "Create a local archive.", Category: "task", NewIDs: map[string]string{"task": "new-task", "requirement": "new-req", "acceptance_criterion": "new-ac", "check": "new-check"}}
	ledger := EngineeringLedger{ID: "draft", Revision: 1}
	for _, mutate := range []func(map[string]any){
		func(p map[string]any) {
			p["operations"].([]any)[0].(map[string]any)["item"].(map[string]any)["requirement"].(map[string]any)["rank"] = 77
		},
		func(p map[string]any) {
			p["operations"].([]any)[1].(map[string]any)["item"].(map[string]any)["task"].(map[string]any)["id"] = "invented"
		},
		func(p map[string]any) {
			p["operations"].([]any)[1].(map[string]any)["item"].(map[string]any)["task"].(map[string]any)["write_files"] = []string{"invented.go"}
		},
		func(p map[string]any) {
			p["operations"].([]any)[3].(map[string]any)["item"].(map[string]any)["check"].(map[string]any)["argv"] = []string{"invented-command"}
		},
	} {
		projection := taskProjection(&step)
		mutate(projection)
		raw, _ := json.Marshal(projection)
		if _, err := normalizeParserPlanningItems(raw, ParserPart{Text: step.Source, Kind: "task", Reason: "oracle"}, step, ledger); err == nil {
			t.Fatalf("unsupported semantic projection accepted: %s", raw)
		}
	}
}
