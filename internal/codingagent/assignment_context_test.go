package codingagent

import (
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
			func(c *Assignment) { c.Checks = []CheckResult{{ID: "b-check"}} },
			func(c *Assignment) { c.Continuation = &TaskContinuation{Task: task} },
			func(c *Assignment) { c.CompletedTaskIDs = []string{"sibling"} },
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

func TestTaskRejectsRepeatedFileReferences(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	for _, mutate := range []func(*Task){
		func(task *Task) { task.ReadFiles = []string{"reference.txt", "reference.txt"} },
		func(task *Task) { task.WriteFiles = []string{"a.txt", "a.txt"} },
		func(task *Task) { task.DeleteFiles = []string{"old.txt", "old.txt"} },
	} {
		invalid := task
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatal("repeated task file reference was accepted")
		}
	}
}

func TestTaskAllowsFileReferencesAcrossOperations(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	task.ReadFiles = []string{"a.txt"}
	task.DeleteFiles = []string{"a.txt"}
	if err := task.Validate(); err != nil {
		t.Fatalf("normal read/edit/replace context was rejected: %v", err)
	}
	controller, _ := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) { return nil, nil }, passingChecks)
	for _, role := range []Role{Execute, Review} {
		call := controller.assignment(role, &task, controller.Configuration.Executor)
		if err := call.ValidateTaskContext(); err != nil {
			t.Fatalf("overlapping context failed %s dispatch: %v", role, err)
		}
	}
}

func TestResponseParsersReceiveNoExecutionOrParentContext(t *testing.T) {
	for _, role := range []Role{ExecutionResponse, ReviewResponse} {
		call := Assignment{Role: role, Response: "Ordinary final response."}
		if role == ReviewResponse {
			task := fixtureWorkload().Tasks[0]
			call.Task = &task
		}
		if err := call.ValidateTaskContext(); err != nil {
			t.Fatal(err)
		}
		for _, change := range []func(*Assignment){
			func(call *Assignment) { call.Objective = &Objective{Instruction: "parent objective"} },
			func(call *Assignment) { call.Clarifications = []TaskClarification{{Answer: "unrelated answer"}} },
			func(call *Assignment) { call.ReadFiles = []string{"unrelated-file"} },
			func(call *Assignment) { call.PlanningLedger = &EngineeringLedger{} },
		} {
			invalid := call
			change(&invalid)
			if err := invalid.ValidateTaskContext(); err == nil {
				t.Fatalf("%s accepted unrelated context", role)
			}
		}
	}
}
