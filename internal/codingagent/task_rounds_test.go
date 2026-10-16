package codingagent

import (
	"context"
	"testing"
)

func TestTaskContinuesBeyondThreeExecutionReviewRounds(t *testing.T) {
	executions, reviews := 0, 0
	workload := fixtureWorkload()
	task := workload.Tasks[0]
	controller, _ := fixtureController(t, workload, func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions++
			return "Completed the requested task changes.", nil
		}
		reviews++
		result := cleanReview()
		if reviews <= 4 {
			result.Findings = []Finding{{TaskID: task.ID, Requirement: task.Acceptance[0], Observation: "The task-local acceptance behavior still needs correction."}}
		}
		return result, nil
	}, passingChecks)
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if executions != 5 || reviews != 5 {
		t.Fatalf("task stopped early: executions=%d reviews=%d", executions, reviews)
	}
}
