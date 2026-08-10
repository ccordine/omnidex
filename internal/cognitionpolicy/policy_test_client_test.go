package cognitionpolicy

import (
	"context"
	"encoding/json"
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
			Schema:                   llm.PreparedGenerationSchemaV1,
			ProviderObservation:      observed.Observation,
			ProviderIdentityEvidence: observed.Evidence,
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
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: true,
		Content:               client.response,
		ProviderRequestSHA256: mustPolicyTestRequestSHA(prepared),
		ProviderHTTPStatus:    200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
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
			Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: true,
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

func policyTestPreparedGeneration(
	attempt CallAttempt,
	response string,
) llm.PreparedGeneration {
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		panic(err)
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil {
		panic(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC), attempt.ProviderAttestation,
		challenge,
	)
	if err != nil {
		panic(err)
	}
	rawResponse := policyTestRawProviderResponse(attempt.Brain.Model, response)
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: true,
		Content: response, ProviderRequestSHA256: attempt.ExpectedProviderRequestSHA256,
		ProviderHTTPStatus: 200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        policySHA256(string(rawResponse)),
		ProviderResponseBytes:         int64(len(rawResponse)),
		ProviderResponseCaptureSHA256: policySHA256(string(rawResponse)),
		ProviderResponseCapturedBytes: len(rawResponse),
		ProviderResponseCapture:       rawResponse,
		ProviderResponseModel:         attempt.Brain.Model,
		ProviderDonePresent:           true,
		ProviderDone:                  true,
		ProviderDoneReason:            "stop",
		UsagePresent:                  true, Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		}, ProviderObservation: observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}
}

func mustPolicyTestRequestSHA(prepared llm.PreparedModel) string {
	value, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		panic(err)
	}
	return value
}

func policyTestRawProviderResponse(model, response string) []byte {
	generation := llm.PreparedGeneration{
		Content: response, ProviderResponseModel: model,
		ProviderDonePresent: true, ProviderDone: true, ProviderDoneReason: "stop",
		UsagePresent: true, Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		},
	}
	return policyTestRawProviderResponseForGeneration(generation, true)
}

func policyTestRefreshRawProviderResponse(generation *llm.PreparedGeneration, includeEval bool) {
	raw := policyTestRawProviderResponseForGeneration(*generation, includeEval)
	generation.ProviderResponseSHA256 = policySHA256(string(raw))
	generation.ProviderResponseBytes = int64(len(raw))
	generation.ProviderResponseCaptureSHA256 = generation.ProviderResponseSHA256
	generation.ProviderResponseCapturedBytes = len(raw)
	generation.ProviderResponseCapture = raw
}

func policyTestRawProviderResponseForGeneration(
	generation llm.PreparedGeneration,
	includeEval bool,
) []byte {
	var evalCount *int
	var done *bool
	if includeEval {
		evalCount = &generation.Usage.EvalCount
	}
	if generation.ProviderDonePresent {
		done = &generation.ProviderDone
	}
	raw, err := json.Marshal(struct {
		Model              string `json:"model"`
		CreatedAt          string `json:"created_at"`
		Response           string `json:"response"`
		Done               *bool  `json:"done,omitempty"`
		DoneReason         string `json:"done_reason"`
		TotalDuration      int64  `json:"total_duration"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          *int   `json:"eval_count,omitempty"`
		EvalDuration       int64  `json:"eval_duration"`
	}{
		generation.ProviderResponseModel, "2026-08-09T22:00:00Z",
		generation.Content, done,
		generation.ProviderDoneReason, generation.Usage.TotalDurationNanos,
		generation.Usage.LoadDurationNanos, generation.Usage.PromptEvalCount,
		generation.Usage.PromptEvalDurationNanos, evalCount, generation.Usage.EvalDurationNanos,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func policyTestFailedGeneration(attempt CallAttempt) llm.PreparedGeneration {
	generation := policyTestPreparedGeneration(attempt, "ignored")
	generation.Content = ""
	generation.ProviderHTTPStatus = 0
	generation.ProviderResponseDisposition = llm.ProviderResponseTransportError
	generation.ProviderResponseComplete = false
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
	return generation
}
