package queue

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/jackc/pgx/v5"
)

func TestPostgresProviderProcessObservationsSealThenAppendAuditChain(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "provider-process-chain",
	)
	pre := providerProcessReceiptForTest(t, fixture)
	if err := repository.RecordCognitionProviderProcessObservation(ctx, pre); err != nil {
		t.Fatalf("record preterminal observation: %v", err)
	}
	if err := repository.RecordCognitionProviderProcessObservation(ctx, pre); err != nil {
		t.Fatalf("replay exact preterminal observation: %v", err)
	}
	active, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationTerminalTrace, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if active.EpisodeBrain != fixture.Start.BrainBootstrap.AttestedBrain || active.TerminalTraceSHA256 != "" ||
		active.TotalRecords != 2 || len(active.Records) != 2 {
		t.Fatalf("active provider process page=%+v", active)
	}
	if _, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationPostSealAudit, Limit: 1,
		},
	); err == nil {
		t.Fatal("active episode exposed a post-seal provider observation page")
	}

	command := cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure"))
	seal, err := repository.CancelCognitionEpisode(ctx, command)
	if err != nil {
		t.Fatalf("seal episode with process observation: %v", err)
	}
	assertProviderObservationCannotCrossSeal(t, pool, ctx, fixture.EpisodeID, seal.TraceSHA256, pre)
	sealed, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationTerminalTrace, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.TerminalTraceSHA256 != seal.TraceSHA256 ||
		len(sealed.Records) != 2 || sealed.Records[1].Activation.Receipt.ID != pre.Receipt.ID {
		t.Fatalf("sealed provider process page=%+v", sealed)
	}
	assertSealedProviderProcessRecord(t, repository, fixture, pre)

	post := providerProcessReceiptForTest(t, fixture)
	if post.Receipt.ID == pre.Receipt.ID {
		t.Fatal("fresh terminal replay observation reused preterminal identity")
	}
	if err := repository.RecordCognitionProviderProcessObservation(ctx, post); err != nil {
		t.Fatalf("append post-seal observation: %v", err)
	}
	if err := repository.RecordCognitionProviderProcessObservation(ctx, post); err != nil {
		t.Fatalf("replay exact post-seal observation: %v", err)
	}
	after, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationPostSealAudit, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if after.TerminalTraceSHA256 != seal.TraceSHA256 ||
		len(after.Records) != 1 || after.Records[0].Activation.Receipt.ID != post.Receipt.ID ||
		after.PostSealAuditHeadSHA256 != after.Records[0].ChainSHA256 {
		t.Fatalf("post-seal provider process page=%+v", after)
	}
	var persistedTrace string
	if err := pool.QueryRow(ctx, `SELECT trace_sha256 FROM cognition_terminal_seals WHERE episode_id=$1`,
		fixture.EpisodeID).Scan(&persistedTrace); err != nil {
		t.Fatal(err)
	}
	if persistedTrace != seal.TraceSHA256 {
		t.Fatal("post-seal audit append changed the immutable terminal trace")
	}
}

func assertProviderObservationCannotCrossSeal(
	t *testing.T,
	pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	ctx context.Context,
	episodeID cognition.EpisodeID,
	traceSHA256 string,
	activation cognitionpolicy.ProviderProcessActivation,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_provider_postseal_observations (
			observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,
			worker_id,purpose,sequence,source_kind,terminal_trace_sha256,previous_chain_sha256,chain_sha256,
			stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,provider_attestation_sha256,challenge_sha256,
			observed_at,receipt_json,receipt_sha256
		)
		SELECT observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,
		       worker_id,purpose,1,'direct_audit',$2,$2,
		       encode(digest($2||':'||$2||':1:direct_audit:'||receipt_sha256,'sha256'),'hex'),
		       stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
		       provider_observation_json,provider_observation_json_sha256,
		       provider_observation_sha256,provider_attestation_sha256,challenge_sha256,
		       observed_at,receipt_json,receipt_sha256
		FROM cognition_provider_process_observations
		WHERE episode_id=$1 AND observation_id=$3
	`, episodeID, traceSHA256, activation.Receipt.ID)
	if err != nil {
		t.Fatalf("stage cross-seal process observation duplicate: %v", err)
	}
	_, err = tx.Exec(ctx,
		"SET CONSTRAINTS cognition_provider_postseal_observation_cross_table_unique IMMEDIATE",
	)
	if err == nil || !strings.Contains(err.Error(), "already exists pre-seal") {
		t.Fatalf("cross-seal observation identity error=%v", err)
	}
}

func TestPostgresProviderProcessPostSealConcurrentAppendIsContiguous(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "provider-process-concurrent",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	var sealCreated time.Time
	if err := pool.QueryRow(ctx,
		`SELECT created_at FROM cognition_terminal_seals WHERE episode_id=$1`, fixture.EpisodeID,
	).Scan(&sealCreated); err != nil {
		t.Fatal(err)
	}
	receipts := []cognitionpolicy.ProviderProcessActivation{
		providerProcessReceiptForTest(t, fixture), providerProcessReceiptForTest(t, fixture),
	}
	if receipts[0].Receipt.ID == receipts[1].Receipt.ID {
		t.Fatal("concurrent fixtures reused one fresh observation identity")
	}
	if receipts[0].Receipt.Observation.ObservedAt.Before(sealCreated) ||
		receipts[1].Receipt.Observation.ObservedAt.Before(sealCreated) {
		t.Fatal("post-seal observation predates the immutable seal")
	}
	start := make(chan struct{})
	errorsOut := make(chan error, len(receipts))
	var ready sync.WaitGroup
	ready.Add(len(receipts))
	for _, receipt := range receipts {
		receipt := receipt
		go func() {
			ready.Done()
			<-start
			errorsOut <- repository.RecordCognitionProviderProcessObservation(ctx, receipt)
		}()
	}
	ready.Wait()
	close(start)
	for range receipts {
		if err := <-errorsOut; err != nil {
			t.Fatalf("concurrent provider process append: %v", err)
		}
	}
	page, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationPostSealAudit, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 || page.Records[0].Sequence != 1 || page.Records[1].Sequence != 2 {
		t.Fatalf("concurrent post-seal chain=%+v", page.Records)
	}
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM cognition_provider_postseal_observations WHERE episode_id=$1`,
		fixture.EpisodeID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("postseal count=%d error=%v", count, err)
	}
}

func TestPostgresProviderProcessObservationPagesRemainBounded(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "provider-process-pages",
	)
	if _, err := repository.CancelCognitionEpisode(
		ctx, cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure")),
	); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxCognitionProviderObservationPageSize+1; index++ {
		if err := repository.RecordCognitionProviderProcessObservation(
			ctx, providerProcessReceiptForTest(t, fixture),
		); err != nil {
			t.Fatalf("append post-seal observation %d: %v", index, err)
		}
	}
	first, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationPostSealAudit,
			Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalRecords != int64(MaxCognitionProviderObservationPageSize+1) ||
		len(first.Records) != MaxCognitionProviderObservationPageSize ||
		first.NextSequence != int64(MaxCognitionProviderObservationPageSize) {
		t.Fatalf("first provider observation page=%+v", first)
	}
	second, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope:         CognitionProviderObservationPostSealAudit,
			AfterSequence: first.NextSequence, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].Sequence != first.NextSequence+1 ||
		second.PreviousChainSHA256 != first.Records[len(first.Records)-1].ChainSHA256 ||
		second.PostSealAuditHeadSHA256 != first.PostSealAuditHeadSHA256 {
		t.Fatalf("second provider observation page=%+v", second)
	}
}
