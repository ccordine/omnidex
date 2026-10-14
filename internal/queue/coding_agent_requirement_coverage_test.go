package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentParentContextKeepsChildAcceptanceNarrow(t *testing.T) {
	parent := codingagent.Requirement{ID: "objective", Outcome: "The application provides its module and command-line interface."}
	objective := &codingagent.Objective{Instruction: parent.Outcome}
	taskA := codingagent.Task{ID: "module", Goal: "Implement the module", Acceptance: []string{"The module returns the expected value."}, RequirementIDs: []string{parent.ID}, WriteFiles: []string{"module.go"}, Checks: []codingagent.Check{{ID: "module-check", Argv: []string{"go", "test"}}}}
	taskB := codingagent.Task{ID: "cli", Goal: "Implement the CLI", Acceptance: []string{"The CLI prints the module value."}, RequirementIDs: []string{parent.ID}, ReadFiles: []string{"module.go"}, WriteFiles: []string{"cli.go"}, Dependencies: []string{taskA.ID}, Checks: []codingagent.Check{{ID: "cli-check", Argv: []string{"go", "test"}}}}
	workload := codingagent.Workload{Tasks: []codingagent.Task{taskA, taskB}, Checks: []codingagent.Check{{ID: "overall", Argv: []string{"go", "test"}}}}
	statementA, _ := codingagent.PlanTaskStatement(taskA)
	statementB, _ := codingagent.PlanTaskStatement(taskB)
	plan := model.CodingPlan{Leaves: []model.CodingPlanLeaf{
		{ID: "coding_plan_leaf_00000000000000000000000000000001", Statement: statementA, Decision: model.CodingPlanDecisionApproved},
		{ID: "coding_plan_leaf_00000000000000000000000000000002", Statement: statementB, Decision: model.CodingPlanDecisionApproved},
	}}
	entries := []codingagent.Entry{
		{ID: 1, Phase: "call_opened", Assignment: &codingagent.Assignment{Role: codingagent.Plan, Requirements: []codingagent.Requirement{parent}, Objective: objective}},
		{ID: 2, Phase: "plan_frozen", CallID: 1, Workload: &workload},
		{ID: 3, Phase: "call_opened", Assignment: &codingagent.Assignment{Role: codingagent.Review, Task: &taskA, ReadFiles: []string{"module.go"}}},
		{ID: 4, Phase: "task_accepted", TaskID: taskA.ID, CallID: 3},
		{ID: 5, Phase: "call_opened", Assignment: &codingagent.Assignment{Role: codingagent.Review, Task: &taskB, ReadFiles: []string{"cli.go", "module.go"}}},
		{ID: 6, Phase: "task_accepted", TaskID: taskB.ID, CallID: 5},
		{ID: 7, Phase: "call_opened", Assignment: &codingagent.Assignment{Role: codingagent.OverallReview, Workload: &workload, Requirements: []codingagent.Requirement{parent}, Objective: objective}},
		{ID: 8, Phase: "overall_accepted", CallID: 7},
	}
	if err := validateCodingAgentRequirementCoverage(entries, plan); err != nil {
		t.Fatalf("narrow tasks were forced to fulfill the whole parent: %v", err)
	}
	entries[6].Assignment.Objective = &codingagent.Objective{Instruction: "A changed objective"}
	if err := validateCodingAgentRequirementCoverage(entries, plan); err == nil {
		t.Fatal("overall review changed the user-approved objective")
	}
	entries[6].Assignment.Objective = objective
	entries[2].Assignment.Requirements = []codingagent.Requirement{{ID: parent.ID, Outcome: "An invented parent outcome."}}
	if err := validateCodingAgentRequirementCoverage(entries, plan); err == nil {
		t.Fatal("task reviewer received unassigned parent context")
	}
	entries[2].Assignment.Requirements = nil
	changedTask := taskA
	changedTask.Acceptance = []string{parent.Outcome}
	entries[2].Assignment.Task = &changedTask
	if err := validateCodingAgentRequirementCoverage(entries, plan); err == nil {
		t.Fatal("task reviewer expanded the frozen narrow task")
	}
	entries[2].Assignment.Task = &taskA
	cases := []struct {
		name   string
		mutate func(*codingagent.Assignment)
	}{
		{"artifact bindings", func(call *codingagent.Assignment) {
			call.ArtifactBindings = []codingagent.ArtifactBinding{{Token: "ARTIFACT_1", Value: "unassigned owner context"}}
		}},
		{"planning environment", func(call *codingagent.Assignment) {
			call.PlanningEnvironment = &codingagent.PlanningEnvironment{NewFilesAllowed: true}
		}},
		{"construction facts", func(call *codingagent.Assignment) {
			call.ConstructionFacts = []codingagent.ConstructionFact{{Kind: "documentation", Quote: "Unassigned parent constraint"}}
		}},
		{"missing assigned files", func(call *codingagent.Assignment) {
			call.ReadFiles = nil
		}},
		{"unrelated file", func(call *codingagent.Assignment) {
			call.ReadFiles = []string{"unrelated.go"}
		}},
		{"unrelated check", func(call *codingagent.Assignment) {
			call.Checks = []codingagent.CheckResult{{ID: "cli-check", ExitCode: 0}}
		}},
		{"another task finding", func(call *codingagent.Assignment) {
			call.Findings = []codingagent.Finding{{TaskID: taskB.ID, File: "module.go"}}
		}},
		{"unrelated file finding", func(call *codingagent.Assignment) {
			call.Findings = []codingagent.Finding{{TaskID: taskA.ID, File: "unrelated.go"}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			call := *entries[2].Assignment
			test.mutate(&call)
			altered := append([]codingagent.Entry{}, entries...)
			altered[2].Assignment = &call
			if err := validateCodingAgentRequirementCoverage(altered, plan); err == nil {
				t.Fatal("completion accepted context outside the frozen task")
			}
		})
	}
}
