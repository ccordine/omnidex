package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresFreshSchemaReplacementOwnsExactPolicyAbandonment(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "fresh-replacement-abandonment",
	)
	attempt := reserveIndeterminateCognitionCall(t, fixture)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
		Attempt: cognitionAttempt(replacement),
	}
	abandonment, err := repository.AbandonIndeterminateCognitionPolicyCall(ctx, binding)
	if err != nil {
		t.Fatalf("exact replacement abandonment: %v", err)
	}
	if abandonment == nil || abandonment.CallID != attempt.ID ||
		abandonment.SourceDisposition != cognitionruntime.SourceAttemptExpired ||
		abandonment.RecoveryActor != binding.Attempt {
		t.Fatalf("replacement abandonment=%+v", abandonment)
	}
	if err := repository.FinishCognitionPolicyCall(
		ctx, attempt, providerIdentityFailureResult(attempt), cognitionpolicy.CallEvidence{},
	); err == nil {
		t.Fatal("stale source finished after exact replacement abandonment")
	}
}

func TestPostgresFreshSchemaRejectsInvalidPolicyAbandonmentActors(t *testing.T) {
	t.Run("live source", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := startTaskGenerationRetirementFixtureIn(
			t, repository, pool, ctx, "fresh-live-source-abandonment",
		)
		_ = reserveIndeterminateCognitionCall(t, fixture)
		binding := cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
			Attempt: cognitionAttempt(fixture.Authority),
		}
		if value, err := repository.AbandonIndeterminateCognitionPolicyCall(
			ctx, binding,
		); !errors.Is(err, cognitionpolicy.ErrCallIndeterminate) || value != nil {
			t.Fatalf("live source abandonment=%+v error=%v", value, err)
		}
	})
	t.Run("changed replacement", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := startTaskGenerationRetirementFixtureIn(
			t, repository, pool, ctx, "fresh-changed-replacement",
		)
		_ = reserveIndeterminateCognitionCall(t, fixture)
		replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
		replacement.WorkerID += "-changed"
		binding := cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
		}
		if value, err := repository.AbandonIndeterminateCognitionPolicyCall(
			ctx, binding,
		); !errors.Is(err, ErrStaleStepAttempt) || value != nil {
			t.Fatalf("changed replacement abandonment=%+v error=%v", value, err)
		}
	})
	t.Run("expired replacement", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := startTaskGenerationRetirementFixtureIn(
			t, repository, pool, ctx, "fresh-expired-replacement",
		)
		_ = reserveIndeterminateCognitionCall(t, fixture)
		replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
		_ = replaceCognitionAttemptForTest(t, pool, replacement)
		binding := cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: cognitionAttempt(replacement),
		}
		if value, err := repository.AbandonIndeterminateCognitionPolicyCall(
			ctx, binding,
		); !errors.Is(err, ErrStaleStepAttempt) || value != nil {
			t.Fatalf("expired replacement abandonment=%+v error=%v", value, err)
		}
	})
}
