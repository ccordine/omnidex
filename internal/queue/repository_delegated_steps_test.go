package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
)

func TestDelegatedExpansionRejectsInvalidAuthorityBeforePersistence(t *testing.T) {
	repository := &Repository{}
	valid := delegatedSubtaskFixture("task-1")
	cases := []struct {
		name      string
		jobID     int64
		stepID    int64
		subtasks  []artifacts.Subtask
		wantError string
	}{
		{name: "missing job", stepID: 2, subtasks: []artifacts.Subtask{valid}, wantError: "positive job"},
		{name: "missing anchor", jobID: 1, subtasks: []artifacts.Subtask{valid}, wantError: "anchor step"},
		{name: "empty batch", jobID: 1, stepID: 2, wantError: "at least one"},
		{name: "duplicate identity", jobID: 1, stepID: 2, subtasks: []artifacts.Subtask{valid, valid}, wantError: "duplicated"},
		{name: "invalid kind", jobID: 1, stepID: 2, subtasks: []artifacts.Subtask{{
			ID: "task-2", Kind: artifacts.SubtaskKindVerify, RoleID: "subtask_executor",
			ObjectiveID: "objective-1", Objective: "Inspect one bounded surface.", Priority: 50,
			SuccessCriteria: []string{"Evidence is recorded."},
		}}, wantError: "invalid kind"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.ExpandDelegatedSubtasks(
				context.Background(), test.jobID, test.stepID, test.subtasks,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error=%v want substring %q", err, test.wantError)
			}
		})
	}
}

func TestDelegatedExpansionSourceLocksJobBeforeStep(t *testing.T) {
	raw, err := os.ReadFile("repository_delegated_steps.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	jobLock := strings.Index(source, "lockedJobTx(ctx, tx, jobID)")
	stepLock := strings.Index(source, "WHERE id = $1 AND job_id = $2")
	if jobLock < 0 || stepLock < 0 || jobLock >= stepLock {
		t.Fatalf("delegated expansion lock order job=%d step=%d", jobLock, stepLock)
	}
	for _, required := range []string{
		"supersededAt != nil",
		"anchor.generation != job.CurrentGeneration",
		"ErrStaleJobGeneration",
		"status != model.StepStatusRunning",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("delegated expansion omitted authority check %q", required)
		}
	}
}

func delegatedSubtaskFixture(id string) artifacts.Subtask {
	return artifacts.Subtask{
		ID: id, Kind: artifacts.SubtaskKindAnalyze, RoleID: "subtask_executor",
		ObjectiveID: "objective-1", Objective: "Inspect one bounded surface.", Priority: 50,
		SuccessCriteria: []string{"Evidence is recorded."},
	}
}
