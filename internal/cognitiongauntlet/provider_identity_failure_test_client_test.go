package cognitiongauntlet

import (
	"context"
	"errors"
	"sync"

	"github.com/gryph/omnidex/internal/llm"
)

type providerIdentityFailureClient struct {
	*witnessPolicyClient
	mu              sync.Mutex
	failAt          int
	identityCalls   int
	failureEvidence llm.ProviderIdentityEvidence
	successful      llm.ObservedProviderIdentity
}

func (client *providerIdentityFailureClient) ObserveProviderIdentity(
	ctx context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	client.mu.Lock()
	client.identityCalls++
	call := client.identityCalls
	client.mu.Unlock()
	if call != client.failAt {
		observed, err := client.witnessPolicyClient.ObserveProviderIdentity(ctx, request)
		if err == nil {
			owned := observed
			owned.Evidence = observed.Evidence.Clone()
			client.mu.Lock()
			client.successful = owned
			client.mu.Unlock()
		}
		return observed, err
	}
	evidence, err := failedProviderIdentityEvidence(request.Expectation)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	client.mu.Lock()
	client.failureEvidence = evidence.Clone()
	client.mu.Unlock()
	return llm.ObservedProviderIdentity{Evidence: evidence}, errors.New("recorded provider identity failure")
}

func (client *providerIdentityFailureClient) evidence() llm.ProviderIdentityEvidence {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.failureEvidence.Clone()
}

func (client *providerIdentityFailureClient) successfulObservation() llm.ObservedProviderIdentity {
	client.mu.Lock()
	defer client.mu.Unlock()
	owned := client.successful
	owned.Evidence = client.successful.Evidence.Clone()
	return owned
}

func failedProviderIdentityEvidence(
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityEvidence, error) {
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "fixture:backend", "fixture:installed", "fixture:runner",
	)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	successful, err := witnessProviderIdentityEvidence(attestation)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	operations := make([]llm.ProviderIdentityOperationEvidence, len(successful.Operations))
	for index, operation := range successful.Operations {
		disposition := llm.ProviderIdentityNotDispatched
		requestDisposition := llm.ProviderRequestNotDispatched
		status := 0
		responseComplete := false
		contentEncoding := llm.ProviderContentEncodingEvidence{}
		response := []byte(nil)
		if index == 0 {
			disposition = llm.ProviderIdentityHTTPError
			requestDisposition = llm.ProviderRequestDispatched
			status = 503
			responseComplete = true
			contentEncoding = llm.NewProviderContentEncodingEvidence(nil, false)
			response = []byte(`{"error":"identity unavailable"}`)
		}
		operations[index], err = llm.NewProviderIdentityOperationEvidence(
			operation.Operation, operation.Method, operation.Endpoint, requestDisposition,
			operation.Request, status, disposition, responseComplete, contentEncoding, response,
		)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
	}
	return llm.NewProviderIdentityEvidence(operations)
}

var _ llm.Client = (*providerIdentityFailureClient)(nil)
