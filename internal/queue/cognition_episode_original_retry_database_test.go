package queue

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPostgresOriginalCognitionStartExactRetryIsOneInvocationAcrossSeal(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "original-start-exact-retry",
	)
	if _, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact original start retry while active: %v", err)
	}
	assertOriginalCognitionStartInvocationCounts(t, fixture, 0, 0)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	sealedBeforeRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	if _, err := repository.StartCognitionEpisode(
		ctx, fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact original start retry after seal: %v", err)
	}
	if sealedAfterRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID); !bytes.Equal(sealedAfterRetry, sealedBeforeRetry) {
		t.Fatal("exact original start retry changed the byte-exact sealed trace")
	}
	assertOriginalCognitionStartInvocationCounts(t, fixture, 0, 0)

	changedProcess := fixture.Start
	changedProcess.ProviderProcessActivation = providerProcessReceiptForTest(t, fixture)
	if _, err := repository.StartCognitionEpisode(
		ctx, changedProcess, cognitionTestFactAuthority(),
	); !errors.Is(err, ErrCognitionConflict) ||
		!strings.Contains(err.Error(), "original invocation") {
		t.Fatalf("changed process half of original invocation error=%v", err)
	}
	assertOriginalCognitionStartInvocationCounts(t, fixture, 0, 0)
}

func assertOriginalCognitionStartInvocationCounts(
	t *testing.T, fixture taskGenerationRetirementFixture, replayRows, postSealRows int,
) {
	t.Helper()
	var initialBootstrap, activeProcess, replayBootstrap, postSealAudit, postSealProcess int
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
		(SELECT COUNT(*) FROM cognition_episode_provider_identity_evidence WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_postseal_observations WHERE episode_id=$1)`,
		fixture.EpisodeID,
	).Scan(
		&initialBootstrap, &activeProcess, &replayBootstrap, &postSealAudit, &postSealProcess,
	); err != nil {
		t.Fatal(err)
	}
	if initialBootstrap != 1 || activeProcess != 1 || replayBootstrap != replayRows ||
		postSealAudit != postSealRows || postSealProcess != postSealRows {
		t.Fatalf("initial/process/replay/audit/postseal=%d/%d/%d/%d/%d",
			initialBootstrap, activeProcess, replayBootstrap, postSealAudit, postSealProcess)
	}
}
