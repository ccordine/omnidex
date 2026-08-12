package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func TestPostgresCognitionPolicyCallsConsumeOneDurableEpisodeBudget(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "policy-budget")
	fixture.Start.Budget.RemainingPolicyCalls = 2
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil || !reflect.DeepEqual(replayed, prepared) {
		t.Fatalf("clean-desk snapshot replay=%+v error=%v", replayed, err)
	}
	var replayCalls, replaySnapshots int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&replayCalls, &replaySnapshots); err != nil {
		t.Fatal(err)
	}
	if replayCalls != 0 || replaySnapshots != 1 {
		t.Fatalf("snapshot replay calls/snapshots=%d/%d want 0/1", replayCalls, replaySnapshots)
	}

	if err := callCognitionGuardPolicy(t, fixture, 2); err != nil {
		t.Fatalf("first policy call: %v", err)
	}
	if err := callCognitionGuardPolicy(t, fixture, 1); err != nil {
		t.Fatalf("second policy call: %v", err)
	}
	if err := callCognitionGuardPolicy(t, fixture, 0); err == nil {
		t.Fatal("exhausted clean-desk snapshot unexpectedly reached inference")
	}

	var calls int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&calls); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("durable policy calls=%d want 2", calls)
	}
	var snapshots int
	var exhaustedOrdinal, exhaustedRemaining int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),MAX(call_ordinal),MIN(
			(runtime_budget_json::jsonb->>'remaining_policy_calls')::INTEGER
		) FROM cognition_runtime_snapshots WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&snapshots, &exhaustedOrdinal, &exhaustedRemaining); err != nil {
		t.Fatal(err)
	}
	if snapshots != 3 || exhaustedOrdinal != 3 || exhaustedRemaining != 0 {
		t.Fatalf("durable snapshots/last ordinal/remaining=%d/%d/%d want 3/3/0",
			snapshots, exhaustedOrdinal, exhaustedRemaining)
	}
}

func TestPostgresCognitionEpisodeStartBudgetReplayIsExact(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "start-budget-replay")
	changed := fixture.Start
	changed.Budget.RemainingPolicyCalls--
	if _, err := fixture.Repository.StartCognitionEpisode(fixture.Context, changed, cognitionTestFactAuthority()); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("changed start budget replay error=%v", err)
	}
}

func callCognitionGuardPolicy(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	expectedRemaining uint32,
) error {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		return err
	}
	snapshot := prepared.Prepared.Snapshot
	if snapshot.Budget().RemainingPolicyCalls != expectedRemaining {
		t.Fatalf("remaining policy calls=%d want %d", snapshot.Budget().RemainingPolicyCalls, expectedRemaining)
	}
	schema := fixture.Start.ActionCatalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: request,
		EvidenceRefs: []cognition.EvidenceRef{}, ExpectedEffect: "Expose bounded public state.",
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)},
		cognitionTestBrain(), cognitionGuardActivationAuthority(t, fixture),
		cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = policy.Decide(fixture.Context, snapshot)
	return err
}
