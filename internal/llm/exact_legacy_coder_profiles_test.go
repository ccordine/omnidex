package llm

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLegacyCoderProfilesSelectExactStructureUnderOpaqueTags(t *testing.T) {
	t.Parallel()
	for _, fixture := range exactLegacyCoderFixtures() {
		fixture := fixture
		t.Run(fixture.profile, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(
				t, "opaque-coder:local", fixture.showJSON(t, nil),
			)
			identity, err := DeriveExactProviderIdentityExpectation(evidence, selection)
			if err != nil {
				t.Fatal(err)
			}
			if identity.Model != selection.Model || identity.TokenizerProfile != fixture.profile {
				t.Fatalf("identity=%+v", identity)
			}
		})
	}
}

func TestLegacyCoderProfilesRejectStructuralDrift(t *testing.T) {
	t.Parallel()
	mutations := []struct {
		name string
		edit func(*exactIdentityShowResponse)
	}{
		{"capabilities", func(show *exactIdentityShowResponse) { show.Capabilities = []string{"completion", "tools"} }},
		{"template", func(show *exactIdentityShowResponse) { show.Template += " " }},
		{"parameters", func(show *exactIdentityShowResponse) { show.Parameters += "\nseed 1" }},
		{"token id", func(show *exactIdentityShowResponse) {
			show.ModelInfo["tokenizer.ggml.bos_token_id"] = json.RawMessage("999")
		}},
		{"token payload", func(show *exactIdentityShowResponse) { show.ModelInfo["tokenizer.ggml.tokens"] = json.RawMessage(`[]`) }},
		{"merges presence", func(show *exactIdentityShowResponse) {
			if _, exists := show.ModelInfo["tokenizer.ggml.merges"]; exists {
				delete(show.ModelInfo, "tokenizer.ggml.merges")
			} else {
				show.ModelInfo["tokenizer.ggml.merges"] = json.RawMessage("null")
			}
		}},
		{"tokenizer pre", func(show *exactIdentityShowResponse) {
			show.ModelInfo["tokenizer.ggml.pre"] = json.RawMessage(`"changed"`)
		}},
	}
	for _, fixture := range exactLegacyCoderFixtures() {
		fixture := fixture
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(fixture.profile+"/"+mutation.name, func(t *testing.T) {
				t.Parallel()
				selection, evidence := exactProviderProfileEvidence(
					t, "opaque-drift:local", fixture.showJSON(t, mutation.edit),
				)
				if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
					t.Fatal("structurally drifted coder profile was accepted")
				}
			})
		}
	}
}

func TestLegacyCoderPreparedRequestsKeepExactTaskBytes(t *testing.T) {
	t.Parallel()
	for _, fixture := range exactLegacyCoderFixtures() {
		fixture := fixture
		t.Run(fixture.profile, func(t *testing.T) {
			t.Parallel()
			expected := ProviderIdentityExpectation{
				Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
				Model: "opaque-coder:local", Digest: strings.Repeat("a", 64), Quantization: "Q4_0",
				NativeContextLimit: 8192, TokenizerProfile: fixture.profile,
			}
			challenge, err := DeriveProviderIdentityObservationChallenge("legacy-coder-request", expected)
			if err != nil {
				t.Fatal(err)
			}
			zero := ExactPreparedTemperature(0)
			wire, err := ExactPreparedRequestBytes(PreparedModel{
				Protocol: ExactPreparedProtocolRawTextV1, BaseModel: expected.Model, ContextModel: expected.Model,
				Prompt: "return one declaration", PromptHint: MinimalGeneratePrompt,
				MaxOutputTokens: 8192, OutputLimitMode: ExactPreparedOutputLimitNatural,
				ContextTokens: 8192, Temperature: &zero, RawTextStopSequence: ExactPreparedCodeStopV1,
				ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
			})
			if err != nil {
				t.Fatal(err)
			}
			if string(wire) != fixture.wantRequest {
				t.Fatalf("exact request changed:\n got %s\nwant %s", wire, fixture.wantRequest)
			}
			if strings.Contains(string(wire), "num_predict") {
				t.Fatalf("natural coder request contains output cap: %s", wire)
			}
		})
	}
}

func TestLegacyCoderProfileRegistryHasNoModelTagOrSizeAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("exact_provider_model_profile_registry.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"codeqwen:7b", "codegemma:2b", "codegemma:7b", "codellama:7b",
		"deepseek-coder:1.3b", "deepseek-coder:6.7b", "deepseek-coder-v2:16b", "parameter_count",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("profile registry contains model identity branch %q", forbidden)
		}
	}
}

