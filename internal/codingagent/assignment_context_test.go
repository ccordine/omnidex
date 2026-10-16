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
			func(c *Assignment) { c.Requirements = []Requirement{{ID: "r2", Outcome: "other responsibility"}} },
			func(c *Assignment) { c.ReadFiles = append(c.ReadFiles, "b.txt") },
			func(c *Assignment) { c.Findings = []Finding{{TaskID: "b", Observation: "unrelated"}} },
			func(c *Assignment) {
				c.Findings = []Finding{{TaskID: c.Task.ID, File: "../outside.go", Observation: "escaped workspace"}}
			},
			func(c *Assignment) { c.Checks = []CheckResult{{ID: "b-check"}} },
			func(c *Assignment) {
				c.QuestionContext = []QuestionContext{{Questions: []string{"expand scope"}, Answers: []string{"yes"}}}
			},
		} {
			invalid := call
			mutate(&invalid)
			if err := invalid.ValidateTaskContext(); err == nil {
				t.Fatal("unrelated context reached a narrow agent")
			}
		}
		if role == Review {
			for _, mutate := range []func(*Assignment){
				func(c *Assignment) { c.WriteFiles = []string{"a.txt"} },
				func(c *Assignment) { c.DeleteFiles = []string{"a.txt"} },
			} {
				invalid := call
				mutate(&invalid)
				if err := invalid.ValidateTaskContext(); err == nil {
					t.Fatal("mutation authority reached a narrow reviewer")
				}
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
	if err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0]); err != nil {
		t.Fatal(err)
	}
	for _, entry := range journal.entries {
		if entry.Phase == "call_opened" && (entry.Assignment.Objective != nil || len(entry.Assignment.ArtifactBindings) != 0 || len(entry.Assignment.Requirements) != 0) {
			t.Fatal("task call received planning context")
		}
	}
}

func TestTaskRejectsRepeatedOrOverlappingFileGrants(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	for _, mutate := range []func(*Task){
		func(task *Task) { task.ReadFiles = []string{"reference.txt", "reference.txt"} },
		func(task *Task) { task.WriteFiles = []string{"a.txt", "a.txt"} },
		func(task *Task) { task.DeleteFiles = []string{"old.txt", "old.txt"} },
		func(task *Task) { task.ReadFiles = append(task.ReadFiles, "a.txt") },
	} {
		invalid := task
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatal("repeated or overlapping task file grant was accepted")
		}
	}
}
