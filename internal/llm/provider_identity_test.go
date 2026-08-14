package llm

import (
	"fmt"
	"strings"
	"testing"
)

func TestProviderIdentityAttestationBindsEveryLiveField(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	attestation, err := NewProviderIdentityAttestation(
		expected, "ollama:/api/version", "ollama:/api/tags", "ollama:/api/ps",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateFor(expected); err != nil {
		t.Fatal(err)
	}
	changed := expected
	changed.Digest = strings.Repeat("b", 64)
	if err := attestation.ValidateFor(changed); err == nil {
		t.Fatal("attestation accepted another model digest")
	}
}

func providerIdentityTestExpectation() ProviderIdentityExpectation {
	return ProviderIdentityExpectation{
		Backend: "ollama", BackendVersion: "0.24.0", Model: "qwen:9b",
		Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M", NativeContextLimit: 32768,
		TokenizerProfile: ExactPreparedTokenizerProfile,
	}
}

func providerIdentityTestEvidence(t *testing.T, expected ProviderIdentityExpectation) ProviderIdentityEvidence {
	t.Helper()
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	showRequest, err := ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	preloadRequest, err := ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q}}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization,
	))
	show := []byte(`{"capabilities":["completion","vision","tools","thinking"],` +
		`"parameters":"temperature                    1\ntop_k                          20\ntop_p                          0.95\npresence_penalty               1.5",` +
		`"template":"{{ .Prompt }}",` +
		`"model_info":{"general.architecture":"qwen35",` +
		`"tokenizer.ggml.model":"gpt2","tokenizer.ggml.pre":"qwen35",` +
		`"tokenizer.ggml.add_eos_token":false,"tokenizer.ggml.add_padding_token":false,` +
		`"tokenizer.ggml.tokens":null,"tokenizer.ggml.token_type":null,` +
		`"tokenizer.ggml.merges":null}}`)
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,`+
			`"details":{"quantization_level":%q},"context_length":%d}]}`,
		expected.Model, expected.Model, expected.Digest, expected.Quantization,
		expected.NativeContextLimit,
	))
	evidence, err := NewSuccessfulProviderIdentityEvidence(
		[]byte(fmt.Sprintf(`{"version":%q}`, expected.BackendVersion)), installed,
		showRequest, show, preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
