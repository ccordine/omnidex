package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReplacementActivationFailureAtomicallySealsEpisode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	bootstrap := freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	failureEvidence := cognitionProviderFailureEvidence(
		t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload,
	)
	outcome, observationErr := cognitionpolicy.ObserveProviderProcess(
		ctx, cognitionFailurePolicyClient{evidence: failureEvidence},
		bootstrap.AttestedBrain, cognition.EpisodeRef{ID: fixture.EpisodeID},
		cognitionAttempt(replacement), cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observationErr == nil || outcome.Failure == nil {
		t.Fatalf("replacement process outcome=%+v error=%v", outcome, observationErr)
	}
	if err := repository.RecordCognitionProviderProcessFailure(
		ctx, bootstrap, *outcome.Failure,
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordCognitionProviderProcessFailure(
		ctx, bootstrap, *outcome.Failure,
	); err != nil {
		t.Fatalf("replacement activation failure replay: %v", err)
	}

	episode, err := repository.CognitionEpisode(ctx, fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if episode.Status != CognitionEpisodeCanceled || episode.TerminalOutcome !=
		"Provider activation failed before cognition could resume." {
		t.Fatalf("replacement failure episode=%+v", episode)
	}
	var failures, calls, actions, cancellations, seals int
	var cancellationCode string
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$1),
		(SELECT cancellation_code FROM cognition_episode_cancellations WHERE episode_id=$1)`,
		fixture.EpisodeID,
	).Scan(&failures, &calls, &actions, &cancellations, &seals, &cancellationCode); err != nil {
		t.Fatal(err)
	}
	if failures != 1 || calls != 0 || actions != 0 || cancellations != 1 || seals != 1 ||
		cancellationCode != "provider_activation_failed" {
		t.Fatalf("failure/calls/actions/cancellations/seals/code=%d/%d/%d/%d/%d/%s",
			failures, calls, actions, cancellations, seals, cancellationCode)
	}

	page, err := repository.ReadCognitionProviderActivationFailurePage(
		ctx, CognitionProviderActivationFailurePageRequest{
			Authority: replacement, EpisodeID: fixture.EpisodeID, Limit: 1,
		},
	)
	if err != nil || len(page.Records) != 1 || page.Records[0].Process == nil ||
		page.Records[0].SuccessfulBootstrap == nil || page.Records[0].BootstrapEvidence == nil {
		t.Fatalf("replacement failure page=%+v error=%v", page, err)
	}
	trace, err := repository.ReadCognitionSealedTrace(
		ctx, fixture.EpisodeID, CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	seenFailure, seenCancellation := false, false
	bootstrapSources := map[CognitionBrainBootstrapTraceSource]bool{}
	for _, record := range trace.Records {
		seenFailure = seenFailure || record.Kind == "provider_activation_failure"
		seenCancellation = seenCancellation || record.Kind == "cancellation_evidence"
		if record.Kind == CognitionTraceKindProviderBrainBootstrap {
			var payload CognitionBrainBootstrapTrace
			if err := json.Unmarshal(record.Payload, &payload); err != nil || payload.Validate() != nil {
				t.Fatalf("provider Brain bootstrap trace payload=%s error=%v", record.Payload, err)
			}
			bootstrapSources[payload.Source] = true
			if _, err := repository.ReadCognitionProviderIdentityEvidenceManifest(
				ctx, fixture.EpisodeID, payload.Evidence.ID,
			); err != nil {
				t.Fatalf("read sealed bootstrap raw evidence %s: %v", payload.Source, err)
			}
		}
	}
	if !seenFailure || !seenCancellation ||
		!bootstrapSources[CognitionBrainBootstrapEpisodeStart] ||
		!bootstrapSources[CognitionBrainBootstrapActivationFailure] ||
		trace.Seal.SealedBy != replacement {
		t.Fatalf("replacement terminal trace=%+v", trace)
	}

	third := replaceCognitionAttemptForTest(t, pool, replacement)
	replay := fixture.Start
	replay.Authority = third
	replay.BrainBootstrap = freshReplayBrainBootstrap(t, bootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, third, replay.BrainBootstrap.AttestedBrain,
	)
	replayed, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Status != CognitionEpisodeCanceled || replayed.TerminalOutcome != episode.TerminalOutcome {
		t.Fatalf("terminal activation failure replay=%+v", replayed)
	}
	var replayCalls, replayActions int
	var replayTrace string
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		(SELECT trace_sha256 FROM cognition_terminal_seals WHERE episode_id=$1)`,
		fixture.EpisodeID,
	).Scan(&replayCalls, &replayActions, &replayTrace); err != nil {
		t.Fatal(err)
	}
	if replayCalls != 0 || replayActions != 0 || replayTrace != trace.TraceSHA256 {
		t.Fatalf("terminal replay calls/actions/trace=%d/%d/%s", replayCalls, replayActions, replayTrace)
	}
}

