package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func (client *witnessPolicyClient) PrepareContextModel(
	_ context.Context,
	modelName string,
	prompt string,
) (llm.PreparedModel, error) {
	if modelName != client.model || prompt == "" {
		return llm.PreparedModel{}, fmt.Errorf("witness policy received invalid prepared-model authority")
	}
	client.mu.Lock()
	client.prompts = append(client.prompts, prompt)
	client.mu.Unlock()
	return llm.PreparedModel{
		BaseModel: modelName, ContextModel: modelName, Prompt: prompt,
	}, nil
}

func (client *witnessPolicyClient) GeneratePrepared(
	_ context.Context,
	prepared llm.PreparedModel,
) (string, error) {
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return "", err
	}
	if prepared.PromptHint != llm.MinimalGeneratePrompt || prepared.ResponseFormat != llm.ResponseFormatJSON ||
		len(prepared.ResponseSchema) == 0 || prepared.ThinkingEnabled || prepared.Temperature == nil ||
		*prepared.Temperature != 0 || prepared.MaxOutputTokens <= 0 || prepared.ContextTokens <= 0 {
		return "", fmt.Errorf("witness policy received a non-frozen prepared cognition contract")
	}
	return client.generateDecision(prepared.BaseModel, prepared.Prompt)
}

func (client *witnessPolicyClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if err := client.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedGeneration{}, err
	}
	content, err := client.generateDecision(prepared.BaseModel, prepared.Prompt)
	if err != nil {
		return client.exactPreparedFailure(prepared, err)
	}
	return client.exactPreparedGeneration(prepared, content)
}

func (client *witnessPolicyClient) exactPreparedFailure(
	prepared llm.PreparedModel,
	failure error,
) (llm.PreparedGeneration, error) {
	if prepared.ProviderIdentityExpectation == nil {
		return llm.PreparedGeneration{}, failure
	}
	attestation, err := client.providerAttestation(*prepared.ProviderIdentityExpectation)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	observed, err := newWitnessProviderIdentity(
		attestation, prepared.ProviderObservationChallenge, len(client.renderedPrompts())+1,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: true,
		ProviderRequestSHA256:       requestSHA,
		ProviderResponseDisposition: llm.ProviderResponseTransportError,
		ProviderObservation:         observed.Observation,
		ProviderIdentityEvidence:    observed.Evidence,
	}, failure
}

func (client *witnessPolicyClient) exactPreparedGeneration(
	prepared llm.PreparedModel,
	content string,
) (llm.PreparedGeneration, error) {
	if prepared.ProviderIdentityExpectation == nil {
		return llm.PreparedGeneration{}, fmt.Errorf("witness policy lacks provider identity authority")
	}
	attestation, err := client.providerAttestation(*prepared.ProviderIdentityExpectation)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	observed, err := newWitnessProviderIdentity(
		attestation, prepared.ProviderObservationChallenge, len(client.renderedPrompts())+1,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	usage := llm.ProviderGenerationUsage{
		PromptEvalCount: 1, EvalCount: 1, TotalDurationNanos: 4,
		LoadDurationNanos: 1, PromptEvalDurationNanos: 1, EvalDurationNanos: 1,
	}
	rawResponse, err := witnessRawProviderResponse(prepared.ContextModel, content, usage)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	responseSHA := ablationContentSHA256(string(rawResponse))
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: true,
		Content: content, ProviderRequestSHA256: requestSHA,
		ProviderHTTPStatus: 200, ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderResponseComplete: true, ProviderResponseBytesKnown: true,
		ProviderResponseSHA256: responseSHA, ProviderResponseBytes: int64(len(rawResponse)),
		ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(rawResponse),
		ProviderResponseCapture:       rawResponse, ProviderResponseModel: prepared.ContextModel,
		ProviderDonePresent: true, ProviderDone: true, ProviderDoneReason: "stop",
		UsagePresent: true, Usage: usage,
		ProviderObservation:      observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}, nil
}