type exactLegacyCoderFixture struct {
	profile, architecture, model, pre, template, parameters, wantRequest string
	capabilities                                                         []string
	adds, fields                                                         map[string]string
}

func (fixture exactLegacyCoderFixture) showJSON(t *testing.T, mutate func(*exactIdentityShowResponse)) string {
	t.Helper()
	info := map[string]json.RawMessage{
		"general.architecture": json.RawMessage(`"` + fixture.architecture + `"`),
		"tokenizer.ggml.model": json.RawMessage(`"` + fixture.model + `"`),
	}
	if fixture.pre != "" {
		info["tokenizer.ggml.pre"] = json.RawMessage(`"` + fixture.pre + `"`)
	}
	for key, value := range fixture.adds {
		info[key] = json.RawMessage(value)
	}
	for key, value := range fixture.fields {
		info[key] = json.RawMessage(value)
	}
	show := exactIdentityShowResponse{
		Capabilities: append([]string(nil), fixture.capabilities...),
		Template:     fixture.template, Parameters: fixture.parameters, ModelInfo: info,
	}
	if mutate != nil {
		mutate(&show)
	}
	raw, err := json.Marshal(show)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func exactLegacyCoderFixtures() []exactLegacyCoderFixture {
	systemRequest := `{"model":"opaque-coder:local","options":{"num_ctx":8192,"temperature":0},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","truncate":false}`
	gemmaFields := map[string]string{
		"tokenizer.ggml.bos_token_id": "2", "tokenizer.ggml.eos_token_id": "1",
		"tokenizer.ggml.eot_token_id": "107", "tokenizer.ggml.middle_token_id": "68",
		"tokenizer.ggml.padding_token_id": "0", "tokenizer.ggml.prefix_token_id": "67",
		"tokenizer.ggml.suffix_token_id": "69", "tokenizer.ggml.scores": "null",
		"tokenizer.ggml.token_type": "null", "tokenizer.ggml.tokens": "null",
	}
	return []exactLegacyCoderFixture{
		{
			profile: ExactPreparedTokenizerProfileCodeQwen, architecture: "qwen2", model: "llama", pre: "default",
			capabilities: []string{"completion"},
			adds:         map[string]string{"tokenizer.ggml.add_bos_token": "false", "tokenizer.ggml.add_eos_token": "false"},
			fields: map[string]string{
				"tokenizer.ggml.bos_token_id": "2", "tokenizer.ggml.eos_token_id": "4",
				"tokenizer.ggml.padding_token_id": "92298", "tokenizer.ggml.unknown_token_id": "0",
				"tokenizer.ggml.scores": "null", "tokenizer.ggml.token_type": "null", "tokenizer.ggml.tokens": "null",
			},
			template:    "{{ if .System }}<|im_start|>system\n{{ .System }}<|im_end|>\n{{ end }}{{ if .Prompt }}<|im_start|>user\n{{ .Prompt }}<|im_end|>\n{{ end }}<|im_start|>assistant\n{{ .Response }}<|im_end|>\n",
			parameters:  "stop                           \"<|im_start|>\"\nstop                           \"<|im_end|>\"",
			wantRequest: systemRequest,
		},
		{
			profile: ExactPreparedTokenizerProfileCodeGemmaFIM, architecture: "gemma", model: "llama", pre: "default",
			capabilities: []string{"completion", "insert"},
			adds:         map[string]string{"tokenizer.ggml.add_bos_token": "true", "tokenizer.ggml.add_eos_token": "false"},
			fields:       gemmaFields,
			template:     "{{- if .Suffix }}<|fim_prefix|>{{ .Prompt }}<|fim_suffix|>{{ .Suffix }}<|fim_middle|>\n{{- else }}{{ .Prompt }}\n{{- end }}",
			parameters:   "stop                           \"<|fim_prefix|>\"\nstop                           \"<|fim_suffix|>\"\nstop                           \"<|fim_middle|>\"\nstop                           \"<|file_separator|>\"\nrepeat_penalty                 1",
			wantRequest:  `{"model":"opaque-coder:local","options":{"num_ctx":8192,"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":false,"shift":false,"stream":false,"truncate":false}`,
		},
		{
			profile: ExactPreparedTokenizerProfileCodeGemmaChat, architecture: "gemma", model: "llama", pre: "default",
			capabilities: []string{"completion"},
			adds:         map[string]string{"tokenizer.ggml.add_bos_token": "true", "tokenizer.ggml.add_eos_token": "false"}, fields: gemmaFields,
			template:    "<start_of_turn>user\n{{ if .System }}{{ .System }} {{ end }}{{ .Prompt }}<end_of_turn>\n<start_of_turn>model\n{{ .Response }}<end_of_turn>\n",
			parameters:  "repeat_penalty                 1\nstop                           \"<start_of_turn>\"\nstop                           \"<end_of_turn>\"\npenalize_newline               false",
			wantRequest: systemRequest,
		},
		{
			profile: ExactPreparedTokenizerProfileCodeLlama, architecture: "llama", model: "llama",
			capabilities: []string{"completion"}, adds: nil,
			fields: map[string]string{
				"tokenizer.ggml.bos_token_id": "1", "tokenizer.ggml.eos_token_id": "2",
				"tokenizer.ggml.unknown_token_id": "0", "tokenizer.ggml.scores": "null",
				"tokenizer.ggml.token_type": "null", "tokenizer.ggml.tokens": "null",
			},
			template:    "[INST] <<SYS>>{{ .System }}<</SYS>>\n\n{{ .Prompt }} [/INST]\n",
			parameters:  "rope_frequency_base            1e+06\nstop                           \"[INST]\"\nstop                           \"[/INST]\"\nstop                           \"<<SYS>>\"\nstop                           \"<</SYS>>\"",
			wantRequest: systemRequest,
		},
		{
			profile: ExactPreparedTokenizerProfileDeepSeekCoder, architecture: "llama", model: "gpt2",
			capabilities: []string{"completion"},
			adds:         map[string]string{"tokenizer.ggml.add_bos_token": "true", "tokenizer.ggml.add_eos_token": "false"},
			fields: map[string]string{
				"tokenizer.ggml.bos_token_id": "32013", "tokenizer.ggml.eos_token_id": "32021",
				"tokenizer.ggml.padding_token_id": "32014", "tokenizer.ggml.merges": "null",
				"tokenizer.ggml.scores": "null", "tokenizer.ggml.token_type": "null", "tokenizer.ggml.tokens": "null",
			},
			template:   "{{ .System }}\n### Instruction:\n{{ .Prompt }}\n### Response:\n",
			parameters: "", wantRequest: systemRequest,
		},
		{
			profile:      ExactPreparedTokenizerProfileDeepSeekCoderV2,
			architecture: "deepseek2", model: "gpt2", pre: "deepseek-llm",
			capabilities: []string{"completion", "insert"},
			adds:         map[string]string{"tokenizer.ggml.add_bos_token": "true", "tokenizer.ggml.add_eos_token": "false"},
			fields: map[string]string{
				"tokenizer.ggml.bos_token_id": "100000", "tokenizer.ggml.eos_token_id": "100001",
				"tokenizer.ggml.padding_token_id": "100001", "tokenizer.ggml.merges": "null",
				"tokenizer.ggml.token_type": "null", "tokenizer.ggml.tokens": "null",
			},
			template:    "{{- if .Suffix }}<｜fim▁begin｜>{{ .Prompt }}<｜fim▁hole｜>{{ .Suffix }}<｜fim▁end｜>\n{{- else if .Messages }}<｜begin▁of▁sentence｜>\n{{- $system := \"\" }}\n{{- range $i, $_ := .Messages }}\n{{- if eq .Role \"system\" }}\n{{- $system = printf \"%s %s\" $system .Content }}\n{{- else if eq .Role \"user\" }}\n{{- if $system }}{{ $system }}\n{{ $system = \"\" }}\n{{ end }}User: {{ .Content }}\n\n{{ if eq (len (slice $.Messages $i)) 1 }}Assistant:\n{{- end }}\n{{- else if eq .Role \"assistant\" }}Assistant: {{ .Content }}<｜end▁of▁sentence｜>\n{{- end }}\n{{- end }}\n{{- else }}\n{{- if .System }}{{ .System }}\n{{- end }}\n{{- if .Prompt }}User: {{ .Prompt }}\n{{- end }}Assistant:{{ .Response }}\n{{- end }}",
			parameters:  "stop                           \"User:\"\nstop                           \"Assistant:\"",
			wantRequest: systemRequest,
		},
	}
}
