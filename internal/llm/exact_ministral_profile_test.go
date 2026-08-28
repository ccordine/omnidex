package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

const exactMinistralTemplate = `{{- $lastUserIndex := -1 }}
{{- $hasSystemPrompt := false }}
{{- range $index, $_ := .Messages }}
{{- if eq .Role "user" }}{{ $lastUserIndex = $index }}{{ end }}
{{- if eq .Role "system" }}{{ $hasSystemPrompt = true }}{{ end }}
{{- end }}
{{- if not $hasSystemPrompt }}[SYSTEM_PROMPT]You are Ministral-3-8B-Instruct-2512, a Large Language Model (LLM) created by Mistral AI, a French startup headquartered in Paris.
You power an AI assistant called Le Chat.
Your knowledge base was last updated on 2023-10-01.
The current date is {{ currentDate }}.

When you're not sure about some information or when the user's request requires up-to-date or specific data, you must use the available tools to fetch the information. Do not hesitate to use tools whenever they can provide a more accurate or complete response. If no relevant tools are available, then clearly state that you don't have the information and avoid making up anything.
If the user's question is not clear, ambiguous, or does not provide enough context for you to accurately answer the question, you do not try to answer it right away and you rather ask the user to clarify their request (e.g. "What are some good restaurants around me?" => "Where are you?" or "When is the next flight to Tokyo" => "Where do you travel from?").
You are always very attentive to dates, in particular you try to resolve dates (e.g. "yesterday" is {{ yesterdayDate }}) and when asked about information at specific dates, you discard information that is at another date.
You follow these instructions in all languages, and always respond to the user in the language they use or request.
Next sections describe the capabilities that you have.

# WEB BROWSING INSTRUCTIONS

You cannot perform any web search or access internet to open URLs, links etc. If it seems like the user is expecting you to do so, you clarify the situation and ask the user to copy paste the text directly in the chat.

# MULTI-MODAL INSTRUCTIONS

You have the ability to read images, but you cannot generate images. You also cannot transcribe audio files or videos.
You cannot read nor transcribe audio files or videos.

# TOOL CALLING INSTRUCTIONS

You may have access to tools that you can use to fetch information or perform actions. You must use these tools in the following situations:

1. When the request requires up-to-date information.
2. When the request requires specific data that you do not have in your knowledge base.
3. When the request involves actions that you cannot perform without tools.

Always prioritize using tools to provide the most accurate and helpful response. If tools are not available, inform the user that you cannot perform the requested action at the moment.[/SYSTEM_PROMPT]
{{- end }}
{{- range $index, $_ := .Messages }}
{{- if eq .Role "system" }}[SYSTEM_PROMPT]{{ .Content }}[/SYSTEM_PROMPT]
{{- else if eq .Role "user" }}
{{- if and (eq $lastUserIndex $index) $.Tools }}[AVAILABLE_TOOLS]{{ $.Tools }}[/AVAILABLE_TOOLS]
{{- end }}[INST]{{ .Content }}[/INST]
{{- else if eq .Role "assistant" }}
{{- if .Content }}{{ .Content }}
{{- if and (not .ToolCalls) (not (eq (len (slice $.Messages $index)) 1)) }}</s>
{{- end }}
{{- end }}
{{- if .ToolCalls }}[TOOL_CALLS]
{{- range .ToolCalls }}{{ .Function.Name }}[ARGS]{{ .Function.Arguments }}
{{- end }}</s>
{{- end }}
{{- else if eq .Role "tool" }}[TOOL_RESULTS]{{ .Content }}[/TOOL_RESULTS]
{{- end }}
{{- end }}
`

const (
	exactMinistralParameters       = "temperature                    0.15"
	exactMinistralTemplateSHA256   = "5b74c26b9e0b6358e73d0a7eb2f955105097e7bd10b45fa0d2a50f0a906e0798"
	exactMinistralParametersSHA256 = "e96fd3cee0f18f63a5df2dc1115f0bdf681fd2c9f75aa0f082f840f974111b9a"
)