func TestPostgresDirectProviderFailureCannotLeaveActiveEpisode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	assertSQLAttestedBrainExact(t, pool, fixture.Start.BrainBootstrap.AttestedBrain)
	if _, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	bootstrap := freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	evidence := cognitionProviderFailureEvidence(
		t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload,
	)
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		ctx, cognitionFailurePolicyClient{evidence: evidence}, bootstrap.AttestedBrain,
		cognition.EpisodeRef{ID: fixture.EpisodeID}, cognitionAttempt(replacement),
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err == nil || outcome.Failure == nil {
		t.Fatalf("replacement process outcome=%+v error=%v", outcome, err)
	}
	row := newDirectProviderFailureRow(
		t, replacement, fixture.EpisodeID, bootstrap, *outcome.Failure,
	)
	assertDirectProviderFailureRejected(t, repository, replacement, row)

	var activeEpisodes, failures, cancellations, seals int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1 AND status='active'),
		(SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$1)`,
		fixture.EpisodeID,
	).Scan(&activeEpisodes, &failures, &cancellations, &seals); err != nil {
		t.Fatal(err)
	}
	if activeEpisodes != 1 || failures != 0 || cancellations != 0 || seals != 0 {
		t.Fatalf("active/failure/cancellation/seal=%d/%d/%d/%d",
			activeEpisodes, failures, cancellations, seals)
	}
}

func assertSQLAttestedBrainExact(
	t *testing.T,
	pool *pgxpool.Pool,
	brain cognitionpolicy.AttestedBrain,
) {
	t.Helper()
	raw, _, err := cognitionJSON(brain)
	if err != nil {
		t.Fatal(err)
	}
	var exact, unique, ref, sampling, provider, host, observation bool
	if err := pool.QueryRow(t.Context(), `SELECT
		cognition_attested_brain_is_exact($1::jsonb),
		cognition_json_has_unique_keys($1::json),
		cognition_brain_ref_is_exact($1::jsonb->'brain'),
		cognition_sampling_identity_is_exact($1::jsonb->'brain'->'sampling'),
		cognition_provider_attestation_matches_brain(
		  $1::jsonb->'provider_attestation',$1::jsonb->'brain'),
		cognition_host_attestation_is_exact($1::jsonb->'host_hardware_attestation'),
		cognition_provider_observation_is_exact(
		  $1::jsonb->'bootstrap_provider_observation',
		  $1::jsonb->'provider_attestation'->>'attestation_sha256',
		  cognition_provider_bootstrap_challenge($1::jsonb->'brain'))`, string(raw)).Scan(
		&exact, &unique, &ref, &sampling, &provider, &host, &observation,
	); err != nil {
		t.Fatal(err)
	}
	if !exact || !unique {
		t.Fatalf("SQL AttestedBrain exact/unique/ref/sampling/provider/host/observation=%v/%v/%v/%v/%v/%v/%v raw=%s",
			exact, unique, ref, sampling, provider, host, observation, raw)
	}
}
