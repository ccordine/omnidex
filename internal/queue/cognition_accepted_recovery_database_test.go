package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresAcceptedDecisionRecoveryContinuesWithoutAnotherPolicyCall(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "accepted-decision-recovery")
	prepared, step := reserveAcceptedDecisionWithoutAction(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}

	recovery, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, binding)
	if err != nil {
		t.Fatal(err)
	}
	if recovery == nil || recovery.SourceActor != prepared.Prepared.Snapshot.Attempt() ||
		recovery.Binding != binding || recovery.Prepared.Snapshot.SHA256() != prepared.Prepared.Snapshot.SHA256() ||
		recovery.ExistingReconciliation != nil {
		t.Fatalf("accepted recovery=%+v", recovery)
	}
	assertAcceptedRecoveryCounts(t, fixture, 1, 1, 0)

	reconcile := cognitionruntime.ReconciliationCommand{
		Binding: binding, SnapshotSHA256: recovery.Prepared.Snapshot.SHA256(),
		Projection: recovery.Prepared.Snapshot.ContextProjection(), ActionSchema: recovery.ActionSchema,
		Decision: recovery.Decision.Clone(), Recovery: recoveryRefForTest(recovery.Ref()),
	}
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(fixture.Context, reconcile)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Recovery == nil || *receipt.Recovery != recovery.Ref() {
		t.Fatalf("reconciliation recovery=%+v", receipt.Recovery)
	}
	step.Actor = binding.Attempt
	step.Decision = cognitionDecisionPointer(recovery.Decision)
	step.ActionSchema = recovery.ActionSchema.Ref()
	step.ContextProjection = recovery.Prepared.Snapshot.ContextProjection()
	step.SnapshotSHA256 = recovery.Prepared.Snapshot.SHA256()
	action, err := fixture.Repository.PrepareCognitionAction(fixture.Context, cognitionruntime.PrepareActionCommand{
		Binding: binding, Coordinator: step, Reconciliation: receipt,
		Recovery: recoveryRefForTest(recovery.Ref()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if action.Origin != replacement || action.PolicyCallID != recovery.SourcePolicyCallID ||
		action.Action.Actor != binding.Attempt {
		t.Fatalf("recovered action=%+v", action)
	}
	assertAcceptedRecoveryCounts(t, fixture, 1, 1, 1)
	if _, err := fixture.Repository.RecoverAcceptedCognitionDecision(
		fixture.Context,
		cognitionruntime.Binding{
			Episode: binding.Episode, Attempt: cognitionAttempt(fixture.Authority),
		},
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale source recovery error=%v", err)
	}
}

func TestPostgresAcceptedDecisionRecoveryReplaysSameAttemptBeforeReconciliation(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "accepted-same-attempt-recovery")
	prepared, step := reserveAcceptedDecisionWithoutAction(t, fixture)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}

	recovery, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, binding)
	if err != nil {
		t.Fatal(err)
	}
	if recovery == nil || recovery.Binding != binding || recovery.SourceActor != binding.Attempt ||
		recovery.Prepared.Snapshot.SHA256() != prepared.Prepared.Snapshot.SHA256() ||
		recovery.ExistingReconciliation != nil {
		t.Fatalf("same-attempt accepted recovery=%+v", recovery)
	}
	assertAcceptedRecoveryCounts(t, fixture, 1, 1, 0)

	replayed, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, binding)
	if err != nil || replayed == nil || replayed.Ref() != recovery.Ref() {
		t.Fatalf("same-attempt exact replay=%+v error=%v", replayed, err)
	}
	command := cognitionruntime.ReconciliationCommand{
		Binding: binding, SnapshotSHA256: recovery.Prepared.Snapshot.SHA256(),
		Projection: recovery.Prepared.Snapshot.ContextProjection(), ActionSchema: recovery.ActionSchema,
		Decision: recovery.Decision.Clone(), Recovery: recoveryRefForTest(recovery.Ref()),
	}
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(fixture.Context, command)
	if err != nil {
		t.Fatal(err)
	}
	step.Decision = cognitionDecisionPointer(recovery.Decision)
	step.ActionSchema = recovery.ActionSchema.Ref()
	step.ContextProjection = recovery.Prepared.Snapshot.ContextProjection()
	step.SnapshotSHA256 = recovery.Prepared.Snapshot.SHA256()
	action, err := fixture.Repository.PrepareCognitionAction(fixture.Context, cognitionruntime.PrepareActionCommand{
		Binding: binding, Coordinator: step, Reconciliation: receipt,
		Recovery: recoveryRefForTest(recovery.Ref()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if action.PolicyCallID != recovery.SourcePolicyCallID || action.Action.Actor != binding.Attempt {
		t.Fatalf("same-attempt recovered action=%+v", action)
	}
	assertAcceptedRecoveryCounts(t, fixture, 1, 1, 1)
	if _, err := fixture.Repository.DispatchCognitionAction(
		fixture.Context, fixture.Authority, action.Action.ID,
	); err != nil {
		t.Fatal(err)
	}
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("e"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Repository.IngestCognitionTransition(
		fixture.Context, fixture.Authority, action.Action.ID, cognition.Transition{
			ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
			Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
		}, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if afterAction, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, binding); err != nil || afterAction != nil {
		t.Fatalf("completed accepted call remained recoverable: recovery=%+v error=%v", afterAction, err)
	}

	secondPrepared, _ := reserveAcceptedDecisionWithoutAction(t, fixture)
	second, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, binding)
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.SourcePolicyCallID == recovery.SourcePolicyCallID ||
		second.Prepared.Snapshot.SHA256() != secondPrepared.Prepared.Snapshot.SHA256() ||
		second.Prepared.Snapshot.CurrentRevision() != next {
		t.Fatalf("second sequential accepted recovery=%+v", second)
	}
	assertAcceptedRecoveryCounts(t, fixture, 2, 2, 1)
}

func TestPostgresAcceptedDecisionRecoveryReplaysCommittedReconciliationAcrossTakeover(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "accepted-reconciliation-recovery")
	_, _ = reserveAcceptedDecisionWithoutAction(t, fixture)
	second := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	secondBinding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(second),
	}
	secondRecovery, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, secondBinding)
	if err != nil {
		t.Fatal(err)
	}
	command := cognitionruntime.ReconciliationCommand{
		Binding: secondBinding, SnapshotSHA256: secondRecovery.Prepared.Snapshot.SHA256(),
		Projection:   secondRecovery.Prepared.Snapshot.ContextProjection(),
		ActionSchema: secondRecovery.ActionSchema, Decision: secondRecovery.Decision.Clone(),
		Recovery: recoveryRefForTest(secondRecovery.Ref()),
	}
	if _, err := fixture.Repository.ReconcileCognitionRuntimeDecision(fixture.Context, command); err != nil {
		t.Fatal(err)
	}

	third := replaceCognitionAttemptForTest(t, fixture.Pool, second)
	thirdBinding := cognitionruntime.Binding{
		Episode: secondBinding.Episode, Attempt: cognitionAttempt(third),
	}
	thirdRecovery, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, thirdBinding)
	if err != nil {
		t.Fatal(err)
	}
	if thirdRecovery == nil || thirdRecovery.SourcePolicyCallID != secondRecovery.SourcePolicyCallID ||
		thirdRecovery.ExistingReconciliation == nil ||
		thirdRecovery.ExistingReconciliation.Command.Binding != secondBinding {
		t.Fatalf("third-attempt recovery=%+v", thirdRecovery)
	}
	replayed, err := fixture.Repository.RecoverAcceptedCognitionDecision(fixture.Context, thirdBinding)
	if err != nil || replayed.Ref() != thirdRecovery.Ref() {
		t.Fatalf("exact recovery replay=%+v error=%v", replayed, err)
	}
	assertAcceptedRecoveryCounts(t, fixture, 1, 2, 0)
}
