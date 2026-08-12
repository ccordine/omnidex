package queue

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPostgresActiveProviderObservationExactRetryAfterSealIsNoOp(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "active-provider-observation-crossseal-retry",
	)
	activation := providerProcessReceiptForTest(t, fixture)
	if err := repository.RecordCognitionProviderProcessObservation(ctx, activation); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	sealedBeforeRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	if err := repository.RecordCognitionProviderProcessObservation(ctx, activation); err != nil {
		t.Fatalf("exact active observation retry after seal: %v", err)
	}
	if sealedAfterRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID); !bytes.Equal(sealedAfterRetry, sealedBeforeRetry) {
		t.Fatal("exact active observation retry changed the sealed trace")
	}
	assertProviderObservationStoredOnce(t, fixture, activation.Receipt.ID)
}

func TestPostgresProviderObservationAndSealRaceRetryIsOneObservation(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "provider-observation-seal-race-retry",
	)
	activation := providerProcessReceiptForTest(t, fixture)
	start := make(chan struct{})
	observationDone, sealDone := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		observationDone <- repository.RecordCognitionProviderProcessObservation(ctx, activation)
	}()
	go func() {
		<-start
		_, err := repository.CancelCognitionEpisode(
			ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
		)
		sealDone <- err
	}()
	close(start)
	if err := <-observationDone; err != nil {
		t.Fatalf("racing provider observation: %v", err)
	}
	if err := <-sealDone; err != nil {
		t.Fatalf("racing cognition seal: %v", err)
	}
	sealedBeforeRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	if err := repository.RecordCognitionProviderProcessObservation(ctx, activation); err != nil {
		t.Fatalf("exact provider observation retry after race: %v", err)
	}
	if sealedAfterRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID); !bytes.Equal(sealedAfterRetry, sealedBeforeRetry) {
		t.Fatal("provider observation race retry changed the sealed trace")
	}
	assertProviderObservationStoredOnce(t, fixture, activation.Receipt.ID)
}

func TestPostgresPublicProviderObservationRejectsReplayOnlySource(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "public-provider-source-conflict",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordCognitionProviderProcessObservation(
		ctx, replay.ProviderProcessActivation,
	); !errors.Is(err, ErrCognitionConflict) || !strings.Contains(err.Error(), "source") {
		t.Fatalf("public observation accepted replay-only source: %v", err)
	}
}

func assertProviderObservationStoredOnce(
	t *testing.T, fixture taskGenerationRetirementFixture, observationID string,
) {
	t.Helper()
	var active, postSeal int
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE observation_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_postseal_observations WHERE observation_id=$1)`,
		observationID,
	).Scan(&active, &postSeal); err != nil {
		t.Fatal(err)
	}
	if active+postSeal != 1 {
		t.Fatalf("provider observation active/postseal=%d/%d want one total", active, postSeal)
	}
}
