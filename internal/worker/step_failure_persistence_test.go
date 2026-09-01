package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestRunClaimReturnsTerminalFailurePersistenceError(t *testing.T) {
	claim, access := invalidActionClaim(t)
	persistenceErr := errors.New("postgres commit unavailable")
	service := &Service{
		repo:                &queue.Repository{},
		hostDirectoryAccess: access,
		failStep: func(context.Context, queue.FailStepCommand) error {
			return persistenceErr
		},
	}

	err := service.runClaim(context.Background(), "fixture-worker", claim)
	if !errors.Is(err, persistenceErr) || !strings.Contains(err.Error(), "commit failed step lifecycle operation") {
		t.Fatalf("runClaim error = %v, want terminal persistence failure", err)
	}
}

func TestRunClaimDoesNotFailAfterDurableFailureCommit(t *testing.T) {
	claim, access := invalidActionClaim(t)
	var committed queue.FailStepCommand
	service := &Service{
		repo:                &queue.Repository{},
		hostDirectoryAccess: access,
		failStep: func(_ context.Context, command queue.FailStepCommand) error {
			committed = command
			return nil
		},
	}

	if err := service.runClaim(context.Background(), "fixture-worker", claim); err != nil {
		t.Fatalf("runClaim returned an error after durable failure commit: %v", err)
	}
	if committed.StepID != claim.Step.ID || committed.Authority != claim.Authority ||
		!strings.Contains(committed.Error, `worker action "unregistered_fixture_action" is not registered`) {
		t.Fatalf("committed failure command = %#v", committed)
	}
}

func TestWorkerLoopStopsAfterTerminalFailurePersistenceError(t *testing.T) {
	persistenceErr := errors.New("terminal failure receipt was not committed")
	claim := &model.ClaimedStep{
		Job:  model.Job{ID: 19},
		Step: model.Step{ID: 23},
	}
	claimCalls := 0
	executionCalls := 0
	service := &Service{}

	err := service.runLoop(
		context.Background(),
		"fixture-worker",
		time.Millisecond,
		func(context.Context, string) (*model.ClaimedStep, error) {
			claimCalls++
			return claim, nil
		},
		func(context.Context, string, *model.ClaimedStep) error {
			executionCalls++
			return persistenceErr
		},
	)
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("worker loop error = %v, want terminal persistence failure", err)
	}
	if claimCalls != 1 || executionCalls != 1 {
		t.Fatalf("worker loop retried after terminal persistence failure: claims=%d executions=%d", claimCalls, executionCalls)
	}
}

func invalidActionClaim(t *testing.T) (*model.ClaimedStep, workspacefacts.HostDirectoryAccess) {
	t.Helper()
	root := t.TempDir()
	access, err := workspacefacts.NewHostDirectoryAccess("/tmp")
	if err != nil {
		t.Fatalf("construct test host directory authority: %v", err)
	}
	metadata, err := json.Marshal(map[string]string{"client_cwd": root})
	if err != nil {
		t.Fatalf("marshal job metadata: %v", err)
	}
	authority := model.StepAttemptAuthority{
		JobID: 41, Generation: 1, StepID: 73, Attempt: 1, WorkerID: "fixture-worker",
	}
	return &model.ClaimedStep{
		Job: model.Job{
			ID: 41, Status: model.JobStatusRunning, CurrentGeneration: 1, Metadata: metadata,
		},
		Step: model.Step{
			ID: 73, JobID: 41, Generation: 1, Status: model.StepStatusRunning,
			Action: "unregistered_fixture_action", WorkerID: "fixture-worker",
		},
		Authority: authority, LeaseDeadline: time.Now().Add(time.Minute),
	}, access
}
