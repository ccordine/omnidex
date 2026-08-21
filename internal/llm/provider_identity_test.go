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

func TestProviderIdentityDiscoveryBindsCanonicalRunnerToInstalledAlias(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	expected.Model = "qwen3.5:9b"
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	digest := expected.Digest
	installed := []byte(fmt.Sprintf(
		`{"models":[`+
			`{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q}},`+
			`{"name":"qwen3.5:9b-q4_K_M","model":"qwen3.5:9b-q4_K_M","size":1,"digest":%q,"details":{"quantization_level":%q}}]}`,
		expected.Model, expected.Model, digest, expected.Quantization,
		digest, expected.Quantization,
	))
	runner := providerIdentityRunnerResponse(
		"qwen3.5:9b-q4_K_M", digest, expected.Quantization, expected.NativeContextLimit,
	)
	evidence := providerIdentityEvidenceWithModelResponses(
		t, providerIdentityTestEvidence(t, expected), installed, runner,
	)

	got, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != expected.Model || got.Digest != digest || got.Quantization != expected.Quantization {
		t.Fatalf("canonical runner changed selected provider identity: %+v", got)
	}
}

func TestProviderIdentityDiscoveryRejectsNonUniqueOrChangedRunnerIdentity(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	expected.Model = "qwen3.5:9b"
	base := providerIdentityTestEvidence(t, expected)
	installed := base.Operations[1].ResponseCapture
	canonical := "qwen3.5:9b-q4_K_M"
	validRunner := string(providerIdentityRunnerResponse(
		canonical, expected.Digest, expected.Quantization, expected.NativeContextLimit,
	))
	tests := map[string][]byte{
		"different digest": providerIdentityRunnerResponse(
			canonical, strings.Repeat("b", 64), expected.Quantization, expected.NativeContextLimit,
		),
		"different quantization": providerIdentityRunnerResponse(
			canonical, expected.Digest, "Q8_0", expected.NativeContextLimit,
		),
		"different context": providerIdentityRunnerResponse(
			canonical, expected.Digest, expected.Quantization, expected.NativeContextLimit/2,
		),
		"ambiguous aliases": []byte(strings.Replace(
			validRunner,
			`]}`,
			fmt.Sprintf(
				`,{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":%q},"context_length":%d}]}`,
				expected.Model, expected.Model, expected.Digest, expected.Quantization,
				expected.NativeContextLimit,
			),
			1,
		)),
	}
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	for name, runner := range tests {
		name, runner := name, runner
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			evidence := providerIdentityEvidenceWithModelResponses(t, base, installed, runner)
			if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatal("changed running provider identity was accepted")
			}
		})
	}
}

func providerIdentityRunnerResponse(
	name string,
	digest string,
	quantization string,
	contextLimit int,
) []byte {
	return []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,`+
			`"details":{"quantization_level":%q},"context_length":%d}]}`,
		name, name, digest, quantization, contextLimit,
	))
}

func providerIdentityEvidenceWithModelResponses(
	t *testing.T,
	base ProviderIdentityEvidence,
	installed []byte,
	runner []byte,
) ProviderIdentityEvidence {
	t.Helper()
	evidence, err := NewSuccessfulProviderIdentityEvidence(
		base.Operations[0].ResponseCapture,
		installed,
		base.Operations[2].Request,
		base.Operations[2].ResponseCapture,
		base.Operations[3].Request,
		base.Operations[3].ResponseCapture,
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
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
