package queue

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresAuthorityDeniedPolicyOutcomeReplaysExactSentinels(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "authority-denied-policy-replay")
	response := fmt.Sprintf(`{
		"obligation_id":%q,
		"action":{"kind":"observe","arguments":[]},
		"evidence_refs":[],
		"expected_effect":"Inspect the bounded target.","complete":true
	}`, fixture.Start.Root.ID)
	err := reserveTerminalCognitionPolicyCall(
		t, fixture, cognitionGuardPolicyClient{response: response},
	)
	if !errors.Is(err, cognitionpolicy.ErrInvalidDecision) || !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("fresh authority error=%v", err)
	}
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}
	recovered, err := fixture.Repository.ReplayTerminalCognitionPolicyOutcome(fixture.Context, binding)
	if !recovered || !errors.Is(err, cognitionpolicy.ErrInvalidDecision) ||
		!errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("authority replay recovered=%v error=%v", recovered, err)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 || abandoned != 0 {
		t.Fatalf("authority replay rows=%d/%d/%d", snapshots, calls, abandoned)
	}
}

func TestPostgresRejectedPolicyOutcomeReplaysForSameAndReplacementAttempts(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "rejected-policy-replay")
	if err := reserveTerminalCognitionPolicyCall(
		t, fixture, cognitionGuardPolicyClient{response: "not-json"},
	); !errors.Is(err, cognitionpolicy.ErrInvalidDecision) {
		t.Fatalf("fresh rejected error=%v", err)
	}
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}
	recovered, err := fixture.Repository.ReplayTerminalCognitionPolicyOutcome(fixture.Context, binding)
	if !recovered || !errors.Is(err, cognitionpolicy.ErrInvalidDecision) {
		t.Fatalf("same-attempt replay recovered=%v error=%v", recovered, err)
	}
	if snapshots, calls, abandoned := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 || abandoned != 0 {
		t.Fatalf("same replay rows=%d/%d/%d", snapshots, calls, abandoned)
	}
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	binding.Attempt = cognitionAttempt(replacement)
	recovered, err = fixture.Repository.ReplayTerminalCognitionPolicyOutcome(fixture.Context, binding)
	if !recovered || !errors.Is(err, cognitionpolicy.ErrInvalidDecision) {
		t.Fatalf("replacement replay recovered=%v error=%v", recovered, err)
	}
	if snapshots, calls, _ := countCognitionRecoveryRows(t, fixture); snapshots != 1 || calls != 1 {
		t.Fatalf("replacement replay created work: snapshots/calls=%d/%d", snapshots, calls)
	}
}

func TestPostgresFailedPolicyOutcomeReplaysRegisteredGenerationError(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "failed-policy-replay",
	)
	client := failingCognitionPolicyClient{cognitionGuardPolicyClient{response: "unused"}}
	if err := reserveTerminalCognitionPolicyCall(t, fixture, client); !errors.Is(err, cognitionpolicy.ErrGeneration) {
		t.Fatalf("fresh failed error=%v", err)
	}
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}
	recovered, err := fixture.Repository.ReplayTerminalCognitionPolicyOutcome(fixture.Context, binding)
	if !recovered || !errors.Is(err, cognitionpolicy.ErrGeneration) {
		t.Fatalf("failed replay recovered=%v error=%v", recovered, err)
	}
}

func TestPostgresPolicyRecoveryReturnsNoOutcomeWithoutDurableCandidate(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "no-policy-replay")
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(fixture.Authority),
	}
	recovered, err := fixture.Repository.ReplayTerminalCognitionPolicyOutcome(fixture.Context, binding)
	if err != nil || recovered {
		t.Fatalf("recovered=%v error=%v", recovered, err)
	}
	if abandonment, err := fixture.Repository.AbandonIndeterminateCognitionPolicyCall(
		fixture.Context, binding,
	); err != nil || abandonment != nil {
		t.Fatalf("abandonment=%+v error=%v", abandonment, err)
	}
}
