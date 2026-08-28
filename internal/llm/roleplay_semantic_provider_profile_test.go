package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoleplaySemanticPolicyRequiresStructurallyAttestedCompletionModel(t *testing.T) {
	t.Parallel()
	selection, evidence := exactProviderProfileEvidence(
		t, "unregistered-semantic-model:latest", roleplayRawProviderShow,
	)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplaySemanticCompletion
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("semantic roleplay policy bypassed structural profile attestation")
	}

	selection, evidence = exactProviderProfileEvidence(
		t, "registered-semantic-model:latest", exactProviderQwen35Show,
	)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplaySemanticCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	if expected.TokenizerProfile != ExactPreparedTokenizerProfile {
		t.Fatalf("semantic tokenizer profile=%q", expected.TokenizerProfile)
	}
	reconstructed, err := ProviderIdentitySelectionForExpectation(expected)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.Model != selection.Model ||
		reconstructed.NativeContextLimit != selection.NativeContextLimit ||
		reconstructed.ProfilePolicy != "" {
		t.Fatalf("semantic strict selection reconstruction=%+v", reconstructed)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if settings.NativeTemplate || settings.SeparateSystem || settings.SeparateThinking ||
		!settings.NaturalOutputCeiling || settings.Temperature == nil || *settings.Temperature != 0 {
		t.Fatalf("semantic transport settings=%+v", settings)
	}

	prepared := PreparedModel{
		Protocol:  ExactPreparedProtocolRawTextV2,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "return one fictional semantic leaf", PromptHint: MinimalGeneratePrompt,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		Temperature:     settings.Temperature, RawTextStopSequence: ExactPreparedRawChatEndV1,
		ProviderIdentityExpectation:  &expected,
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
	if !request.Raw || request.System != "" ||
		!strings.HasPrefix(request.Prompt, ExactPreparedRawChatUserPrefixV1) ||
		request.Think == nil || *request.Think || request.Options.Temperature == nil ||
		*request.Options.Temperature != 0 {
		t.Fatalf("semantic prepared request=%s", raw)
	}
}

func TestRoleplaySemanticPolicyRetainsStrictTransportFloor(t *testing.T) {
	t.Parallel()
	selection := ProviderIdentitySelection{
		Model: "small-semantic-model:latest", NativeContextLimit: 4096,
		ProfilePolicy: ProviderIdentityProfileRoleplaySemanticCompletion,
	}
	if err := selection.Validate(); err == nil {
		t.Fatal("semantic completion policy accepted a context below the exact transport floor")
	}
}
