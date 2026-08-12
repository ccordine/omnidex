package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresProviderActivationFailureAndSuccessAreMutuallyExclusive(t *testing.T) {
	t.Run("bootstrap failure then start", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := prepareTaskGenerationRetirementFixture(
			t, repository, pool, ctx, "bootstrap-failure-then-success",
		)
		failure := cognitionBrainBootstrapFailure(t, fixture, llm.ProviderIdentityTokenizer)
		if err := repository.RecordCognitionBrainBootstrapFailure(
			ctx, fixture.Authority, fixture.EpisodeID, failure,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.StartCognitionEpisode(
			ctx, fixture.Start, cognitionTestFactAuthority(),
		); err == nil {
			t.Fatal("successful Start followed a recorded bootstrap failure")
		}
		assertProviderActivationOutcomeCounts(t, pool, fixture, 0, 1, 0, 0)
	})

	t.Run("start then bootstrap failure", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := prepareTaskGenerationRetirementFixture(
			t, repository, pool, ctx, "bootstrap-success-then-failure",
		)
		if _, err := repository.StartCognitionEpisode(
			ctx, fixture.Start, cognitionTestFactAuthority(),
		); err != nil {
			t.Fatal(err)
		}
		failure := cognitionBrainBootstrapFailure(t, fixture, llm.ProviderIdentityTokenizer)
		if err := repository.RecordCognitionBrainBootstrapFailure(
			ctx, fixture.Authority, fixture.EpisodeID, failure,
		); err == nil {
			t.Fatal("bootstrap failure followed a successful Start")
		}
		assertProviderActivationOutcomeCounts(t, pool, fixture, 1, 0, 1, 1)
	})

	t.Run("process failure then start", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := prepareTaskGenerationRetirementFixture(
			t, repository, pool, ctx, "process-failure-then-success",
		)
		failure := cognitionProviderProcessFailure(t, fixture,
			cognitionProviderFailureEvidence(t, fixture.Start.BrainBootstrap.AttestedBrain,
				llm.ProviderIdentityPreload))
		if err := repository.RecordCognitionProviderProcessFailure(
			ctx, fixture.Start.BrainBootstrap, failure,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.StartCognitionEpisode(
			ctx, fixture.Start, cognitionTestFactAuthority(),
		); err == nil {
			t.Fatal("successful process activation followed a recorded process failure")
		}
		assertProviderActivationOutcomeCounts(t, pool, fixture, 0, 1, 0, 0)
	})

	t.Run("start then process failure", func(t *testing.T) {
		repository, pool, ctx := policyInputFreshRepository(t)
		fixture := prepareTaskGenerationRetirementFixture(
			t, repository, pool, ctx, "process-success-then-failure",
		)
		if _, err := repository.StartCognitionEpisode(
			ctx, fixture.Start, cognitionTestFactAuthority(),
		); err != nil {
			t.Fatal(err)
		}
		failure := cognitionProviderProcessFailure(t, fixture,
			cognitionProviderFailureEvidence(t, fixture.Start.BrainBootstrap.AttestedBrain,
				llm.ProviderIdentityPreload))
		if err := repository.RecordCognitionProviderProcessFailure(
			ctx, fixture.Start.BrainBootstrap, failure,
		); err == nil {
			t.Fatal("process failure followed a successful process activation")
		}
		assertProviderActivationOutcomeCounts(t, pool, fixture, 1, 0, 1, 1)
	})
}

func TestPostgresProviderInvocationRejectsSecondFailureByDirectSQL(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "direct-second-provider-failure",
	)
	failure := cognitionBrainBootstrapFailure(t, fixture, llm.ProviderIdentityTokenizer)
	if err := repository.RecordCognitionBrainBootstrapFailure(
		ctx, fixture.Authority, fixture.EpisodeID, failure,
	); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO cognition_provider_activation_failures (
		record_id,failure_kind,failure_id,episode_id,evidence_id,
		job_id,generation,step_id,step_attempt,worker_id,
		receipt_json,receipt_sha256,authority_json,authority_sha256
	) SELECT $2,failure_kind,$3,episode_id,evidence_id,
		job_id,generation,step_id,step_attempt,worker_id,
		receipt_json,receipt_sha256,authority_json,authority_sha256
	FROM cognition_provider_activation_failures WHERE episode_id=$1`,
		fixture.EpisodeID, "cognition_provider_failure_"+strings.Repeat("a", 64),
		"brain_bootstrap_failure_"+strings.Repeat("b", 64))
	if err == nil {
		t.Fatal("direct SQL inserted a second failure for one provider invocation")
	}
	assertProviderActivationOutcomeCounts(t, pool, fixture, 0, 1, 0, 0)
}

func cognitionBrainBootstrapFailure(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	failing llm.ProviderIdentityOperation,
) cognitionpolicy.BrainBootstrapFailure {
	t.Helper()
	brain := fixture.Start.BrainBootstrap.AttestedBrain
	evidence := cognitionProviderFailureEvidence(t, brain, failing)
	outcome, err := cognitionpolicy.AttestBrain(
		t.Context(), cognitionFailurePolicyClient{evidence: evidence}, brain.Ref,
	)
	if err == nil || outcome.Failure == nil {
		t.Fatalf("bootstrap failure outcome=%+v error=%v", outcome, err)
	}
	return *outcome.Failure
}

func assertProviderActivationOutcomeCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	fixture taskGenerationRetirementFixture,
	episodes, failures, bootstraps, processes int,
) {
	t.Helper()
	var gotEpisodes, gotFailures, gotBootstraps, gotProcesses, unassociated int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_provider_identity_evidence WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_identity_evidence evidence WHERE NOT EXISTS (
		  SELECT 1 FROM cognition_provider_activation_failures failure
		  WHERE evidence.evidence_id IN (failure.evidence_id,failure.bootstrap_evidence_id)
		) AND NOT EXISTS (
		  SELECT 1 FROM cognition_episode_provider_identity_evidence bootstrap
		  WHERE bootstrap.evidence_id=evidence.evidence_id
		) AND NOT EXISTS (
		  SELECT 1 FROM cognition_provider_process_observations process
		  WHERE process.evidence_id=evidence.evidence_id
		))`, fixture.EpisodeID).Scan(
		&gotEpisodes, &gotFailures, &gotBootstraps, &gotProcesses, &unassociated,
	); err != nil {
		t.Fatal(err)
	}
	if gotEpisodes != episodes || gotFailures != failures || gotBootstraps != bootstraps ||
		gotProcesses != processes || unassociated != 0 {
		t.Fatalf("provider outcomes episodes/failures/bootstrap/process/unassociated=%d/%d/%d/%d/%d",
			gotEpisodes, gotFailures, gotBootstraps, gotProcesses, unassociated)
	}
}
