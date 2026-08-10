package queue

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
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
	if active.EpisodeBrain != fixture.Start.AttestedBrain || active.TerminalTraceSHA256 != "" ||
		active.TotalRecords != 1 || len(active.Records) != 1 {
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
	sealed, err := repository.ReadCognitionProviderProcessObservationPage(
		ctx, fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationTerminalTrace, Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.TerminalTraceSHA256 != seal.TraceSHA256 ||
		len(sealed.Records) != 1 || sealed.Records[0].Receipt.ID != pre.ID {
		t.Fatalf("sealed provider process page=%+v", sealed)
	}
	assertSealedProviderProcessRecord(t, repository, fixture, pre)

	post := providerProcessReceiptForTest(t, fixture)
	if post.ID == pre.ID {
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
		len(after.Records) != 1 || after.Records[0].Receipt.ID != post.ID ||
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
	receipts := []cognitionpolicy.ProviderProcessObservation{
		providerProcessReceiptForTest(t, fixture), providerProcessReceiptForTest(t, fixture),
	}
	if receipts[0].ID == receipts[1].ID {
		t.Fatal("concurrent fixtures reused one fresh observation identity")
	}
	if receipts[0].Observation.ObservedAt.Before(sealCreated) ||
		receipts[1].Observation.ObservedAt.Before(sealCreated) {
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

func providerProcessReceiptForTest(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) cognitionpolicy.ProviderProcessObservation {
	t.Helper()
	receipt, err := cognitionpolicy.ObserveProviderProcess(
		fixture.Context, cognitionGuardPolicyClient{}, fixture.Start.AttestedBrain,
		cognition.EpisodeRef{ID: fixture.EpisodeID}, cognition.AttemptRef{
			JobID: fixture.Authority.JobID, Generation: fixture.Authority.Generation,
			StepID: fixture.Authority.StepID, Attempt: uint64(fixture.Authority.Attempt),
			WorkerID: fixture.Authority.WorkerID,
		}, cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func assertSealedProviderProcessRecord(
	t *testing.T,
	repository *Repository,
	fixture taskGenerationRetirementFixture,
	receipt cognitionpolicy.ProviderProcessObservation,
) {
	t.Helper()
	page, err := repository.ReadCognitionSealedTrace(
		fixture.Context, fixture.EpisodeID,
		CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range page.Records {
		if record.Kind == "provider_process_observation" && record.ID == receipt.ID {
			return
		}
	}
	t.Fatalf("sealed trace omitted provider process observation %q", receipt.ID)
}
