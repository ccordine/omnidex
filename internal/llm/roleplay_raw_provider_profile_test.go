package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

const roleplayRawProviderShow = `{
  "capabilities":["completion"],
  "parameters":"temperature 0.71\nrepeat_penalty 1.08",
  "template":"{{ .System }}\n{{ .Prompt }}",
  "model_info":{
    "general.architecture":"fictional-architecture",
    "fictional-architecture.context_length":4096,
    "tokenizer.ggml.model":"fictional-tokenizer",
    "tokenizer.ggml.pre":"fictional-pre"
  }
}`

func TestRoleplayRawProviderContextPolicyIsNarrowerThanStrictStations(t *testing.T) {
	raw := ProviderIdentitySelection{
		Model: "story-model:latest", NativeContextLimit: 4096,
		ProfilePolicy: ProviderIdentityProfileRoleplayRawCompletion,
	}
	if err := raw.Validate(); err != nil {
		t.Fatalf("roleplay raw selection rejected native 4096 context: %v", err)
	}
	strict := raw
	strict.ProfilePolicy = ""
	if err := strict.Validate(); err == nil {
		t.Fatal("strict provider selection accepted a context below the strict 8192-token floor")
	}

	rawExpectation := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
		Model: raw.Model, Digest: strings.Repeat("a", 64), Quantization: "Q4_0",
		NativeContextLimit: 4096, TokenizerProfile: ExactPreparedTokenizerProfileRoleplayRaw,
	}
	if err := rawExpectation.Validate(); err != nil {
		t.Fatalf("roleplay raw expectation rejected native 4096 context: %v", err)
	}
	strictExpectation := rawExpectation
	strictExpectation.TokenizerProfile = ExactPreparedTokenizerProfile
	if err := strictExpectation.Validate(); err == nil {
		t.Fatal("strict provider expectation accepted a context below the strict floor")
	}
}

func TestDeriveRoleplayRawContextLimitUsesExactModelMetadata(t *testing.T) {
	got, err := DeriveRoleplayRawContextLimit([]byte(roleplayRawProviderShow), 8192)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4096 {
		t.Fatalf("derived context=%d want 4096", got)
	}

	large := strings.Replace(
		roleplayRawProviderShow,
		`"fictional-architecture.context_length":4096`,
		`"fictional-architecture.context_length":262144`,
		1,
	)
	got, err = DeriveRoleplayRawContextLimit([]byte(large), 8192)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 {
		t.Fatalf("derived context=%d want configured 8192 ceiling", got)
	}
}

func TestDeriveRoleplayRawContextLimitRejectsInexactOrUnsupportedMetadata(t *testing.T) {
	for name, show := range map[string]string{
		"missing": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":4096,`,
			"",
			1,
		),
		"string": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":4096`,
			`"fictional-architecture.context_length":"4096"`,
			1,
		),
		"decimal": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":4096`,
			`"fictional-architecture.context_length":4096.0`,
			1,
		),
		"below roleplay floor": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":4096`,
			`"fictional-architecture.context_length":2048`,
			1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DeriveRoleplayRawContextLimit([]byte(show), 8192); err == nil {
				t.Fatal("unsupported roleplay model context metadata was accepted")
			}
		})
	}
}

func TestRoleplayRawProviderPolicyAcceptsUnregisteredCompletionModel(t *testing.T) {
	selection, evidence := exactProviderProfileEvidence(t, "story-model:latest", roleplayRawProviderShow)

	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("strict provider identity policy accepted an unregistered model profile")
	}
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	if expected.TokenizerProfile != ExactPreparedTokenizerProfileRoleplayRaw {
		t.Fatalf("unexpected roleplay tokenizer profile %q", expected.TokenizerProfile)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.NativeTemplate || settings.SeparateThinking || !settings.SeparateSystem ||
		settings.Temperature == nil || *settings.Temperature != 0.8 {
		t.Fatalf("unexpected roleplay local transport settings: %+v", settings)
	}

	reconstructed, err := ProviderIdentitySelectionForExpectation(expected)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed != selection {
		t.Fatalf("selection reconstruction changed: got %+v want %+v", reconstructed, selection)
	}
}

func TestRoleplayRawProviderUsesItsNativeSystemTemplate(t *testing.T) {
	selection, evidence := exactProviderProfileEvidence(t, "story-model:latest", roleplayRawProviderShow)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	temperature := ExactPreparedTemperature(0.8)
	prepared := PreparedModel{
		Protocol:  ExactPreparedProtocolStructuredV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "write one fictional response", PromptHint: MinimalGeneratePrompt,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ResponseFormat:  ResponseFormatJSON,
		ResponseSchema: map[string]any{
			"type": "object", "properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
		Temperature: &temperature, ProviderIdentityExpectation: &expected,
		ProviderObservationChallenge: strings.Repeat("b", 64),
	}
	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request exactPreparedRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.Raw || request.System != prepared.Prompt || request.Prompt != MinimalGeneratePrompt ||
		request.Think == nil || *request.Think {
		t.Fatalf("roleplay request bypassed its native instruction template: %s", raw)
	}
}

func TestRoleplayRawProviderPolicyStillRequiresCompletionCapability(t *testing.T) {
	show := strings.Replace(roleplayRawProviderShow, `"capabilities":["completion"]`, `"capabilities":["embedding"]`, 1)
	selection, evidence := exactProviderProfileEvidence(t, "embedding-model:latest", show)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("roleplay raw policy accepted a model without completion capability")
	}
}

func TestProviderIdentityProfilePolicyChangesDiscoveryChallenge(t *testing.T) {
	strict := ProviderIdentitySelection{Model: "story-model:latest", NativeContextLimit: 8192}
	raw := strict
	raw.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	strictChallenge, err := DeriveProviderIdentityDiscoveryChallenge("test:roleplay", strict)
	if err != nil {
		t.Fatal(err)
	}
	rawChallenge, err := DeriveProviderIdentityDiscoveryChallenge("test:roleplay", raw)
	if err != nil {
		t.Fatal(err)
	}
	if strictChallenge == rawChallenge {
		t.Fatal("strict and roleplay provider policies produced the same discovery authority")
	}
}