func TestDeriveExactProviderIdentityExpectationAcceptsClosedMinistralProfile(t *testing.T) {
	t.Parallel()
	assertExactR1FixtureDigest(t, exactMinistralTemplate, exactMinistralTemplateSHA256)
	assertExactR1FixtureDigest(t, exactMinistralParameters, exactMinistralParametersSHA256)
	show := exactMinistralShow(t, exactMinistralTemplate, exactMinistralParameters)
	selection, evidence := exactProviderProfileEvidence(t, "opaque-middleweight:local", show)
	got, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatalf("derive registered Ministral structural profile: %v", err)
	}
	if got.Model != selection.Model || got.TokenizerProfile != ExactPreparedTokenizerProfileMistral3 {
		t.Fatalf("derived identity=%+v", got)
	}
}

func TestDeriveExactProviderIdentityExpectationRejectsMinistralProfileDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		show string
	}{
		{"template", exactMinistralShow(t, exactMinistralTemplate+" ", exactMinistralParameters)},
		{"parameters", exactMinistralShow(t, exactMinistralTemplate, "temperature                    0.16")},
		{"capabilities", strings.Replace(exactMinistralShow(t, exactMinistralTemplate, exactMinistralParameters), `"tools"]`, `"tools","thinking"]`, 1)},
		{"token boundary", strings.Replace(exactMinistralShow(t, exactMinistralTemplate, exactMinistralParameters), `"tokenizer.ggml.add_unknown_token":false`, `"tokenizer.ggml.add_unknown_token":true`, 1)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selection, evidence := exactProviderProfileEvidence(t, "opaque-drift:local", test.show)
			if got, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatalf("drifted Ministral provider profile was accepted: %+v", got)
			}
		})
	}
}

func TestExactMinistralPreparedRequestUsesNativeSystemAndNaturalCompletion(t *testing.T) {
	t.Parallel()
	show := exactMinistralShow(t, exactMinistralTemplate, exactMinistralParameters)
	selection, evidence := exactProviderProfileEvidence(t, "opaque-middleweight:local", show)
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("ministral-request-contract", expected)
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedModel{
		Protocol: ExactPreparedProtocolRawTextV2, BaseModel: expected.Model,
		ContextModel: expected.Model, Prompt: "return one declaration",
		PromptHint: MinimalGeneratePrompt, MaxOutputTokens: 1024,
		OutputLimitMode:             ExactPreparedOutputLimitNatural,
		ContextTokens:               expected.NativeContextLimit,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatalf("render attested Ministral request: %v", err)
	}
	want := `{"model":"opaque-middleweight:local","options":{"num_ctx":8192},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","truncate":false}`
	if string(wire) != want {
		t.Fatalf("Ministral exact request changed:\n got %s\nwant %s", wire, want)
	}
	var request map[string]any
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	if _, exists := request["think"]; exists {
		t.Fatalf("Ministral request supplied unsupported thinking control: %s", wire)
	}
	options := request["options"].(map[string]any)
	for _, forbidden := range []string{"num_predict", "temperature", "stop"} {
		if _, exists := options[forbidden]; exists {
			t.Fatalf("Ministral request overrode native %s: %s", forbidden, wire)
		}
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil || !settings.NativeTemplate || !settings.SeparateSystem ||
		settings.SeparateThinking || settings.Temperature != nil {
		t.Fatalf("Ministral transport settings=%+v error=%v", settings, err)
	}
}

func exactMinistralShow(t *testing.T, template string, parameters string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"capabilities": []string{"completion", "vision", "tools"},
		"model_info": map[string]any{
			"general.architecture": "mistral3", "tokenizer.ggml.model": "gpt2",
			"tokenizer.ggml.pre": "default", "tokenizer.ggml.add_bos_token": true,
			"tokenizer.ggml.add_eos_token": false, "tokenizer.ggml.add_padding_token": false,
			"tokenizer.ggml.add_unknown_token": false, "tokenizer.ggml.tokens": nil,
			"tokenizer.ggml.token_type": nil, "tokenizer.ggml.merges": nil,
		},
		"parameters": parameters, "template": template,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
