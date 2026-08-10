package queue

import (
	"errors"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresSameAttemptCannotAbandonItsIndeterminatePolicyCall(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "same-attempt-indeterminate")
	_ = reserveIndeterminateCognitionCall(t, fixture)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}

	abandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(fixture.Context, binding)
	if !errors.Is(err, cognitionpolicy.ErrCallIndeterminate) || abandonment != nil {
		t.Fatalf("abandonment=%+v error=%v", abandonment, err)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 || abandoned != 0 {
		t.Fatalf("snapshots/calls/abandonments=%d/%d/%d", snapshots, calls, abandoned)
	}
}

func TestPostgresStaleFinishRacesTypedAbandonmentWithoutActionAuthority(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "abandonment-finish-race")
	attempt := reserveIndeterminateCognitionCall(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}

	start := make(chan struct{})
	abandonmentErrors := make(chan error, 1)
	finishErrors := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(fixture.Context, binding)
		abandonmentErrors <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		finishErrors <- fixture.Repository.FinishCognitionPolicyCall(
			fixture.Context, attempt, providerIdentityFailureResult(attempt),
			cognitionpolicy.CallEvidence{},
		)
	}()
	close(start)
	wait.Wait()
	if err := <-abandonmentErrors; err != nil {
		t.Fatalf("abandonment lost exact replacement authority: %v", err)
	}
	if err := <-finishErrors; err == nil {
		t.Fatal("stale source finished while replacement abandoned the call")
	}
	var calls, abandonments, actions, receipts int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1 AND status='abandoned'),
		 (SELECT COUNT(*) FROM cognition_policy_call_abandonments WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_environment_receipts WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&calls, &abandonments, &actions, &receipts); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || abandonments != 1 || actions != 0 || receipts != 0 {
		t.Fatalf("calls/abandonments/actions/receipts=%d/%d/%d/%d", calls, abandonments, actions, receipts)
	}
}

func TestPostgresChangedReplacementCannotAbandonIndeterminateCall(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "changed-replacement-abandonment")
	_ = reserveIndeterminateCognitionCall(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	replacement.WorkerID += "-changed"
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}
	if abandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(
		fixture.Context, binding,
	); !errors.Is(err, ErrStaleStepAttempt) || abandonment != nil {
		t.Fatalf("changed replacement abandonment=%+v error=%v", abandonment, err)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 || abandoned != 0 {
		t.Fatalf("snapshots/calls/abandonments=%d/%d/%d", snapshots, calls, abandoned)
	}
}

func TestPostgresMultipleIndeterminateCallsFailWithoutGuessing(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "multiple-indeterminate-calls")
	_ = reserveIndeterminateCognitionCall(t, fixture)
	_ = reserveIndeterminateCognitionCall(t, fixture)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}
	if abandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(
		fixture.Context, binding,
	); !errors.Is(err, ErrCognitionConflict) || abandonment != nil {
		t.Fatalf("multiple-call abandonment=%+v error=%v", abandonment, err)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 2 || calls != 2 || abandoned != 0 {
		t.Fatalf("snapshots/calls/abandonments=%d/%d/%d", snapshots, calls, abandoned)
	}
}

func TestPostgresSuccessiveReplacementAttemptsAbandonOneCallEach(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "successive-call-takeovers")
	first := reserveIndeterminateCognitionCall(t, fixture)
	firstReplacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	firstBinding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(firstReplacement),
	}
	firstAbandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(
		fixture.Context, firstBinding,
	)
	if err != nil {
		t.Fatal(err)
	}

	fixture.Authority = firstReplacement
	second := reserveIndeterminateCognitionCall(t, fixture)
	secondReplacement := replaceCognitionAttemptForTest(t, fixture.Pool, firstReplacement)
	secondBinding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(secondReplacement),
	}
	secondAbandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(
		fixture.Context, secondBinding,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstAbandonment.CallID != first.ID || secondAbandonment.CallID != second.ID ||
		firstAbandonment.ID == secondAbandonment.ID || secondAbandonment.CallOrdinal != 2 {
		t.Fatalf("successive abandonments first=%+v second=%+v", firstAbandonment, secondAbandonment)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 2 || calls != 2 || abandoned != 2 {
		t.Fatalf("snapshots/calls/abandonments=%d/%d/%d", snapshots, calls, abandoned)
	}
}

func TestPostgresReplacementAtomicallyAbandonsAndReplaysExactCall(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "replacement-abandonment")
	attempt := reserveIndeterminateCognitionCall(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}

	abandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(fixture.Context, binding)
	if err != nil {
		t.Fatal(err)
	}
	if abandonment == nil || abandonment.CallID != attempt.ID ||
		abandonment.SourceActor != attempt.Actor || abandonment.RecoveryActor != binding.Attempt ||
		abandonment.SourceDisposition != cognitionruntime.SourceAttemptExpired ||
		abandonment.ValidateFor(binding) != nil {
		t.Fatalf("abandonment=%+v", abandonment)
	}
	replay, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(fixture.Context, binding)
	if err != nil || replay == nil || replay.Ref() != abandonment.Ref() {
		t.Fatalf("abandonment replay=%+v error=%v", replay, err)
	}
	if err := fixture.Repository.FinishCognitionPolicyCall(
		fixture.Context, attempt, providerIdentityFailureResult(attempt),
		cognitionpolicy.CallEvidence{},
	); err == nil {
		t.Fatal("stale source finished an abandoned policy call")
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 || abandoned != 1 {
		t.Fatalf("snapshots/calls/abandonments=%d/%d/%d", snapshots, calls, abandoned)
	}
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context, CognitionRuntimeSnapshotCommand{Authority: replacement, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.CallOrdinal != 2 || prepared.Prepared.Snapshot.Budget().RemainingPolicyCalls !=
		fixture.Start.Budget.RemainingPolicyCalls-1 {
		t.Fatalf("replacement snapshot ordinal/budget=%d/%d", prepared.CallOrdinal,
			prepared.Prepared.Snapshot.Budget().RemainingPolicyCalls)
	}
}

func TestPostgresAbandonedCallIsConsumedWhenEpisodeBudgetIsExhausted(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "abandon-budget")
	fixture.Start.Budget.RemainingPolicyCalls = 1
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	_ = reserveIndeterminateCognitionCall(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
	}
	if _, err := repository.AbandonIndeterminateCognitionPolicyCall(ctx, binding); err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: replacement, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Prepared.Snapshot.Budget().RemainingPolicyCalls != 0 {
		t.Fatalf("abandoned call was refunded: budget=%+v", prepared.Prepared.Snapshot.Budget())
	}
}
