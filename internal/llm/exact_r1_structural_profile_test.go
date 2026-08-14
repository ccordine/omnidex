package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

const (
	exactR1Qwen3ProfileID    = "ollama-0.24.0-qwen3-qwen2-boundary-v1"
	exactR1Qwen2BOSProfileID = "ollama-0.24.0-qwen2-qwen2-bos-boundary-v1"

	exactR1TemplateSHA256      = "c5ad996bda6eed4df6e3b605a9869647624851ac248209d22fd5e2c0cc1121d3"
	exactR18BParametersSHA256  = "b3669fdd1d59cec39ead1d59150e66c4791f54b7ad95d034521d2011670ad2e1"
	exactR114BParametersSHA256 = "52cf8b77cb8c5fe26ae7cbb18482e4c5ff22a861b6ee8e6e7a7085e7f6844e2f"
)

const exactR1Template = `{{- if .System }}{{ .System }}{{ end }}
{{- range $i, $_ := .Messages }}
{{- $last := eq (len (slice $.Messages $i)) 1}}
{{- if eq .Role "user" }}<｜User｜>{{ .Content }}
{{- else if eq .Role "assistant" }}<｜Assistant｜>
  {{- if and $.IsThinkSet (and $last .Thinking) -}}
<think>
{{ .Thinking }}
</think>
{{- end }}{{ .Content }}{{- if not $last }}<｜end▁of▁sentence｜>{{- end }}
{{- end }}
{{- if and $last (ne .Role "assistant") }}<｜Assistant｜>
{{- if and $.IsThinkSet (not $.Think) -}}
<think>

</think>

{{ end }}
{{- end -}}
{{- end }}`

const exactR1ControlStops = `stop                           "<｜begin▁of▁sentence｜>"
stop                           "<｜end▁of▁sentence｜>"
stop                           "<｜User｜>"
stop                           "<｜Assistant｜>"`

const exactR18BParameters = exactR1ControlStops + `
temperature                    0.6
top_p                          0.95`

const exactR114BParameters = exactR1ControlStops

func TestDeriveExactProviderIdentityExpectationAcceptsClosedR1StructuralProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		model         string
		architecture  string
		addBOS        bool
		parameters    string
		parametersSHA string
		wantProfile   string
	}{
		{
			name: "installed eight billion boundary under opaque tag", model: "opaque-alpha:local",
			architecture: "qwen3", addBOS: false, parameters: exactR18BParameters,
			parametersSHA: exactR18BParametersSHA256, wantProfile: exactR1Qwen3ProfileID,
		},
		{
			name: "installed fourteen billion boundary under opaque tag", model: "opaque-beta:local",
			architecture: "qwen2", addBOS: true, parameters: exactR114BParameters,
			parametersSHA: exactR114BParametersSHA256, wantProfile: exactR1Qwen2BOSProfileID,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertExactR1FixtureDigest(t, exactR1Template, exactR1TemplateSHA256)
			assertExactR1FixtureDigest(t, test.parameters, test.parametersSHA)
			show := exactR1Show(t, test.architecture, test.addBOS, test.parameters, nil)
			selection, evidence := exactProviderProfileEvidence(t, test.model, show)
			got, err := DeriveExactProviderIdentityExpectation(evidence, selection)
			if err != nil {
				t.Fatalf("derive registered R1 structural profile: %v", err)
			}
			if got.Model != test.model || got.TokenizerProfile != test.wantProfile {
				t.Fatalf("identity=%+v want model=%q profile=%q", got, test.model, test.wantProfile)
			}
		})
	}
}

func TestDeriveExactProviderIdentityExpectationRejectsR1ProfileDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		architecture string
		addBOS       bool
		parameters   string
		template     string
		capabilities []string
	}{
		{
			name: "eight billion template digest", architecture: "qwen3", addBOS: false,
			parameters: exactR18BParameters, template: exactR1Template + " ",
		},
		{
			name: "eight billion parameters digest", architecture: "qwen3", addBOS: false,
			parameters: strings.Replace(exactR18BParameters, "top_p                          0.95", "top_p                          0.94", 1),
		},
		{
			name: "eight billion capabilities", architecture: "qwen3", addBOS: false,
			parameters: exactR18BParameters, capabilities: []string{"completion", "thinking", "tools"},
		},
		{
			name: "fourteen billion template digest", architecture: "qwen2", addBOS: true,
			parameters: exactR114BParameters, template: exactR1Template + " ",
		},
		{
			name: "fourteen billion parameters digest", architecture: "qwen2", addBOS: true,
			parameters: exactR114BParameters + "\ntemperature                    0.6",
		},
		{
			name: "fourteen billion BOS boundary", architecture: "qwen2", addBOS: false,
			parameters: exactR114BParameters,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			template := test.template
			if template == "" {
				template = exactR1Template
			}
			show := exactR1Show(t, test.architecture, test.addBOS, test.parameters, test.capabilities)
			if template != exactR1Template {
				show = exactR1ShowWithTemplate(t, test.architecture, test.addBOS, test.parameters, template, test.capabilities)
			}
			selection, evidence := exactProviderProfileEvidence(t, "opaque-drift:local", show)
			if got, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
				t.Fatalf("drifted R1 provider profile was accepted: %+v", got)
			}
		})
	}
}

func exactR1Show(
	t *testing.T,
	architecture string,
	addBOS bool,
	parameters string,
	capabilities []string,
) string {
	t.Helper()
	return exactR1ShowWithTemplate(t, architecture, addBOS, parameters, exactR1Template, capabilities)
}

func exactR1ShowWithTemplate(
	t *testing.T,
	architecture string,
	addBOS bool,
	parameters string,
	template string,
	capabilities []string,
) string {
	t.Helper()
	if capabilities == nil {
		capabilities = []string{"completion", "thinking"}
	}
	info := map[string]any{
		"general.architecture":         architecture,
		"tokenizer.ggml.model":         "gpt2",
		"tokenizer.ggml.pre":           "qwen2",
		"tokenizer.ggml.add_bos_token": addBOS,
		"tokenizer.ggml.add_eos_token": false,
		"tokenizer.ggml.tokens":        nil,
		"tokenizer.ggml.token_type":    nil,
		"tokenizer.ggml.merges":        nil,
	}
	raw, err := json.Marshal(map[string]any{
		"capabilities": capabilities,
		"model_info":   info,
		"parameters":   parameters,
		"template":     template,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertExactR1FixtureDigest(t *testing.T, value string, want string) {
	t.Helper()
	digest := sha256.Sum256([]byte(value))
	if got := hex.EncodeToString(digest[:]); got != want {
		t.Fatalf("R1 fixture digest=%s want=%s", got, want)
	}
}
