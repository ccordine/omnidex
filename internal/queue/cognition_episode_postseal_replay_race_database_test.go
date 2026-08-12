package queue

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReplayAndSealRaceHasOneLockedEvidenceBoundary(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "replay-seal-race",
	)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	fixture.Authority = replacement
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	cancellation := cognitionCancellationForTest(
		t, fixture, errors.New("bounded policy failure"),
	)
	start := make(chan struct{})
	replayDone := make(chan error, 1)
	sealDone := make(chan error, 1)
	go func() {
		<-start
		_, err := repository.StartCognitionEpisode(ctx, replay, cognitionTestFactAuthority())
		replayDone <- err
	}()
	go func() {
		<-start
		_, err := repository.CancelCognitionEpisode(ctx, cancellation)
		sealDone <- err
	}()
	close(start)
	if err := <-replayDone; err != nil {
		t.Fatalf("racing episode replay: %v", err)
	}
	if err := <-sealDone; err != nil {
		t.Fatalf("racing episode seal: %v", err)
	}
	sealedBeforeRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact replay after replay/seal race: %v", err)
	}
	if sealedAfterRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID); !bytes.Equal(sealedAfterRetry, sealedBeforeRetry) {
		t.Fatal("exact replay after race changed the byte-exact sealed trace")
	}
	if _, err := repository.ReadCognitionSealedTrace(
		ctx, fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	); err != nil {
		t.Fatalf("read trace after replay/seal race: %v", err)
	}

	var traceReplay, postSealAudit, activeProcess, postSealProcess, sealedReplay int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence
		 WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits
		 WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_postseal_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_terminal_seals seals,
		 jsonb_array_elements(seals.trace_json::jsonb->'records') record
		 WHERE seals.episode_id=$1 AND record->>'kind'='provider_brain_bootstrap'
		   AND record->>'phase'='2')`, fixture.EpisodeID).Scan(
		&traceReplay, &postSealAudit, &activeProcess, &postSealProcess, &sealedReplay,
	); err != nil {
		t.Fatal(err)
	}
	if traceReplay+postSealAudit != 1 || traceReplay != sealedReplay ||
		activeProcess != 1+traceReplay || postSealProcess != postSealAudit {
		t.Fatalf("trace-replay/audit/active-process/postseal-process/sealed-replay=%d/%d/%d/%d/%d",
			traceReplay, postSealAudit, activeProcess, postSealProcess, sealedReplay)
	}
	assertCognitionReplayProcessLinksExact(t, pool, fixture.EpisodeID)
}

func TestPostgresActiveReplayRetryAfterSealIsOneExactInvocation(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "active-replay-retry-after-seal",
	)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	fixture.Authority = replacement
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	sealedBeforeRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	if _, err := repository.StartCognitionEpisode(
		ctx, replay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact active replay retry after seal: %v", err)
	}
	if sealedAfterRetry := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID); !bytes.Equal(sealedAfterRetry, sealedBeforeRetry) {
		t.Fatal("exact active replay retry changed the byte-exact sealed trace")
	}
	assertOneCognitionReplayInvocationForTest(t, pool, fixture.EpisodeID)
	assertCognitionReplayProcessLinksExact(t, pool, fixture.EpisodeID)

	changedProcess := replay
	changedProcess.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, fixture.Authority,
		replay.BrainBootstrap.AttestedBrain,
	)
	if _, err := repository.StartCognitionEpisode(
		ctx, changedProcess, cognitionTestFactAuthority(),
	); !errors.Is(err, ErrCognitionConflict) ||
		!strings.Contains(err.Error(), "bootstrap authority changed") {
		t.Fatalf("changed process half of sealed replay error=%v", err)
	}
	assertOneCognitionReplayInvocationForTest(t, pool, fixture.EpisodeID)
}

func assertCognitionReplayProcessLinksExact(
	t *testing.T, pool *pgxpool.Pool, episodeID cognition.EpisodeID,
) {
	t.Helper()
	var activeExact, postSealExact int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence replay
		 JOIN cognition_provider_process_observations process
		   ON process.observation_id=replay.process_observation_id
		 WHERE replay.episode_id=$1 AND replay.process_receipt_sha256=process.receipt_sha256
		   AND replay.process_evidence_id=process.evidence_id),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits audit
		 JOIN cognition_provider_postseal_observations process
		   ON process.observation_id=audit.process_observation_id
		 WHERE audit.episode_id=$1 AND process.source_kind='episode_replay'
		   AND audit.terminal_trace_sha256=process.terminal_trace_sha256
		   AND audit.process_chain_sha256=process.chain_sha256)`, episodeID,
	).Scan(&activeExact, &postSealExact); err != nil {
		t.Fatal(err)
	}
	if activeExact+postSealExact != 1 {
		t.Fatalf("exact active/postseal replay process links=%d/%d", activeExact, postSealExact)
	}
}

func assertOneCognitionReplayInvocationForTest(
	t *testing.T, pool *pgxpool.Pool, episodeID cognition.EpisodeID,
) {
	t.Helper()
	var traceReplay, postSealAudit, activeProcess, postSealProcess int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence
		 WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits
		 WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_postseal_observations WHERE episode_id=$1)`,
		episodeID,
	).Scan(&traceReplay, &postSealAudit, &activeProcess, &postSealProcess); err != nil {
		t.Fatal(err)
	}
	if traceReplay+postSealAudit != 1 || activeProcess != 1+traceReplay ||
		postSealProcess != postSealAudit {
		t.Fatalf("trace-replay/audit/active-process/postseal-process=%d/%d/%d/%d",
			traceReplay, postSealAudit, activeProcess, postSealProcess)
	}
}

func TestPostgresPostSealObservationSourceRequiresMatchingReplayAudit(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "postseal-source-totality",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	direct := providerProcessReceiptForTest(t, fixture)
	if err := repository.RecordCognitionProviderProcessObservation(ctx, direct); err != nil {
		t.Fatalf("record direct post-seal audit: %v", err)
	}
	page, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationPostSealAudit, Limit: 1,
		},
	)
	if err != nil || len(page.Records) != 1 ||
		page.Records[0].PostSealSource != CognitionProviderPostSealDirectAudit {
		t.Fatalf("direct post-seal page=%+v error=%v", page, err)
	}

	orphan := providerProcessReceiptForTest(t, fixture)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		ctx, tx, orphan.IdentityEvidence,
	); err != nil {
		t.Fatal(err)
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, fixture.EpisodeID, true)
	if err != nil || !found {
		t.Fatalf("load terminal episode found=%v error=%v", found, err)
	}
	receiptJSON, err := exactjson.Canonical(orphan.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertPostSealProviderProcessObservationTx(
		ctx, tx, fixture.Authority, episode, orphan,
		CognitionProviderPostSealEpisodeReplay, receiptJSON, cognitionPayloadSHA(receiptJSON),
	); err != nil {
		t.Fatal(err)
	}
	err = tx.Commit(ctx)
	if err == nil || !strings.Contains(err.Error(), "bootstrap audit totality") {
		t.Fatalf("orphan episode-replay process observation error=%v", err)
	}
}
