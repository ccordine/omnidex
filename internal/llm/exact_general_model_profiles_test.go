package llm

import (
	"testing"
)

func TestGeneralLocalModelProfilesAreClosedNativeTemplateProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id            string
		architecture  string
		model         string
		pre           string
		caps          []string
		adds          map[string]bool
		deterministic bool
	}{
		{
			id:           ExactPreparedTokenizerProfilePhi3GPT4O,
			architecture: "phi3", model: "gpt2", pre: "gpt-4o",
			caps: []string{"completion", "tools"},
			adds: map[string]bool{
				"tokenizer.ggml.add_bos_token": false, "tokenizer.ggml.add_eos_token": false,
			},
		},
		{
			id:           ExactPreparedTokenizerProfilePhi3DBRX,
			architecture: "phi3", model: "gpt2", pre: "dbrx",
			caps: []string{"completion"},
			adds: nil,
		},
		{
			id:           ExactPreparedTokenizerProfileGemma3,
			architecture: "gemma3", model: "llama", pre: "default",
			caps: []string{"completion", "vision"},
			adds: map[string]bool{
				"tokenizer.ggml.add_bos_token": true, "tokenizer.ggml.add_eos_token": false,
				"tokenizer.ggml.add_padding_token": false, "tokenizer.ggml.add_unknown_token": false,
			},
		},
		{
			id:           ExactPreparedTokenizerProfileLlama32,
			architecture: "llama", model: "gpt2", pre: "llama-bpe",
			caps: []string{"completion", "tools"},
			adds: nil, deterministic: true,
		},
		{
			id:           ExactPreparedTokenizerProfileQwen25Coder,
			architecture: "qwen2", model: "gpt2", pre: "qwen2",
			caps: []string{"completion", "tools", "insert"},
			adds: map[string]bool{
				"tokenizer.ggml.add_bos_token": false,
			},
			deterministic: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()
			profile, err := exactProviderModelProfileByID(test.id)
			if err != nil {
				t.Fatal(err)
			}
			if profile.architecture != test.architecture || profile.tokenizerModel != test.model ||
				profile.tokenizerPre != test.pre || !sameExactProfileStrings(profile.capabilities, test.caps) ||
				!sameExactProfileBools(profile.explicitAdd, test.adds) ||
				profile.templateSHA256 == "" || len(profile.parameterSHA256s) != 1 ||
				profile.transport != exactPreparedTransportNativeSystem ||
				profile.requestTemperatureSet != test.deterministic ||
				(test.deterministic && profile.requestTemperature != 0) {
				t.Fatalf("profile=%+v", profile)
			}
			settings, err := ResolveExactPreparedTransport(ProviderIdentityExpectation{
				Backend: ExactPreparedProviderBackend, BackendVersion: "0.24.0", Model: "opaque:local",
				Digest:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				Quantization: "Q4_K_M", NativeContextLimit: 8192, TokenizerProfile: test.id,
			})
			if err != nil || !settings.NativeTemplate || !settings.SeparateSystem ||
				settings.SeparateThinking ||
				(test.deterministic && (settings.Temperature == nil || *settings.Temperature != 0)) ||
				(!test.deterministic && settings.Temperature != nil) {
				t.Fatalf("settings=%+v error=%v", settings, err)
			}
		})
	}
}

func sameExactProfileBools(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key, want := range left {
		if got, exists := right[key]; !exists || got != want {
			return false
		}
	}
	return true
}

func sameExactProfileStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
