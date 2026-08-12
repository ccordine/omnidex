package cognitionpolicy

import (
	"context"
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type policyTestClient struct {
	response               string
	err                    error
	generateCalls          int
	legacyPreparedCalls    int
	plainGenerateCalls     int
	prepareCalls           int
	cleanupCalls           int
	otherCalls             int
	model                  string
	prompt                 string
	prompts                []string
	prepared               []llm.PreparedModel
	changePreparedIdentity bool
	mutateContract         bool
	attestations           []llm.ProviderIdentityAttestation
	attestationErr         error
	observationCalls       int
	generationOverride     func(llm.PreparedModel, llm.PreparedGeneration) (llm.PreparedGeneration, error)
	exactOverride          func(llm.PreparedModel) (llm.PreparedGeneration, error)
}

func (client *policyTestClient) Generate(context.Context, string, string) (string, error) {
	client.plainGenerateCalls++
	return "", errors.New("plain Generate must not be called by cognition policy")
}

func (client *policyTestClient) PrepareContextModel(
	_ context.Context,
	model string,
	prompt string,
) (llm.PreparedModel, error) {
	client.prepareCalls++
	prepared := llm.PreparedModel{BaseModel: model, ContextModel: model, Prompt: prompt}
	if client.changePreparedIdentity {
		prepared.ContextModel = "changed-model"
	}
	return prepared, nil
}

func (client *policyTestClient) GeneratePrepared(
	context.Context,
	llm.PreparedModel,
) (string, error) {
	client.legacyPreparedCalls++
	return "", errors.New("legacy GeneratePrepared must not be called by cognition policy")
}

func (client *policyTestClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.observationCalls++
	if client.exactOverride != nil {
		return client.exactOverride(prepared)
	}
	if client.attestationErr != nil {
		return llm.PreparedGeneration{}, client.attestationErr
	}
	expected := *prepared.ProviderIdentityExpectation
	attestation, err := client.testAttestation(expected)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	call := client.observationCalls
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 21, 0, call, 0, time.UTC), attestation,
		prepared.ProviderObservationChallenge,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	if err := attestation.ValidateFor(expected); err != nil {
		return llm.PreparedGeneration{
			Schema:                     llm.PreparedGenerationSchemaV1,
			ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
			ProviderObservation:        observed.Observation,
			ProviderIdentityEvidence:   observed.Evidence,
		}, err
	}
	client.generateCalls++
	client.model, client.prompt = prepared.ContextModel, prepared.Prompt
	client.prompts = append(client.prompts, prepared.Prompt)
	client.prepared = append(client.prepared, prepared)
	if client.mutateContract {
		prepared.ResponseSchema["type"] = "array"
	}
	rawResponse := policyTestRawProviderResponse(prepared.ContextModel, client.response)
	generation := llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    client.response,
		ProviderRequestSHA256:      mustPolicyTestRequestSHA(prepared),
		ProviderHTTPStatus:         200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        policySHA256(string(rawResponse)),
		ProviderResponseBytes:         int64(len(rawResponse)),
		ProviderResponseCaptureSHA256: policySHA256(string(rawResponse)),
		ProviderResponseCapturedBytes: len(rawResponse),
		ProviderResponseCapture:       rawResponse,
		ProviderResponseModel:         prepared.ContextModel,
		ProviderDonePresent:           true,
		ProviderDone:                  true,
		ProviderDoneReason:            "stop",
		UsagePresent:                  true, Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		}, ProviderObservation: observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}
	if client.generationOverride != nil {
		return client.generationOverride(prepared, generation)
	}
	if client.err != nil {
		return llm.PreparedGeneration{
			Schema:                      llm.PreparedGenerationSchemaV1,
			ProviderRequestDisposition:  llm.ProviderRequestDispatched,
			ProviderRequestSHA256:       mustPolicyTestRequestSHA(prepared),
			ProviderResponseDisposition: llm.ProviderResponseTransportError,
			ProviderObservation:         observed.Observation,
			ProviderIdentityEvidence:    observed.Evidence,
		}, client.err
	}
	return generation, nil
}

func (client *policyTestClient) testAttestation(
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	if len(client.attestations) > 0 {
		index := client.observationCalls - 1
		if index >= len(client.attestations) {
			index = len(client.attestations) - 1
		}
		return client.attestations[index], nil
	}
	return llm.NewProviderIdentityAttestation(
		expected, "test:/version", "test:/installed", "test:/runner",
	)
}

func (client *policyTestClient) CleanupPreparedModel(llm.PreparedModel) { client.cleanupCalls++ }

func (client *policyTestClient) RequireExactPreparedContract() error { return nil }

func (client *policyTestClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return expected.Validate()
}

func (client *policyTestClient) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if prepared.ResponseFormat != llm.ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
		prepared.ThinkingEnabled || prepared.Temperature == nil || *prepared.Temperature != 0 ||
		prepared.ProviderIdentityExpectation == nil || prepared.ProviderObservationChallenge == "" {
		return errors.New("prepared contract is not exact")
	}
	return nil
}

func (client *policyTestClient) Embedding(context.Context, string) ([]float64, error) {
	client.otherCalls++
	return nil, nil
}
