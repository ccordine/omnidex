package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoleplaySemanticPolicyAdmitsArbitraryCompletionModelDeterministically(t *testing.T) {
	t.Parallel()
	selection, evidence := exactProviderProfileEvidence(
		t, "unregistered-semantic-model:latest", roleplayRawProviderShow,
	)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplaySemanticCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	if expected.TokenizerProfile != ExactPreparedTokenizerProfileRoleplaySemantic {
		t.Fatalf("semantic tokenizer profile=%q", expected.TokenizerProfile)
	}
	reconstructed, err := ProviderIdentitySelectionForExpectation(expected)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed != selection {
		t.Fatalf("semantic selection reconstruction=%+v want %+v", reconstructed, selection)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.NativeTemplate || !settings.SeparateSystem || settings.SeparateThinking ||
		settings.Temperature == nil || *settings.Temperature != 0 {
		t.Fatalf("semantic transport settings=%+v", settings)
	}
	if next, ok, err := NextExactPreparedTemperature(expected, settings.Temperature); err != nil {
		t.Fatal(err)
	} else if ok || next != nil {
		t.Fatalf("deterministic semantic profile advanced to %v", next)
	}

	prepared := PreparedModel{
		Protocol:  ExactPreparedProtocolStructuredV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "return one fictional semantic leaf", PromptHint: MinimalGeneratePrompt,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ResponseFormat:  ResponseFormatJSON,
		ResponseSchema: map[string]any{
			"type": "object", "properties": map[string]any{
				"leaf": map[string]any{"type": "string"},
			},
		},
		Temperature: settings.Temperature, ProviderIdentityExpectation: &expected,
		ProviderObservationChallenge: strings.Repeat("c", 64),
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
		request.Think == nil || *request.Think || request.Options.Temperature == nil ||
		*request.Options.Temperature != 0 {
		t.Fatalf("semantic prepared request=%s", raw)
	}
}

func TestRoleplaySemanticPolicyUsesNarrowCompletionContextFloor(t *testing.T) {
	t.Parallel()
	selection := ProviderIdentitySelection{
		Model: "small-semantic-model:latest", NativeContextLimit: 4096,
		ProfilePolicy: ProviderIdentityProfileRoleplaySemanticCompletion,
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("semantic completion context rejected: %v", err)
	}
}
