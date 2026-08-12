package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicySealsIndeterminateProviderWriteWithoutInventedUsage(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "indeterminate provider write")
	snapshot, _ := policyTestSnapshot(t, projection)
	providerErr := errors.New("connection failed during request body write")
	client := &policyTestClient{
		err: providerErr,
		generationOverride: func(_ llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.ProviderRequestDisposition = llm.ProviderRequestWriteIndeterminate
			generation.Content = ""
			generation.ProviderHTTPStatus = 0
			generation.ProviderResponseDisposition = llm.ProviderResponseTransportError
			generation.ProviderResponseComplete = false
			generation.ProviderContentEncoding = llm.ProviderContentEncodingEvidence{}
			generation.ProviderResponseBytesKnown = false
			generation.ProviderResponseSHA256 = ""
			generation.ProviderResponseBytes = 0
			generation.ProviderResponseCaptureSHA256 = ""
			generation.ProviderResponseCapturedBytes = 0
			generation.ProviderResponseCapture = nil
			generation.ProviderResponseModel = ""
			generation.ProviderDonePresent = false
			generation.ProviderDone = false
			generation.ProviderDoneReason = ""
			generation.UsagePresent = false
			generation.Usage = llm.ProviderGenerationUsage{}
			return generation, providerErr
		},
	}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	outcome, decideErr := policy.Decide(context.Background(), snapshot)
	if !outcome.PolicyCallConsumed || !errors.Is(decideErr, ErrGeneration) {
		t.Fatalf("outcome=%+v error=%v", outcome, decideErr)
	}
	if len(journal.results) != 1 {
		t.Fatalf("terminal results=%d", len(journal.results))
	}
	result := journal.results[0]
	if result.FailureCode != CallFailureGeneration ||
		result.ProviderRequestDisposition != llm.ProviderRequestWriteIndeterminate ||
		result.ProviderResponseDisposition != llm.ProviderResponseTransportError ||
		result.ProviderUsagePresent || result.ProviderUsage != (llm.ProviderGenerationUsage{}) {
		t.Fatalf("indeterminate result=%+v", result)
	}

	replayClient := &policyTestClient{}
	replay, err := New(
		replayClient, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection),
		&policyTestCallJournal{reservation: &CallReservation{
			Attempt: journal.attempts[0], ExistingResult: &result,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	replayed, replayErr := replay.Decide(context.Background(), snapshot)
	if !errors.Is(replayErr, ErrGeneration) || replayed.PolicyCallConsumed ||
		replayClient.observationCalls != 0 {
		t.Fatalf("replay=%+v calls=%d error=%v", replayed, replayClient.observationCalls, replayErr)
	}
}