func witnessRawProviderResponse(
	model string,
	content string,
	usage llm.ProviderGenerationUsage,
) ([]byte, error) {
	return json.Marshal(struct {
		Model              string `json:"model"`
		CreatedAt          string `json:"created_at"`
		Response           string `json:"response"`
		Done               bool   `json:"done"`
		DoneReason         string `json:"done_reason"`
		TotalDuration      int64  `json:"total_duration"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          int    `json:"eval_count"`
		EvalDuration       int64  `json:"eval_duration"`
	}{
		model, "2026-08-09T23:00:00Z", content, true, "stop",
		usage.TotalDurationNanos, usage.LoadDurationNanos, usage.PromptEvalCount,
		usage.PromptEvalDurationNanos, usage.EvalCount, usage.EvalDurationNanos,
	})
}

func (*witnessPolicyClient) CleanupPreparedModel(llm.PreparedModel) {}

func (*witnessPolicyClient) RequireExactPreparedContract() error { return nil }

func (*witnessPolicyClient) ValidateExactPreparedProvider(
	llm.ProviderIdentityExpectation,
) error {
	return nil
}

func (client *witnessPolicyClient) ExpectedPreparedRequestSHA256(
	prepared llm.PreparedModel,
) (string, error) {
	if err := client.ValidateExactPreparedContract(prepared); err != nil {
		return "", err
	}
	return llm.ExactPreparedRequestSHA256(prepared)
}

func (*witnessPolicyClient) AttestProviderIdentity(
	_ context.Context,
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	return llm.NewProviderIdentityAttestation(
		expected, "fixture:backend", "fixture:installed", "fixture:runner",
	)
}

func (client *witnessPolicyClient) ObserveProviderIdentity(
	_ context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	attestation, err := client.providerAttestation(request.Expectation)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return newWitnessProviderIdentity(attestation, request.ChallengeSHA256, 1)
}

func (*witnessPolicyClient) providerAttestation(
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	return llm.NewProviderIdentityAttestation(
		expected, "fixture:backend", "fixture:installed", "fixture:runner",
	)
}

func newWitnessProviderIdentity(
	attestation llm.ProviderIdentityAttestation,
	challenge string,
	sequence int,
) (llm.ObservedProviderIdentity, error) {
	stamp := time.Date(2026, 8, 9, 23, 0, sequence%60, 0, time.UTC)
	evidence, err := witnessProviderIdentityEvidence(attestation)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return llm.NewObservedProviderIdentity(stamp, attestation, evidence, challenge)
}

func witnessProviderIdentityEvidence(
	attestation llm.ProviderIdentityAttestation,
) (llm.ProviderIdentityEvidence, error) {
	selection := llm.ProviderIdentitySelection{
		Model: attestation.Model, NativeContextLimit: attestation.NativeContextLimit,
	}
	tokenizerRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	version := []byte(fmt.Sprintf(`{"version":%q}`, attestation.BackendVersion))
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"parent_model":"","format":"gguf","family":"qwen3","families":["qwen3"],"parameter_size":"9B","quantization_level":%q}}]}`,
		attestation.Model, attestation.Model, attestation.Digest, attestation.Quantization,
	))
	tokenizer := []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"size_vram":1,"digest":%q,"details":{"parent_model":"","format":"gguf","family":"qwen3","families":["qwen3"],"parameter_size":"9B","quantization_level":%q},"context_length":%d}]}`,
		attestation.Model, attestation.Model, attestation.Digest,
		attestation.Quantization, attestation.NativeContextLimit,
	))
	return llm.NewSuccessfulProviderIdentityEvidence(
		version, installed, tokenizerRequest, tokenizer,
		preloadRequest, []byte(`{"done":true}`), runner,
	)
}

func (*witnessPolicyClient) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return err
	}
	if prepared.BaseModel == "" || prepared.ContextModel != prepared.BaseModel || prepared.Prompt == "" ||
		prepared.PromptHint != llm.MinimalGeneratePrompt || prepared.MaxOutputTokens <= 0 ||
		prepared.ContextTokens <= 0 || prepared.ResponseFormat != llm.ResponseFormatJSON ||
		len(prepared.ResponseSchema) == 0 || prepared.ThinkingEnabled || prepared.Temperature == nil ||
		*prepared.Temperature != 0 {
		return fmt.Errorf("witness policy received a non-exact prepared cognition contract")
	}
	input, err := llm.ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedInputBudget(
		prepared.ContextTokens, len([]byte(input))+llm.MaxRawInputSpecialTokenReserve,
		prepared.MaxOutputTokens, input, llm.MaxRawInputSpecialTokenReserve,
	)
}

func (*witnessPolicyClient) Embedding(context.Context, string) ([]float64, error) {
	return nil, fmt.Errorf("witness policy does not permit embedding fallback")
}

func (client *witnessPolicyClient) calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.next
}

func (client *witnessPolicyClient) renderedPrompts() []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]string{}, client.prompts...)
}
