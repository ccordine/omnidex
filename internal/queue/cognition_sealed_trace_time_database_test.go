package queue

import (
	"errors"
	"testing"
	"time"
)

func TestPostgresCognitionSealedTraceExposesExactUTCMicrosecondTimes(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "sealed-trace-time-authority",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ReadCognitionSealedTrace(
		ctx, fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]time.Time{
		"episode start": page.EpisodeStartedAt,
		"sealed at":     page.SealedAt,
		"terminal seal": page.Seal.CreatedAt,
	} {
		if value.IsZero() || value.Location() != time.UTC || value.Nanosecond()%1_000 != 0 {
			t.Fatalf("%s is not an exact UTC microsecond: %v (%v)", name, value, value.Location())
		}
	}
	if !page.SealedAt.Equal(page.Seal.CreatedAt) {
		t.Fatal("page seal time differs from terminal seal creation time")
	}
}
