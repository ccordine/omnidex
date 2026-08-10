package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresCancelJobRetiresActiveCognitionBeforeAttempt(t *testing.T) {
	fixture := startLifecycleRetirementFixture(t, "lifecycle-cancel-active")
	command := testCancelCommand(
		t, fixture.Job.ID, "lifecycle-cancel-active", "Cancel the owning job exactly.",
	)
	canceled, err := fixture.Repository.CancelJob(fixture.Context, command)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != model.JobStatusCanceled {
		t.Fatalf("canceled job=%+v", canceled)
	}
	assertLifecycleCognitionRetired(t, fixture, command.OperationID, LifecycleCancelJob)
	before := lifecycleCognitionCounts(t, fixture, command.OperationID)
	replayed, err := fixture.Repository.CancelJob(fixture.Context, command)
	if err != nil || !reflect.DeepEqual(replayed, canceled) {
		t.Fatalf("cancellation replay=%+v error=%v", replayed, err)
	}
	if after := lifecycleCognitionCounts(t, fixture, command.OperationID); after != before {
		t.Fatalf("replay counts=%v want %v", after, before)
	}
}

func TestPostgresLifecycleRetirementRejectsUnresolvedStateWithoutPartialMutation(t *testing.T) {
	states := []struct {
		name  string
		label string
		seed  func(*testing.T, taskGenerationRetirementFixture)
	}{
		{name: "prepared action", label: "prepared-action", seed: func(t *testing.T, fixture taskGenerationRetirementFixture) {
			_ = prepareCognitionGuardAction(t, fixture, "retirement-unresolved-action")
		}},
		{name: "started policy call", label: "started-policy-call", seed: func(t *testing.T, fixture taskGenerationRetirementFixture) {
			_ = reserveIndeterminateCognitionCall(t, fixture)
		}},
	}
	operations := []struct {
		name string
		run  func(*testing.T, taskGenerationRetirementFixture, string) (LifecycleOperationID, error)
	}{
		{name: "cancel", run: func(t *testing.T, fixture taskGenerationRetirementFixture, label string) (LifecycleOperationID, error) {
			command := testCancelCommand(t, fixture.Job.ID, label, "Reject ambiguous cognition state.")
			_, err := fixture.Repository.CancelJob(fixture.Context, command)
			return command.OperationID, err
		}},
		{name: "replan", run: func(t *testing.T, fixture taskGenerationRetirementFixture, label string) (LifecycleOperationID, error) {
			command := testReplanCommand(t, fixture.Job.ID, label, "Reject ambiguous cognition state.")
			_, err := fixture.Repository.ReplanJob(fixture.Context, command)
			return command.OperationID, err
		}},
	}
	for _, operation := range operations {
		for _, state := range states {
			t.Run(operation.name+"/"+state.name, func(t *testing.T) {
				label := "retirement-rollback-" + operation.name + "-" + state.label
				fixture := startLifecycleRetirementFixture(t, label)
				state.seed(t, fixture)
				operationID, err := operation.run(t, fixture, label)
				if !errors.Is(err, ErrCognitionConflict) {
					t.Fatalf("%s error=%v, want cognition conflict", operation.name, err)
				}
				assertLifecycleCognitionRollback(t, fixture, operationID)
			})
		}
	}
}

func TestPostgresCanceledEpisodeRecoversUnderExactReplacementAttempt(t *testing.T) {
	fixture := startLifecycleRetirementFixture(t, "canceled-replacement-recovery")
	command := cognitionCancellationForTest(t, fixture, errors.New("durable response was lost"))
	seal, err := fixture.Repository.CancelCognitionEpisode(fixture.Context, command)
	if err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}
	progress, err := fixture.Repository.CognitionRuntimeTerminalProgress(fixture.Context, binding)
	if err != nil {
		t.Fatal(err)
	}
	if progress == nil || progress.State != cognitionruntime.ProgressCanceled ||
		progress.Cancellation == nil || progress.Cancellation.TraceSHA256 != seal.TraceSHA256 ||
		progress.Cancellation.Code != command.Code || progress.Completion == nil ||
		progress.Completion.Outcome != cognition.CompletionUnsatisfied {
		t.Fatalf("recovered canceled progress=%+v", progress)
	}
	again, err := fixture.Repository.CognitionRuntimeTerminalProgress(fixture.Context, binding)
	if err != nil || !reflect.DeepEqual(again, progress) {
		t.Fatalf("canceled progress replay=%+v error=%v", again, err)
	}
	var snapshots, calls, actions int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&snapshots, &calls, &actions); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 || calls != 0 || actions != 0 {
		t.Fatalf("canceled recovery created snapshot/call/action=%d/%d/%d", snapshots, calls, actions)
	}
}

