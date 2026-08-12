package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func (client *repositoryCognitionTestClient) ObserveProviderIdentity(
	_ context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return client.repositoryCognitionObservedIdentity(
		request.Expectation, request.ChallengeSHA256,
	)
}

func (client *repositoryCognitionTestClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if err := client.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedGeneration{}, err
	}
	expected := *prepared.ProviderIdentityExpectation
	observed, err := client.repositoryCognitionObservedIdentity(
		expected, prepared.ProviderObservationChallenge,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	response, err := repositoryCognitionDecisionFromEnvelope(prepared.Prompt)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	raw, err := repositoryCognitionRawProviderResponse(prepared.ContextModel, response)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	responseSHA := repositoryCognitionSHA256(raw)

	client.mu.Lock()
	client.generated++
	client.prompts = append(client.prompts, prepared.Prompt)
	client.mu.Unlock()
	return llm.PreparedGeneration{
		Schema:                        llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition:    llm.ProviderRequestDispatched,
		Content:                       response,
		ProviderRequestSHA256:         requestSHA,
		ProviderHTTPStatus:            200,
		ProviderResponseDisposition:   llm.ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        responseSHA,
		ProviderResponseBytes:         int64(len(raw)),
		ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(raw),
		ProviderResponseCapture:       raw,
		ProviderResponseModel:         prepared.ContextModel,
		ProviderDonePresent:           true,
		ProviderDone:                  true,
		ProviderDoneReason:            "stop",
		UsagePresent:                  true,
		Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: 10, EvalCount: 1, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		},
		ProviderObservation:      observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}, nil
}

func (client *repositoryCognitionTestClient) repositoryCognitionObservedIdentity(
	expected llm.ProviderIdentityExpectation,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	client.mu.Lock()
	client.attestations++
	client.mu.Unlock()
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "worker-test:/version", "worker-test:/installed", "worker-test:/runner",
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	selection := llm.ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	showRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q}}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization,
	))
	show := []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q},"context_length":%d}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization,
		expected.NativeContextLimit,
	))
	evidence, err := llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(fmt.Sprintf(`{"version":%q}`, expected.BackendVersion)), installed,
		showRequest, show, preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		return llm.ObservedProviderIdentity{}, err
	}
	return llm.NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation, evidence, challenge,
	)
}

func repositoryCognitionRawProviderResponse(model, response string) ([]byte, error) {
	done := true
	evalCount := 1
	return json.Marshal(struct {
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
		model, "2026-08-12T12:00:00Z", response, &done, "stop",
		10, 1, 10, 2, &evalCount, 3,
	})
}

func repositoryCognitionSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
