package queue

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresActiveReplayBootstrapRequiresItsExactProcessObservation(t *testing.T) {
	tests := []struct {
		name           string
		stageUnrelated bool
	}{
		{name: "bootstrap only"},
		{name: "unrelated same-actor process", stageUnrelated: true},
	}
	for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
			repository, pool, ctx := policyInputFreshRepository(t)
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "active-replay-process-totality",
			)
			replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
			replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
			if test.stageUnrelated {
				unrelated := cognitionGuardProviderProcessActivationFor(
					t, ctx, fixture.EpisodeID, replacement,
					replay.BrainBootstrap.AttestedBrain,
				)
				if err := repository.RecordCognitionProviderProcessObservation(
					ctx, unrelated,
				); err != nil {
					t.Fatal(err)
				}
				replay.ProviderProcessActivation = unrelated
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err := insertCognitionEpisodeReplayBootstrapEvidenceTx(ctx, tx, replay); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx,
				"SET CONSTRAINTS cognition_episode_replay_identity_evidence_exact IMMEDIATE",
			)
			if err == nil || !strings.Contains(err.Error(), "replay bootstrap") {
				t.Fatalf("%s deferred totality error=%v", test.name, err)
			}
		})
	}
}

func TestPostgresActiveReplayBootstrapExactProcessPairPassesAndSeals(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "active-replay-exact-process-pair",
	)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	fixture.Authority = replacement
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	var exactLinks int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)
		FROM cognition_episode_replay_provider_identity_evidence replay
		JOIN cognition_provider_process_observations process
		  ON process.observation_id=replay.process_observation_id
		WHERE replay.episode_id=$1 AND replay.step_attempt=$2
		  AND replay.process_receipt_sha256=process.receipt_sha256
		  AND replay.process_evidence_id=process.evidence_id
		  AND process.observed_at>=replay.observed_at`,
		fixture.EpisodeID, replacement.Attempt,
	).Scan(&exactLinks); err != nil || exactLinks != 1 {
		t.Fatalf("sealed exact active replay link count=%d error=%v", exactLinks, err)
	}
}

func TestCognitionEpisodeStartRejectsProcessObservationBeforeBootstrap(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "process-before-bootstrap-go",
	)
	inverted := cognitionReplayWithProcessBeforeBootstrap(t, fixture, fixture.Authority)
	if _, err := repository.StartCognitionEpisode(
		ctx, inverted, cognitionTestFactAuthority(),
	); err == nil || !strings.Contains(err.Error(), "predates") {
		t.Fatalf("inverted process/bootstrap Start error=%v", err)
	}
}

func TestPostgresActiveReplayRejectsProcessObservationBeforeBootstrap(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "process-before-bootstrap-sql",
	)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	inverted := cognitionReplayWithProcessBeforeBootstrap(t, fixture, replacement)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := insertCognitionEpisodeReplayBootstrapEvidenceTx(ctx, tx, inverted); err != nil {
		t.Fatal(err)
	}
	if err := persistCognitionProviderProcessActivationTx(
		ctx, tx, replacement,
		cognitionActiveReplayEpisode(fixture, replacement, inverted.BrainBootstrap.AttestedBrain),
		inverted.ProviderProcessActivation, "",
	); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit(ctx)
	if err == nil || !strings.Contains(err.Error(), "replay bootstrap") {
		t.Fatalf("inverted direct replay commit error=%v", err)
	}
}

func TestPostgresActiveReplayBootstrapRejectsChangedProcessLink(t *testing.T) {
	for _, mutation := range []string{"process ID", "receipt digest"} {
		t.Run(mutation, func(t *testing.T) {
			repository, pool, ctx := policyInputFreshRepository(t)
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "active-replay-process-link",
			)
			replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
			replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
			if _, err := repository.StartCognitionEpisode(
				ctx, replay, cognitionTestFactAuthority(),
			); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, `CREATE TEMP TABLE saved_replay_bootstrap
				ON COMMIT DROP AS SELECT *
				FROM cognition_episode_replay_provider_identity_evidence
				WHERE episode_id=$1 AND step_attempt=$2`,
				fixture.EpisodeID, replacement.Attempt,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `ALTER TABLE cognition_episode_replay_provider_identity_evidence
				DISABLE TRIGGER cognition_episode_replay_identity_evidence_immutable`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `DELETE FROM cognition_episode_replay_provider_identity_evidence
				WHERE episode_id=$1 AND step_attempt=$2`, fixture.EpisodeID, replacement.Attempt); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `ALTER TABLE cognition_episode_replay_provider_identity_evidence
				ENABLE TRIGGER cognition_episode_replay_identity_evidence_immutable`); err != nil {
				t.Fatal(err)
			}
			processID := "process_observation_id"
			receiptSHA := "process_receipt_sha256"
			if mutation == "process ID" {
				processID = `(SELECT observation_id FROM cognition_provider_process_observations
					WHERE episode_id=saved.episode_id AND sequence=1)`
			} else {
				receiptSHA = `repeat('f',64)`
			}
			_, err = tx.Exec(ctx, `INSERT INTO cognition_episode_replay_provider_identity_evidence (
				replay_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
				provider_observation_json,provider_observation_json_sha256,
				provider_observation_sha256,observed_at,process_observation_id,
				process_receipt_sha256,process_evidence_id,authority_json,authority_sha256,created_at
			) SELECT replay_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,
				worker_id,provider_observation_json,provider_observation_json_sha256,
				provider_observation_sha256,observed_at,`+processID+`,`+receiptSHA+`,
				process_evidence_id,authority_json,authority_sha256,created_at
			FROM saved_replay_bootstrap saved`)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx,
				"SET CONSTRAINTS cognition_episode_replay_identity_evidence_exact IMMEDIATE",
			)
			if err == nil || !strings.Contains(err.Error(), "replay bootstrap") {
				t.Fatalf("changed %s error=%v", mutation, err)
			}
		})
	}
}

func cognitionReplayWithProcessBeforeBootstrap(
	t *testing.T, fixture taskGenerationRetirementFixture, authority model.StepAttemptAuthority,
) CognitionEpisodeStart {
	t.Helper()
	replay := fixture.Start
	replay.Authority = authority
	processBrain := freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, fixture.Context, fixture.EpisodeID, authority, processBrain.AttestedBrain,
	)
	replay.BrainBootstrap = cognitionBootstrapAt(
		t, processBrain,
		replay.ProviderProcessActivation.Receipt.Observation.ObservedAt.Add(time.Microsecond),
	)
	if !replay.ProviderProcessActivation.Receipt.Observation.ObservedAt.Before(
		replay.BrainBootstrap.AttestedBrain.BootstrapObservation.ObservedAt,
	) {
		t.Fatal("test fixture did not invert provider observation causality")
	}
	return replay
}

func cognitionBootstrapAt(
	t *testing.T, original cognitionpolicy.BrainBootstrap, observedAt time.Time,
) cognitionpolicy.BrainBootstrap {
	t.Helper()
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(original.AttestedBrain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := queueTestObservedProviderIdentity(
		observedAt, original.AttestedBrain.Attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewAttestedBrain(
		original.AttestedBrain.Ref, original.AttestedBrain.Attestation,
		observed.Observation, original.AttestedBrain.Host,
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(brain, observed.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	return bootstrap
}

func cognitionActiveReplayEpisode(
	fixture taskGenerationRetirementFixture,
	authority model.StepAttemptAuthority,
	brain cognitionpolicy.AttestedBrain,
) CognitionEpisode {
	return CognitionEpisode{
		EpisodeID: fixture.EpisodeID, Authority: authority,
		AttestedBrain: brain,
		Status:        CognitionEpisodeActive,
	}
}