func TestPostgresLifecycleRetirementRejectsTerminalEnvironmentTruth(t *testing.T) {
	repository, pool, ctx := lifecycleRetirementTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "terminal-truth")
	fixture.Start.Transition.Terminal = true
	fixture.Start.Transition.PublicOutcome = "The environment already completed."
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	command := testCancelCommand(t, fixture.Job.ID, "terminal-truth", "Do not relabel terminal truth.")
	if _, err := repository.CancelJob(ctx, command); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("terminal environment cancellation error=%v", err)
	}
	assertLifecycleCognitionRollback(t, fixture, command.OperationID)
}

func TestPostgresLifecycleRetirementSealsExplicitEmptyEpisodeSet(t *testing.T) {
	repository, pool, ctx := lifecycleRetirementTestRepository(t)
	job, err := repository.EnqueueJob(ctx, "lifecycle-empty-seal-set", model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	command := testCancelCommand(t, job.ID, "lifecycle-empty-set", "Cancel without cognition episodes.")
	if _, err := repository.CancelJob(ctx, command); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, found, err := loadCognitionLifecycleSealSetTx(ctx, tx, command.OperationID)
	_ = tx.Rollback(ctx)
	if err != nil || !found || set.Entries == nil || len(set.Entries) != 0 {
		t.Fatalf("empty lifecycle seal set=%+v found=%t error=%v", set, found, err)
	}
	if _, err := repository.CancelJob(ctx, command); err != nil {
		t.Fatalf("empty lifecycle seal-set replay: %v", err)
	}
}

func TestPostgresLifecycleRetirementRejectsEnvironmentAheadOfQueue(t *testing.T) {
	for _, kind := range []LifecycleOperationKind{LifecycleCancelJob, LifecycleReplanJob} {
		t.Run(string(kind), func(t *testing.T) {
			label := "environment-ahead-" + string(kind)
			fixture := startLifecycleRetirementFixture(t, label)
			episode := cognition.EpisodeRef{ID: fixture.EpisodeID}
			if _, err := fixture.Repository.StartCognitionEnvironment(
				fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
			); err != nil {
				t.Fatal(err)
			}
			action := prepareCognitionGuardAction(t, fixture, label)
			if _, err := fixture.Repository.DispatchCognitionAction(
				fixture.Context, fixture.Authority, action.Action.ID,
			); err != nil {
				t.Fatal(err)
			}
			receipt := environmentTransitionReceipt(t, episode, action, false)
			if _, err := fixture.Repository.CommitCognitionEnvironmentAction(
				fixture.Context, episode, fixture.Start.Scenario, receipt,
			); err != nil {
				t.Fatal(err)
			}
			operationID, err := rejectDriftWithLifecycleOperation(t, fixture, label, kind)
			if !errors.Is(err, ErrCognitionConflict) {
				t.Fatalf("environment drift %s error=%v", kind, err)
			}
			assertLifecycleCognitionRollback(t, fixture, operationID)
			state, err := fixture.Repository.CognitionEnvironmentState(
				fixture.Context, episode, fixture.Start.Scenario,
			)
			if err != nil || state.Current != receipt.Transition.Current {
				t.Fatalf("failed lifecycle changed environment state=%+v error=%v", state, err)
			}
		})
	}
}

func rejectDriftWithLifecycleOperation(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	label string,
	kind LifecycleOperationKind,
) (LifecycleOperationID, error) {
	t.Helper()
	if kind == LifecycleCancelJob {
		command := testCancelCommand(t, fixture.Job.ID, label, "Reject environment drift.")
		_, err := fixture.Repository.CancelJob(fixture.Context, command)
		return command.OperationID, err
	}
	command := testReplanCommand(t, fixture.Job.ID, label, "Reject environment drift.")
	_, err := fixture.Repository.ReplanJob(fixture.Context, command)
	return command.OperationID, err
}
