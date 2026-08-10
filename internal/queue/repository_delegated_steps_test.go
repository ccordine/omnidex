package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func TestDelegatedExpansionRejectsInvalidAuthorityBeforePersistence(t *testing.T) {
	repository := &Repository{}
	valid := delegatedSubtaskFixture("task-1")
	cases := []struct {
		name      string
		authority model.StepAttemptAuthority
		subtasks  []artifacts.Subtask
		wantError string
	}{
		{name: "missing job", authority: model.StepAttemptAuthority{StepID: 2}, subtasks: []artifacts.Subtask{valid}, wantError: "positive job"},
		{name: "missing anchor", authority: model.StepAttemptAuthority{JobID: 1}, subtasks: []artifacts.Subtask{valid}, wantError: "anchor step"},
		{name: "empty batch", authority: model.StepAttemptAuthority{JobID: 1, StepID: 2}, wantError: "at least one"},
		{name: "duplicate identity", authority: model.StepAttemptAuthority{JobID: 1, StepID: 2}, subtasks: []artifacts.Subtask{valid, valid}, wantError: "duplicated"},
		{name: "invalid kind", authority: model.StepAttemptAuthority{JobID: 1, StepID: 2}, subtasks: []artifacts.Subtask{{
			ID: "task-2", Kind: artifacts.SubtaskKindVerify, RoleID: "subtask_executor",
			ObjectiveID: "objective-1", Objective: "Inspect one bounded surface.", Priority: 50,
			SuccessCriteria: []string{"Evidence is recorded."},
		}}, wantError: "invalid kind"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.ExpandDelegatedSubtasks(
				context.Background(), test.authority, test.subtasks,
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
