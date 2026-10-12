package codingagent

import (
	"context"
	"testing"
)

func TestNarrowCallsRejectParentAndUnrelatedContext(t *testing.T) {
	controller, _ := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) { return nil, nil }, passingChecks)
	task := fixtureWorkload().Tasks[0]
	for _, role := range []Role{Execute, Review} {
		call := controller.assignment(role, &task, controller.Configuration.Executor)
		if err := call.ValidateTaskContext(); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*Assignment){
			func(c *Assignment) { c.Objective = &Objective{Instruction: "entire original request"} },
			func(c *Assignment) { c.Workload = &Workload{} },
			func(c *Assignment) { c.Requirements = []Requirement{{ID: "r2", Outcome: "other responsibility"}} },
			func(c *Assignment) { c.ReadFiles = append(c.ReadFiles, "b.txt") },
			func(c *Assignment) { c.Findings = []Finding{{TaskID: "b", Observation: "unrelated"}} },
			func(c *Assignment) { c.Checks = []CheckResult{{ID: "b-check"}} },
		} {
			invalid := call
			mutate(&invalid)
			if err := invalid.ValidateTaskContext(); err == nil {
				t.Fatal("unrelated context reached a narrow agent")
			}
		}
	}
}

func TestAnswersRemainSpecificToTheirAssignedStep(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Task != nil && len(call.QuestionContext) != 0 {
			t.Fatal("global question or sibling answer reached this task")
		}
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	for _, task := range []string{"", "other-task"} {
		question, _ := journal.Append(context.Background(), Entry{Generation: 1, Phase: "questions", TaskID: task, Questions: []string{"not this task's question"}})
		_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "answers", QuestionID: question, Answers: []string{"unrelated context"}})
	}
	controller.Objective = &Objective{Instruction: "original request withheld from decomposition and execution"}
	controller.ArtifactBindings = []ArtifactBinding{{Token: "ARTIFACT_1", Value: "NaN/infinity"}}
	controller.PlanningEnvironment = &PlanningEnvironment{NewFilesAllowed: true, Executables: []string{"python3"}}
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, entry := range journal.entries {
		if entry.Phase == "call_opened" && entry.Role == Plan && entry.Assignment.Objective != nil {
			t.Fatal("task decomposition reinterpreted the original request")
		}
		if entry.Phase == "call_opened" && entry.Role == Plan && (len(entry.Assignment.ArtifactBindings) != 1 || entry.Assignment.ArtifactBindings[0].Value != "NaN/infinity" || entry.Assignment.PlanningEnvironment == nil) {
			t.Fatal("finalized planner context lost its actual artifact binding or capabilities")
		}
	}
}
