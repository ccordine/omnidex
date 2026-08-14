package llm

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDeriveExactProviderIdentityExpectationSelectsRegisteredStructuralProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		model         string
		show          string
		wantTokenizer string
	}{
		{
			name:          "existing qwen35 profile",
			model:         "qwen3.5:9b-q4_K_M",
			show:          exactProviderQwen35Show,
			wantTokenizer: ExactPreparedTokenizerProfile,
		},
		{
			name:          "installed r1 qwen3 profile",
			model:         "deepseek-r1:8b",
			show:          exactProviderQwen3Qwen2Show,
			wantTokenizer: ExactPreparedTokenizerProfileQwen3Qwen2,
		},
		{
			name:          "same structural profile under an opaque model tag",
			model:         "opaque-local-model:8b",
			show:          exactProviderQwen3Qwen2Show,
			wantTokenizer: ExactPreparedTokenizerProfileQwen3Qwen2,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(t, test.model, test.show)
			got, err := DeriveExactProviderIdentityExpectation(evidence, selection)
			if err != nil {
				t.Fatalf("derive registered structural provider profile: %v", err)
			}
			if got.Model != test.model || got.TokenizerProfile != test.wantTokenizer {
				t.Fatalf("derived identity=%+v, want model=%q tokenizer_profile=%q", got, test.model, test.wantTokenizer)
			}
		})
	}
}

func TestDeriveExactProviderIdentityExpectationRejectsUnregisteredStructuralProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		show string
	}{
		{
			name: "registered qwen3 tokenizer without required thinking capability",
			show: strings.Replace(
				exactProviderQwen3Qwen2Show,
				`["completion","thinking"]`, `["completion"]`, 1,
			),
		},
		{
			name: "unknown tokenizer family",
			show: strings.Replace(
				exactProviderQwen3Qwen2Show,
				`"tokenizer.ggml.pre":"qwen2"`, `"tokenizer.ggml.pre":"unregistered"`, 1,
			),
		},
		{
			name: "qwen35 boundary behavior changed",
			show: strings.Replace(
				exactProviderQwen35Show,
				`"tokenizer.ggml.add_eos_token":false`,
				`"tokenizer.ggml.add_bos_token":false,"tokenizer.ggml.add_eos_token":false`,
				1,
			),
		},
		{
			name: "qwen35 template digest changed",
			show: strings.Replace(
				exactProviderQwen35Show, `"template":"{{ .Prompt }}"`, `"template":"{{.Prompt}}"`, 1,
			),
		},
		{
			name: "qwen35 parameters digest changed",
			show: strings.Replace(
				exactProviderQwen35Show, "presence_penalty               1.5", "presence_penalty               1.4", 1,
			),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(t, "structural-rejection:8b", test.show)
			if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatal("unregistered provider model identity was accepted")
			}
		})
	}
}

func TestExactProviderProfileSelectionHasNoModelTagBranches(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"exact_provider_model_profile.go", "exact_profile_request.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"deepseek-r1", "ministral-3:", "qwen3.5:"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s contains model-tag branch %q", name, forbidden)
			}
		}
	}
}

const exactProviderQwen35Show = `{
  "capabilities":["completion","vision","tools","thinking"],
  "parameters":"temperature                    1\ntop_k                          20\ntop_p                          0.95\npresence_penalty               1.5",
  "template":"{{ .Prompt }}",
  "model_info":{
    "general.architecture":"qwen35",
    "tokenizer.ggml.model":"gpt2",
    "tokenizer.ggml.pre":"qwen35",
    "tokenizer.ggml.add_eos_token":false,
    "tokenizer.ggml.add_padding_token":false,
    "tokenizer.ggml.tokens":null,
    "tokenizer.ggml.token_type":null,
    "tokenizer.ggml.merges":null
  }
}`

const exactProviderQwen3Qwen2Show = `{
  "capabilities":["completion","thinking"],
  "parameters":"stop                           \"<｜begin▁of▁sentence｜>\"\nstop                           \"<｜end▁of▁sentence｜>\"\nstop                           \"<｜User｜>\"\nstop                           \"<｜Assistant｜>\"\ntemperature                    0.6\ntop_p                          0.95",
  "template":"{{- if .System }}{{ .System }}{{ end }}\n{{- range $i, $_ := .Messages }}\n{{- $last := eq (len (slice $.Messages $i)) 1}}\n{{- if eq .Role \"user\" }}<｜User｜>{{ .Content }}\n{{- else if eq .Role \"assistant\" }}<｜Assistant｜>\n  {{- if and $.IsThinkSet (and $last .Thinking) -}}\n<think>\n{{ .Thinking }}\n</think>\n{{- end }}{{ .Content }}{{- if not $last }}<｜end▁of▁sentence｜>{{- end }}\n{{- end }}\n{{- if and $last (ne .Role \"assistant\") }}<｜Assistant｜>\n{{- if and $.IsThinkSet (not $.Think) -}}\n<think>\n\n</think>\n\n{{ end }}\n{{- end -}}\n{{- end }}",
  "model_info":{
    "general.architecture":"qwen3",
    "tokenizer.ggml.model":"gpt2",
    "tokenizer.ggml.pre":"qwen2",
    "tokenizer.ggml.add_bos_token":false,
    "tokenizer.ggml.add_eos_token":false,
    "tokenizer.ggml.tokens":null,
    "tokenizer.ggml.token_type":null,
    "tokenizer.ggml.merges":null
  }
}`

func exactProviderProfileEvidence(
	t *testing.T,
	model string,
	show string,
) (ProviderIdentitySelection, ProviderIdentityEvidence) {
	t.Helper()
	selection := ProviderIdentitySelection{Model: model, NativeContextLimit: 8192}
	showRequest, err := ExactProviderTokenizerRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	preloadRequest, err := ExactProviderPreloadRequestBytes(selection)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	installed := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"}}]}`,
		model, model, digest,
	))
	runner := []byte(fmt.Sprintf(
		`{"models":[{"name":%q,"model":%q,"size":1,"digest":%q,"details":{"quantization_level":"Q4_K_M"},"context_length":8192}]}`,
		model, model, digest,
	))
	evidence, err := NewSuccessfulProviderIdentityEvidence(
		[]byte(`{"version":"0.24.0"}`), installed,
		showRequest, []byte(show), preloadRequest, []byte(`{"done":true}`), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	return selection, evidence
}
