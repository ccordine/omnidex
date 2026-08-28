package llm

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestQwen3NativeProfileSelectsExactStructureUnderOpaqueTags(t *testing.T) {
	t.Parallel()
	parameters := []string{
		exactQwen3NativeParametersTemperatureFirst,
		exactQwen3NativeParametersRepeatFirst,
	}
	for index, value := range parameters {
		selection, evidence := exactProviderProfileEvidence(
			t, "opaque-qwen3-"+string(rune('a'+index))+":local",
			exactQwen3NativeShowJSON(t, value, nil),
		)
		identity, err := DeriveExactProviderIdentityExpectation(evidence, selection)
		if err != nil {
			t.Fatalf("derive Qwen3 native parameter variant %d: %v", index, err)
		}
		if identity.Model != selection.Model ||
			identity.TokenizerProfile != ExactPreparedTokenizerProfileQwen3Native {
			t.Fatalf("variant %d identity=%+v", index, identity)
		}
	}
}

func TestQwen3NativeProfileRejectsStructuralDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*exactIdentityShowResponse)
	}{
		{"capabilities", func(show *exactIdentityShowResponse) {
			show.Capabilities = []string{"completion", "thinking"}
		}},
		{"template", func(show *exactIdentityShowResponse) { show.Template += " " }},
		{"parameters", func(show *exactIdentityShowResponse) {
			show.Parameters = strings.Replace(show.Parameters, "top_p                          0.95", "top_p                          0.94", 1)
		}},
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
				t, "opaque-qwen3-drift:local",
				exactQwen3NativeShowJSON(t, exactQwen3NativeParametersRepeatFirst, test.mutate),
			)
			if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatal("structurally drifted Qwen3 native profile was accepted")
			}
		})
	}
}

func TestQwen3NativePreparedRequestUsesSystemAndUncappedThinking(t *testing.T) {
	t.Parallel()
	expected := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
		Model: "opaque-qwen3-native:local", Digest: strings.Repeat("a", 64),
		Quantization: "Q4_K_M", NativeContextLimit: 8192,
		TokenizerProfile: ExactPreparedTokenizerProfileQwen3Native,
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("qwen3-native-request", expected)
	if err != nil {
		t.Fatal(err)
	}
	temperature := ExactPreparedTemperature(0.6)
	prepared := PreparedModel{
		Protocol: ExactPreparedProtocolRawTextV2, BaseModel: expected.Model,
		ContextModel: expected.Model, Prompt: "return one declaration",
		PromptHint: MinimalGeneratePrompt, MaxOutputTokens: expected.NativeContextLimit,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ContextTokens:   expected.NativeContextLimit, ThinkingEnabled: true, Temperature: &temperature,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"opaque-qwen3-native:local","options":{"num_ctx":8192,"temperature":0.6},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","think":true,"truncate":false}`
	if string(wire) != want {
		t.Fatalf("Qwen3 native exact request changed:\n got %s\nwant %s", wire, want)
	}
}

func TestQwen3NativeProfileHasNoModelTagAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("exact_provider_model_profile_registry.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"qwen3:0.6b", "qwen3:1.7b", "qwen3:8b"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("profile registry contains model-tag branch %q", forbidden)
		}
	}
}

func exactQwen3NativeShowJSON(
	t *testing.T,
	parameters string,
	mutate func(*exactIdentityShowResponse),
) string {
	t.Helper()
	show := exactIdentityShowResponse{
		Capabilities: []string{"completion", "tools", "thinking"},
		Template:     exactQwen3NativeTemplate, Parameters: parameters,
		ModelInfo: map[string]json.RawMessage{
			"general.architecture":         json.RawMessage(`"qwen3"`),
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

const exactQwen3NativeParametersTemperatureFirst = "temperature                    0.6\ntop_k                          20\ntop_p                          0.95\nrepeat_penalty                 1\nstop                           \"<|im_start|>\"\nstop                           \"<|im_end|>\""
const exactQwen3NativeParametersRepeatFirst = "repeat_penalty                 1\nstop                           \"<|im_start|>\"\nstop                           \"<|im_end|>\"\ntemperature                    0.6\ntop_k                          20\ntop_p                          0.95"

const exactQwen3NativeTemplate = `
{{- $lastUserIdx := -1 -}}
{{- range $idx, $msg := .Messages -}}
{{- if eq $msg.Role "user" }}{{ $lastUserIdx = $idx }}{{ end -}}
{{- end }}
{{- if or .System .Tools }}<|im_start|>system
{{ if .System }}
{{ .System }}
{{- end }}
{{- if .Tools }}

# Tools

You may call one or more functions to assist with the user query.

You are provided with function signatures within <tools></tools> XML tags:
<tools>
{{- range .Tools }}
{"type": "function", "function": {{ .Function }}}
{{- end }}
</tools>

For each function call, return a json object with function name and arguments within <tool_call></tool_call> XML tags:
<tool_call>
{"name": <function-name>, "arguments": <args-json-object>}
</tool_call>
{{- end -}}
<|im_end|>
{{ end }}
{{- range $i, $_ := .Messages }}
{{- $last := eq (len (slice $.Messages $i)) 1 -}}
{{- if eq .Role "user" }}<|im_start|>user
{{ .Content }}
{{- if and $.IsThinkSet (eq $i $lastUserIdx) }}
   {{- if $.Think -}}
      {{- " "}}/think
   {{- else -}}
      {{- " "}}/no_think
   {{- end -}}
{{- end }}<|im_end|>
{{ else if eq .Role "assistant" }}<|im_start|>assistant
{{ if (and $.IsThinkSet (and .Thinking (or $last (gt $i $lastUserIdx)))) -}}
<think>{{ .Thinking }}</think>
{{ end -}}
{{ if .Content }}{{ .Content }}
{{- else if .ToolCalls }}<tool_call>
{{ range .ToolCalls }}{"name": "{{ .Function.Name }}", "arguments": {{ .Function.Arguments }}}
{{ end }}</tool_call>
{{- end }}{{ if not $last }}<|im_end|>
{{ end }}
{{- else if eq .Role "tool" }}<|im_start|>user
<tool_response>
{{ .Content }}
</tool_response><|im_end|>
{{ end }}
{{- if and (ne .Role "assistant") $last }}<|im_start|>assistant
{{ if and $.IsThinkSet (not $.Think) -}}
<think>

</think>

{{ end -}}
{{ end }}
{{- end }}`
