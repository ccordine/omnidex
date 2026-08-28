package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/model"
)

type persistedGapRejectingProvider struct{ startupTestLLM }

func (persistedGapRejectingProvider) RequireExactPreparedContract() error {
	return errors.New("persisted gap contract rejection")
}

func (persistedGapRejectingProvider) DiscoverProviderIdentityEvidence(
	context.Context, llm.ProviderIdentitySelection, string,
) (llm.ObservedProviderIdentity, error) {
	panic("provider discovery must not run after contract rejection")
}

func TestPostgresNamedGapResolvesLazyAbsentProviderAfterPersistence(t *testing.T) {
	ctx, repository, pool := openRepositoryTestDatabase(t)
	marker := "lazy-provider-gap-" + time.Now().UTC().Format("20060102150405.000000000")
	jobRecord, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}

	transports := llmprovider.NewLazyFromConfig(config.Config{
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	opts := validWorkerOptions()
	opts.EmbeddingProvider = ""
	opts.EmbeddingModel = ""
	service, err := New(repository, transports.Stations, transports.Embeddings, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := assemblyline.NewRepositorySearchAnchorCoverageJob(
		assemblyline.RepositorySearchAnchorLeafInput{
			UnresolvedConcept: "registered owner", AcceptedAnchors: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.executeExactPortableStation(
		ctx, claim.Authority, portable, "configured-station-model",
	)
	if err == nil || !strings.Contains(err.Error(), "LLM_PROVIDER is not configured") {
		t.Fatalf("station error=%v, want absent provider authority", err)
	}

	var gaps, discoveries, receipts, captures, outcomes, calls, evidence, dispatched int
	err = pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM station_gap_openings WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discoveries WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discovery_receipts WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discovery_captures c
		   JOIN station_provider_discoveries d ON d.id=c.opening_id WHERE d.job_id=$1),
		 (SELECT count(*) FROM station_gap_outcomes WHERE job_id=$1),
		 (SELECT count(*) FROM station_call_openings WHERE job_id=$1),
		 (SELECT count(*) FROM llm_call_evidence WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discovery_captures c
		   JOIN station_provider_discoveries d ON d.id=c.opening_id
		   WHERE d.job_id=$1 AND c.request_disposition<>'not_dispatched')
	`, jobRecord.ID).Scan(
		&gaps, &discoveries, &receipts, &captures, &outcomes, &calls, &evidence, &dispatched,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gaps != 1 || discoveries != 1 || receipts != 1 || captures != 5 || outcomes != 1 ||
		calls != 0 || evidence != 0 || dispatched != 0 {
		t.Fatalf("gap/discovery/receipt/capture/outcome/call/evidence/dispatched=%d/%d/%d/%d/%d/%d/%d/%d",
			gaps, discoveries, receipts, captures, outcomes, calls, evidence, dispatched)
	}
}

func TestPostgresExactContractRejectionOccursAfterPersistedGap(t *testing.T) {
	ctx, repository, pool := openRepositoryTestDatabase(t)
	marker := "persisted-contract-gap-" + time.Now().UTC().Format("20060102150405.000000000")
	jobRecord, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}
	service, err := New(
		repository, persistedGapRejectingProvider{}, startupTestLLM{}, nil, validWorkerOptions(),
	)
	if err != nil {
		t.Fatalf("worker construction validated provider early: %v", err)
	}
	portable, err := assemblyline.NewRepositorySearchAnchorCoverageJob(
		assemblyline.RepositorySearchAnchorLeafInput{
			UnresolvedConcept: "registered owner", AcceptedAnchors: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.executeExactPortableStation(
		ctx, claim.Authority, portable, "configured-station-model",
	)
	if err == nil || !strings.Contains(err.Error(), "persisted gap contract rejection") {
		t.Fatalf("station error=%v", err)
	}
	var gaps, discoveries, receipts, outcomes, calls, dispatched int
	if err := pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM station_gap_openings WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discoveries WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discovery_receipts WHERE job_id=$1),
		 (SELECT count(*) FROM station_gap_outcomes WHERE job_id=$1),
		 (SELECT count(*) FROM station_call_openings WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discovery_captures c
		   JOIN station_provider_discoveries d ON d.id=c.opening_id
		   WHERE d.job_id=$1 AND c.request_disposition<>'not_dispatched')
	`, jobRecord.ID).Scan(&gaps, &discoveries, &receipts, &outcomes, &calls, &dispatched); err != nil {
		t.Fatal(err)
	}
	if gaps != 1 || discoveries != 1 || receipts != 1 || outcomes != 1 || calls != 0 || dispatched != 0 {
		t.Fatalf("gap/discovery/receipt/outcome/call/dispatched=%d/%d/%d/%d/%d/%d",
			gaps, discoveries, receipts, outcomes, calls, dispatched)
	}
}
