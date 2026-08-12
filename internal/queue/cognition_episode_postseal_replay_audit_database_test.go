package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestPostgresTerminalReplayBootstrapIsSeparateFromSealedTrace(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "postseal-replay-audit",
	)
	activeActor := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	activeReplay := cognitionReplayStartForTest(t, fixture.Start, activeActor)
	if _, err := repository.StartCognitionEpisode(
		ctx, activeReplay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("active episode replay: %v", err)
	}
	fixture.Authority = activeActor
	seal, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	)
	if err != nil {
		t.Fatalf("seal replayed episode: %v", err)
	}

	beforeRaw := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	beforePage := cognitionSealedTraceForTest(t, repository, fixture.EpisodeID)
	activeReplayID := cognitionReplayBootstrapIDForTest(t, pool, fixture.EpisodeID, activeActor.Attempt)
	assertCognitionBootstrapSourceForTest(
		t, beforePage, CognitionBrainBootstrapEpisodeReplay, activeReplayID,
	)

	terminalActor := replaceCognitionAttemptForTest(t, pool, activeActor)
	terminalReplay := cognitionReplayStartForTest(t, fixture.Start, terminalActor)
	replayed, err := repository.StartCognitionEpisode(
		ctx, terminalReplay, cognitionTestFactAuthority(),
	)
	if err != nil {
		t.Fatalf("terminal episode replay: %v", err)
	}
	if replayed.Status != CognitionEpisodeCanceled || replayed.TerminalOutcome == "" {
		t.Fatalf("terminal replay episode=%+v", replayed)
	}

	afterRaw := cognitionTerminalTraceBytesForTest(t, pool, fixture.EpisodeID)
	afterPage := cognitionSealedTraceForTest(t, repository, fixture.EpisodeID)
	if !bytes.Equal(afterRaw, beforeRaw) || !reflect.DeepEqual(afterPage, beforePage) ||
		afterPage.TraceSHA256 != seal.TraceSHA256 {
		t.Fatal("terminal invocation changed the byte-exact sealed cognition trace")
	}
	assertCognitionTerminalReplayAuditForTest(
		t, pool, fixture.EpisodeID, terminalActor.Attempt, seal.TraceSHA256,
		terminalReplay.BrainBootstrap.BootstrapEvidence.Ref.ID,
		terminalReplay.ProviderProcessActivation.Receipt.ID,
		terminalReplay.ProviderProcessActivation.IdentityEvidence.Ref.ID,
	)

	if _, err := repository.StartCognitionEpisode(
		ctx, terminalReplay, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact terminal episode replay: %v", err)
	}
	assertCognitionTerminalReplayAuditForTest(
		t, pool, fixture.EpisodeID, terminalActor.Attempt, seal.TraceSHA256,
		terminalReplay.BrainBootstrap.BootstrapEvidence.Ref.ID,
		terminalReplay.ProviderProcessActivation.Receipt.ID,
		terminalReplay.ProviderProcessActivation.IdentityEvidence.Ref.ID,
	)
}

func TestPostgresReplayBootstrapTraceTableRejectsTerminalEpisode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "terminal-replay-table-guard",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := cognitionReplayStartForTest(t, fixture.Start, replacement)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	err = insertCognitionEpisodeReplayBootstrapEvidenceTx(ctx, tx, replay)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err == nil || !strings.Contains(err.Error(), "active episode") {
		t.Fatalf("terminal insert into trace-bound replay table error=%v", err)
	}
}

func cognitionReplayStartForTest(
	t *testing.T,
	original CognitionEpisodeStart,
	authority model.StepAttemptAuthority,
) CognitionEpisodeStart {
	t.Helper()
	replay := original
	replay.Authority = authority
	replay.BrainBootstrap = freshReplayBrainBootstrap(t, original.BrainBootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, t.Context(), original.EpisodeID, authority,
		replay.BrainBootstrap.AttestedBrain,
	)
	return replay
}

func cognitionTerminalTraceBytesForTest(
	t *testing.T,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	episodeID cognition.EpisodeID,
) []byte {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT trace_json FROM cognition_terminal_seals WHERE episode_id=$1`, episodeID,
	).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func cognitionSealedTraceForTest(
	t *testing.T, repository *Repository, episodeID cognition.EpisodeID,
) CognitionSealedTracePage {
	t.Helper()
	page, err := repository.ReadCognitionSealedTrace(
		t.Context(), episodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatalf("read sealed cognition trace: %v", err)
	}
	if page.NextOffset != -1 || len(page.Records) != page.TotalRecords {
		t.Fatalf("test fixture trace was not one complete bounded page: %+v", page)
	}
	return page
}

func cognitionReplayBootstrapIDForTest(
	t *testing.T,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	episodeID cognition.EpisodeID,
	attempt int64,
) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `SELECT replay_id
		FROM cognition_episode_replay_provider_identity_evidence
		WHERE episode_id=$1 AND step_attempt=$2`, episodeID, attempt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertCognitionBootstrapSourceForTest(
	t *testing.T, page CognitionSealedTracePage,
	source CognitionBrainBootstrapTraceSource, id string,
) {
	t.Helper()
	for _, record := range page.Records {
		if record.Kind != CognitionTraceKindProviderBrainBootstrap || record.ID != id {
			continue
		}
		var payload CognitionBrainBootstrapTrace
		if err := json.Unmarshal(record.Payload, &payload); err != nil ||
			payload.Validate() != nil || payload.Source != source {
			t.Fatalf("bootstrap source payload=%s error=%v", record.Payload, err)
		}
		return
	}
	t.Fatalf("sealed trace omitted bootstrap source %s/%s", source, id)
}

func assertCognitionTerminalReplayAuditForTest(
	t *testing.T,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	episodeID cognition.EpisodeID,
	attempt int64,
	traceSHA, bootstrapEvidenceID, processObservationID, processEvidenceID string,
) {
	t.Helper()
	var oldRows, auditRows, processRows, exactLinks int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT COUNT(*) FROM cognition_episode_replay_provider_identity_evidence
		 WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits
		 WHERE episode_id=$1 AND step_attempt=$2),
		(SELECT COUNT(*) FROM cognition_provider_postseal_observations
		 WHERE episode_id=$1 AND step_attempt=$2),
		(SELECT COUNT(*) FROM cognition_episode_postseal_replay_bootstrap_audits audits
		 JOIN cognition_provider_postseal_observations observations
		   ON observations.observation_id=audits.process_observation_id
		 WHERE audits.episode_id=$1 AND audits.step_attempt=$2
		   AND audits.terminal_trace_sha256=$3 AND audits.evidence_id=$4
		   AND audits.process_observation_id=$5
		   AND observations.evidence_id=$6
		   AND audits.process_chain_sha256=observations.chain_sha256
		   AND ROW(audits.episode_id,audits.job_id,audits.generation,audits.step_id,
		           audits.step_attempt,audits.worker_id)
		       =ROW(observations.episode_id,observations.job_id,observations.generation,
		           observations.step_id,observations.step_attempt,observations.worker_id))`,
		episodeID, attempt, traceSHA, bootstrapEvidenceID,
		processObservationID, processEvidenceID,
	).Scan(&oldRows, &auditRows, &processRows, &exactLinks); err != nil {
		t.Fatal(err)
	}
	if oldRows != 1 || auditRows != 1 || processRows != 1 || exactLinks != 1 {
		t.Fatalf("trace-replay/audit/process/exact-link=%d/%d/%d/%d",
			oldRows, auditRows, processRows, exactLinks)
	}
}
