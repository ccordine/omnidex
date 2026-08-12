package qwenselector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

const testModel = "qwen3.5:9b-q4_K_M"

type recordingExactClient struct {
	provider       llm.ObservedProviderIdentity
	content        string
	err            error
	mutate         func(*llm.PreparedModel)
	validateMutate func(*llm.PreparedModel)
	mutateResult   func(*llm.PreparedGeneration)
	beforeReturn   func()
	promptTokens   int
	outputTokens   int

	mu       sync.Mutex
	calls    int
	prepared llm.PreparedModel
	request  []byte
}

func (client *recordingExactClient) RequireExactPreparedContract() error { return nil }

func (client *recordingExactClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return llm.ValidateExactPreparedProviderExpectation(expected)
}

func (client *recordingExactClient) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if client.validateMutate != nil {
		client.validateMutate(&prepared)
	}
	_, err := llm.ExactPreparedRequestBytes(prepared)
	return err
}

func (client *recordingExactClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	raw, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	client.mu.Lock()
	client.calls++
	client.prepared = prepared
	client.request = append([]byte{}, raw...)
	client.mu.Unlock()
	if client.mutate != nil {
		client.mutate(&prepared)
	}
	promptTokens, outputTokens := client.promptTokens, client.outputTokens
	if promptTokens == 0 {
		promptTokens = 1
	}
	if outputTokens == 0 {
		outputTokens = 1
	}
	generation, generationErr := testGeneration(
		prepared, client.provider, client.content, promptTokens, outputTokens,
	)
	if generationErr != nil {
		return llm.PreparedGeneration{}, generationErr
	}
	if client.mutateResult != nil {
		client.mutateResult(&generation)
	}
	if client.beforeReturn != nil {
		client.beforeReturn()
	}
	return generation, client.err
}

func testProvider(t *testing.T) llm.ObservedProviderIdentity {
	t.Helper()
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: testModel, Quantization: "Q4_K_M",
		NativeContextLimit: 32768, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	expected.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	attestation, err := llm.NewProviderIdentityAttestation(
		expected, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := llm.ProviderIdentitySelection{Model: testModel, NativeContextLimit: 32768}
	tokenizerRequest, err := llm.ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	preloadRequest, err := llm.ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"}}]}`,
		testModel, testModel, expected.Digest,
	))
	tokenizer := []byte(`{"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"},"context_length":32768}]}`,
		testModel, testModel, expected.Digest,
	))
	evidence, err := llm.NewSuccessfulProviderIdentityEvidence(
		[]byte(`{"version":"0.24.0"}`), installed, tokenizerRequest, tokenizer,
		preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge("qwen-selector-test", expected)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewObservedProviderIdentity(
		time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC), attestation, evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func testGeneration(
	prepared llm.PreparedModel,
	provider llm.ObservedProviderIdentity,
	content string,
	promptCount int,
	evalCount int,
) (llm.PreparedGeneration, error) {
	observed, err := llm.NewObservedProviderIdentity(
		time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), provider.Attestation,
		provider.Evidence, prepared.ProviderObservationChallenge,
	)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	done := true
	raw, err := json.Marshal(struct {
		Model          string `json:"model"`
		CreatedAt      string `json:"created_at"`
		Response       string `json:"response"`
		Done           *bool  `json:"done"`
		DoneReason     string `json:"done_reason"`
		Total          int64  `json:"total_duration"`
		Load           int64  `json:"load_duration"`
		PromptCount    *int   `json:"prompt_eval_count"`
		PromptDuration int64  `json:"prompt_eval_duration"`
		Eval           *int   `json:"eval_count"`
		EvalDuration   int64  `json:"eval_duration"`
	}{
		Model: prepared.ContextModel, CreatedAt: "2026-08-12T12:01:00Z",
		Response: content, DoneReason: "stop", Done: &done,
		Total: 10, Load: 1, PromptCount: &promptCount, PromptDuration: 2,
		Eval: &evalCount, EvalDuration: 3,
	})
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	digest := sha256.Sum256(raw)
	responseSHA := hex.EncodeToString(digest[:])
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content: content, ProviderRequestSHA256: requestSHA, ProviderHTTPStatus: 200,
		ProviderResponseDisposition: llm.ProviderResponseSucceeded, ProviderResponseComplete: true,
		ProviderContentEncoding:    llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown: true, ProviderResponseSHA256: responseSHA,
		ProviderResponseBytes: int64(len(raw)), ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw,
		ProviderResponseModel: prepared.ContextModel, ProviderDonePresent: true,
		ProviderDone: true, ProviderDoneReason: "stop", UsagePresent: true,
		Usage: llm.ProviderGenerationUsage{
			PromptEvalCount: promptCount, EvalCount: evalCount, TotalDurationNanos: 10,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 2, EvalDurationNanos: 3,
		},
		ProviderObservation: observed.Observation, ProviderIdentityEvidence: observed.Evidence,
	}, nil
}
