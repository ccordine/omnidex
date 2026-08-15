package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQwen25CoderProfileSelectsExactStructureUnderOpaqueTags(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{"opaque-coder:first", "opaque-coder:second"} {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(
				t, modelName, exactQwen25CoderShowJSON(t, nil),
			)
			identity, err := DeriveExactProviderIdentityExpectation(evidence, selection)
			if err != nil {
				t.Fatal(err)
			}
			if identity.Model != modelName ||
				identity.TokenizerProfile != ExactPreparedTokenizerProfileQwen25Coder {
				t.Fatalf("identity=%+v", identity)
			}
		})
	}
}

func TestQwen25CoderProfileRejectsStructuralDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*exactIdentityShowResponse)
	}{
		{"capabilities", func(show *exactIdentityShowResponse) { show.Capabilities = []string{"completion", "tools"} }},
		{"template", func(show *exactIdentityShowResponse) { show.Template += " " }},
		{"parameters", func(show *exactIdentityShowResponse) { show.Parameters = "temperature 0" }},
		{"BOS", func(show *exactIdentityShowResponse) {
			show.ModelInfo["tokenizer.ggml.add_bos_token"] = json.RawMessage("true")
		}},
		{"EOS presence", func(show *exactIdentityShowResponse) {
			show.ModelInfo["tokenizer.ggml.add_eos_token"] = json.RawMessage("false")
		}},
		{"tokenizer pre", func(show *exactIdentityShowResponse) {
			show.ModelInfo["tokenizer.ggml.pre"] = json.RawMessage(`"changed"`)
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(
				t, "opaque-coder:drift", exactQwen25CoderShowJSON(t, test.mutate),
			)
			if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatal("structurally drifted Qwen 2.5 Coder profile was accepted")
			}
		})
	}
}

func TestQwen25CoderPreparedRequestUsesNaturalNativeTemplate(t *testing.T) {
	t.Parallel()
	expected := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: "0.24.0",
		Model:        "opaque-coder:local",
		Digest:       strings.Repeat("a", 64),
		Quantization: "Q4_K_M", NativeContextLimit: 8192,
		TokenizerProfile: ExactPreparedTokenizerProfileQwen25Coder,
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("qwen25-coder-request", expected)
	if err != nil {
		t.Fatal(err)
	}
	zero := ExactPreparedTemperature(0)
	prepared := PreparedModel{
		Protocol: ExactPreparedProtocolRawTextV1, BaseModel: expected.Model,
		ContextModel: expected.Model, Prompt: "return one declaration",
		PromptHint: MinimalGeneratePrompt, MaxOutputTokens: expected.NativeContextLimit,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ContextTokens:   expected.NativeContextLimit, Temperature: &zero,
		RawTextStopSequence:         ExactPreparedCodeStopV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"opaque-coder:local","options":{"num_ctx":8192,"temperature":0},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","truncate":false}`
	if string(wire) != want {
		t.Fatalf("Qwen 2.5 Coder exact request changed:\n got %s\nwant %s", wire, want)
	}
}

func exactQwen25CoderShowJSON(
	t *testing.T,
	mutate func(*exactIdentityShowResponse),
) string {
	t.Helper()
	show := exactIdentityShowResponse{
		Capabilities: []string{"completion", "tools", "insert"},
		Template:     exactQwen25CoderTemplate,
		ModelInfo: map[string]json.RawMessage{
			"general.architecture":         json.RawMessage(`"qwen2"`),
			"tokenizer.ggml.model":         json.RawMessage(`"gpt2"`),
			"tokenizer.ggml.pre":           json.RawMessage(`"qwen2"`),
			"tokenizer.ggml.add_bos_token": json.RawMessage("false"),
			"tokenizer.ggml.tokens":        json.RawMessage("null"),
			"tokenizer.ggml.token_type":    json.RawMessage("null"),
			"tokenizer.ggml.merges":        json.RawMessage("null"),
		},
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

var exactQwen25CoderTemplate = strings.Join([]string{
	"{{- if .Suffix }}<|fim_prefix|>{{ .Prompt }}<|fim_suffix|>{{ .Suffix }}<|fim_middle|>",
	"{{- else if .Messages }}",
	"{{- if or .System .Tools }}<|im_start|>system",
	"{{- if .System }}",
	"{{ .System }}",
	"{{- end }}",
	"{{- if .Tools }}",
	"",
	"# Tools",
	"",
	"You may call one or more functions to assist with the user query.",
	"",
	"You are provided with function signatures within <tools></tools>:",
	"<tools>",
	"{{- range .Tools }}",
	`{"type": "function", "function": {{ .Function }}}`,
	"{{- end }}",
	"</tools>",
	"",
	"For each function call, return a json object with function name and arguments within <tool_call></tool_call> with NO other text. Do not include any backticks or ```json.",
	"<tool_call>",
	`{"name": <function-name>, "arguments": <args-json-object>}`,
	"</tool_call>",
	"{{- end }}<|im_end|>",
	"{{ end }}",
	"{{- range $i, $_ := .Messages }}",
	"{{- $last := eq (len (slice $.Messages $i)) 1 -}}",
	`{{- if eq .Role "user" }}<|im_start|>user`,
	"{{ .Content }}<|im_end|>",
	`{{ else if eq .Role "assistant" }}<|im_start|>assistant`,
	"{{ if .Content }}{{ .Content }}",
	"{{- else if .ToolCalls }}<tool_call>",
	`{{ range .ToolCalls }}{"name": "{{ .Function.Name }}", "arguments": {{ .Function.Arguments }}}`,
	"{{ end }}</tool_call>",
	"{{- end }}{{ if not $last }}<|im_end|>",
	"{{ end }}",
	`{{- else if eq .Role "tool" }}<|im_start|>user`,
	"<tool_response>",
	"{{ .Content }}",
	"</tool_response><|im_end|>",
	"{{ end }}",
	`{{- if and (ne .Role "assistant") $last }}<|im_start|>assistant`,
	"{{ end }}",
	"{{- end }}",
	"{{- else }}",
	"{{- if .System }}<|im_start|>system",
	"{{ .System }}<|im_end|>",
	"{{ end }}{{ if .Prompt }}<|im_start|>user",
	"{{ .Prompt }}<|im_end|>",
	"{{ end }}<|im_start|>assistant",
	"{{ end }}{{ .Response }}{{ if .Response }}<|im_end|>{{ end }}",
}, "\n")
