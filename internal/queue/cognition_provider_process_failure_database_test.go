package queue

import (
	"bytes"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresProcessFailureRetainsSuccessfulBootstrapAndRawFailure(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "provider-process-failure-evidence",
	)
	bootstrap := fixture.Start.BrainBootstrap
	failureEvidence := cognitionProviderFailureEvidence(
		t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload,
	)
	failure := cognitionProviderProcessFailure(
		t, fixture, failureEvidence,
	)
	if err := repository.RecordCognitionProviderProcessFailure(ctx, bootstrap, failure); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordCognitionProviderProcessFailure(ctx, bootstrap, failure); err != nil {
		t.Fatalf("exact process failure replay: %v", err)
	}

	changedBootstrap := cognitionChangedBrainBootstrap(t, bootstrap)
	if err := repository.RecordCognitionProviderProcessFailure(
		ctx, changedBootstrap, failure,
	); err == nil {
		t.Fatal("process failure replay accepted changed successful bootstrap evidence")
	}
	changedFailure := cognitionProviderProcessFailure(t, fixture,
		cognitionProviderFailureEvidence(t, bootstrap.AttestedBrain, llm.ProviderIdentityTokenizer),
	)
	if err := repository.RecordCognitionProviderProcessFailure(
		ctx, bootstrap, changedFailure,
	); err == nil {
		t.Fatal("process invocation accepted a second changed failure")
	}

	var episodes, failures, operations, calls, observations, unassociated int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_identity_evidence_operations operations
		 JOIN cognition_provider_activation_failures failures
		   ON operations.evidence_id IN (failures.evidence_id,failures.bootstrap_evidence_id)
		 WHERE failures.episode_id=$1),
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_process_observations WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_identity_evidence evidence
		 WHERE NOT EXISTS (
		   SELECT 1 FROM cognition_provider_activation_failures failures
		   WHERE evidence.evidence_id IN (failures.evidence_id,failures.bootstrap_evidence_id)
		 ))`, fixture.EpisodeID).Scan(
		&episodes, &failures, &operations, &calls, &observations, &unassociated,
	); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 || failures != 1 || operations != 10 || calls != 0 ||
		observations != 0 || unassociated != 0 {
		t.Fatalf("episode/failure/operations/calls/observations/unassociated=%d/%d/%d/%d/%d/%d",
			episodes, failures, operations, calls, observations, unassociated)
	}

	page, err := repository.ReadCognitionProviderActivationFailurePage(
		ctx, CognitionProviderActivationFailurePageRequest{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID, Limit: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRecords != 1 || len(page.Records) != 1 {
		t.Fatalf("process failure page=%+v", page)
	}
	record := page.Records[0]
	if record.Process == nil || record.Bootstrap != nil || record.SuccessfulBootstrap == nil ||
		*record.SuccessfulBootstrap != bootstrap.AttestedBrain || record.BootstrapEvidence == nil ||
		record.BootstrapEvidence.Ref != bootstrap.BootstrapEvidence.Ref ||
		record.Evidence.Ref != failureEvidence.Ref {
		t.Fatalf("process failure record=%+v", record)
	}
	for _, evidence := range []llm.ProviderIdentityEvidence{
		bootstrap.BootstrapEvidence, failureEvidence,
	} {
		assertProviderFailureBodies(t, repository, fixture, record.RecordID, evidence)
	}
}

func cognitionProviderProcessFailure(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	evidence llm.ProviderIdentityEvidence,
) cognitionpolicy.ProviderProcessFailure {
	t.Helper()
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		t.Context(), cognitionFailurePolicyClient{evidence: evidence},
		fixture.Start.BrainBootstrap.AttestedBrain,
		cognition.EpisodeRef{ID: fixture.EpisodeID}, cognitionAttempt(fixture.Authority),
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err == nil || outcome.Failure == nil {
		t.Fatalf("process failure outcome=%+v error=%v", outcome, err)
	}
	return *outcome.Failure
}

func cognitionChangedBrainBootstrap(
	t *testing.T,
	bootstrap cognitionpolicy.BrainBootstrap,
) cognitionpolicy.BrainBootstrap {
	t.Helper()
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(bootstrap.AttestedBrain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := queueTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 3, 1, 0, 0, time.UTC),
		bootstrap.AttestedBrain.Attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewAttestedBrain(
		bootstrap.AttestedBrain.Ref, observed.Attestation, observed.Observation,
		bootstrap.AttestedBrain.Host,
	)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := cognitionpolicy.NewBrainBootstrap(brain, observed.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func assertProviderFailureBodies(
	t *testing.T,
	repository *Repository,
	fixture taskGenerationRetirementFixture,
	recordID string,
	evidence llm.ProviderIdentityEvidence,
) {
	t.Helper()
	for index, operation := range evidence.Operations {
		for _, body := range []struct {
			kind CognitionProviderIdentityBodyKind
			want []byte
		}{
			{CognitionProviderIdentityRequestBody, operation.Request},
			{CognitionProviderIdentityResponseBody, operation.ResponseCapture},
		} {
			page, err := repository.ReadCognitionProviderActivationFailureBody(
				t.Context(), CognitionProviderActivationFailureBodyRequest{
					Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
					RecordID: recordID, EvidenceID: evidence.Ref.ID,
					OperationIndex: index, Kind: body.kind,
					Limit: MaxCognitionPolicyEvidencePageBytes,
				},
			)
			if err != nil || page.TotalBytes != len(body.want) ||
				page.NextOffset != len(body.want) || !bytes.Equal(page.Content, body.want) {
				t.Fatalf("body kind=%s operation=%d page=%+v error=%v", body.kind, index, page, err)
			}
		}
	}
}
