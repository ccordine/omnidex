package queue

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

type cognitionFailurePolicyClient struct {
	cognitionGuardPolicyClient
	evidence llm.ProviderIdentityEvidence
}

func (client cognitionFailurePolicyClient) ObserveProviderIdentity(
	context.Context,
	llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return llm.ObservedProviderIdentity{Evidence: client.evidence},
		errors.New("provider identity test failure")
}

func TestPostgresBrainBootstrapFailureRetainsRawEvidenceBeforeEpisode(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "provider-activation-failures",
	)
	brain := fixture.Start.BrainBootstrap.AttestedBrain

	bootstrapEvidence := cognitionProviderFailureEvidence(
		t, brain, llm.ProviderIdentityTokenizer,
	)
	bootstrapOutcome, bootstrapErr := cognitionpolicy.AttestBrain(
		ctx, cognitionFailurePolicyClient{evidence: bootstrapEvidence}, brain.Ref,
	)
	if bootstrapErr == nil || bootstrapOutcome.Failure == nil {
		t.Fatalf("bootstrap outcome=%+v error=%v", bootstrapOutcome, bootstrapErr)
	}
	if err := repository.RecordCognitionBrainBootstrapFailure(
		ctx, fixture.Authority, fixture.EpisodeID, *bootstrapOutcome.Failure,
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordCognitionBrainBootstrapFailure(
		ctx, fixture.Authority, fixture.EpisodeID, *bootstrapOutcome.Failure,
	); err != nil {
		t.Fatalf("exact bootstrap failure replay: %v", err)
	}

	var episodes, failures, operations, unassociated int
	if err := pool.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_provider_activation_failures WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_provider_identity_evidence_operations operations
		  JOIN cognition_provider_activation_failures failures
		    ON failures.evidence_id=operations.evidence_id WHERE failures.episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_provider_identity_evidence evidence
		  WHERE NOT EXISTS (SELECT 1 FROM cognition_provider_activation_failures failures
		                    WHERE failures.evidence_id=evidence.evidence_id))
	`, fixture.EpisodeID).Scan(&episodes, &failures, &operations, &unassociated); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 || failures != 1 || operations != 5 || unassociated != 0 {
		t.Fatalf("episodes=%d failures=%d operations=%d unassociated=%d",
			episodes, failures, operations, unassociated)
	}

	request := CognitionProviderActivationFailurePageRequest{
		Authority: fixture.Authority, EpisodeID: fixture.EpisodeID, Limit: 1,
	}
	page, err := repository.ReadCognitionProviderActivationFailurePage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRecords != 1 || len(page.Records) != 1 ||
		page.Records[0].Bootstrap == nil || page.Records[0].Process != nil ||
		page.Records[0].SuccessfulBootstrap != nil || page.Records[0].BootstrapEvidence != nil {
		t.Fatalf("Brain bootstrap failure page=%+v", page)
	}
	for _, record := range page.Records {
		if bootstrapEvidence.Ref != record.Evidence.Ref || len(record.Evidence.Operations) != 5 {
			t.Fatalf("provider failure evidence manifest=%+v", record.Evidence)
		}
		if cognitionPayloadSHA(record.ReceiptJSON) != record.ReceiptSHA256 ||
			cognitionPayloadSHA(record.AuthorityJSON) != record.AuthoritySHA256 {
			t.Fatalf("provider failure canonical JSON is not SHA-bound: %+v", record)
		}
		receiptJSON, authorityJSON := append([]byte{}, record.ReceiptJSON...),
			append([]byte{}, record.AuthorityJSON...)
		record.ReceiptJSON[0], record.AuthorityJSON[0] = '!', '!'
		replay, replayErr := repository.ReadCognitionProviderActivationFailurePage(ctx, request)
		if replayErr != nil || len(replay.Records) != 1 ||
			!bytes.Equal(replay.Records[0].ReceiptJSON, receiptJSON) ||
			!bytes.Equal(replay.Records[0].AuthorityJSON, authorityJSON) {
			t.Fatalf("provider failure reader did not return defensive JSON copies: %+v %v", replay, replayErr)
		}
		assertProviderFailureBodies(t, repository, fixture, record.RecordID, bootstrapEvidence)
	}
}

func cognitionProviderFailureEvidence(
	t *testing.T,
	brain cognitionpolicy.AttestedBrain,
	failing llm.ProviderIdentityOperation,
) llm.ProviderIdentityEvidence {
	t.Helper()
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(brain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := queueTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 3, 0, 0, 0, time.UTC),
		brain.Attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	operations := observed.Evidence.Clone().Operations
	failed := false
	for index := range operations {
		operation := operations[index]
		if operation.Operation == failing {
			operations[index], err = llm.NewProviderIdentityOperationEvidence(
				operation.Operation, operation.Method, operation.Endpoint,
				llm.ProviderRequestDispatched, operation.Request, 200,
				llm.ProviderIdentityInvalidJSON, true,
				llm.NewProviderContentEncodingEvidence(nil, false), []byte(`{`),
			)
			failed = true
		} else if failed {
			operations[index], err = llm.NewProviderIdentityOperationEvidence(
				operation.Operation, operation.Method, operation.Endpoint,
				llm.ProviderRequestNotDispatched, operation.Request, 0,
				llm.ProviderIdentityNotDispatched, false,
				llm.ProviderContentEncodingEvidence{}, nil,
			)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
