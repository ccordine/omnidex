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
    "fictional-architecture.context_length":8192,
    "tokenizer.ggml.model":"fictional-tokenizer",
    "tokenizer.ggml.pre":"fictional-pre"
  }
}`

func TestRoleplayRawProviderContextPolicyRetainsStrictTransportFloor(t *testing.T) {
	raw := ProviderIdentitySelection{
		Model: "story-model:latest", NativeContextLimit: 8192,
		ProfilePolicy: ProviderIdentityProfileRoleplayRawCompletion,
	}
	if err := raw.Validate(); err != nil {
		t.Fatalf("roleplay raw selection rejected native 8192 context: %v", err)
	}
	belowFloor := raw
	belowFloor.NativeContextLimit = 4096
	if err := belowFloor.Validate(); err == nil {
		t.Fatal("roleplay provider selection accepted a context below the exact transport floor")
	}

	strictExpectation := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
		Model: raw.Model, Digest: strings.Repeat("a", 64), Quantization: "Q4_0",
		NativeContextLimit: 4096, TokenizerProfile: ExactPreparedTokenizerProfile,
	}
	if err := strictExpectation.Validate(); err == nil {
		t.Fatal("roleplay selection policy bypassed the strict provider expectation floor")
	}
}

func TestDeriveRoleplayCompletionContextLimitUsesExactModelMetadata(t *testing.T) {
	got, err := DeriveRoleplayCompletionContextLimit([]byte(roleplayRawProviderShow), 16384)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 {
		t.Fatalf("derived context=%d want 8192", got)
	}

	large := strings.Replace(
		roleplayRawProviderShow,
		`"fictional-architecture.context_length":8192`,
		`"fictional-architecture.context_length":262144`,
		1,
	)
	got, err = DeriveRoleplayCompletionContextLimit([]byte(large), 8192)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 {
		t.Fatalf("derived context=%d want configured 8192 ceiling", got)
	}
}

func TestDeriveRoleplayCompletionContextLimitRejectsInexactOrUnsupportedMetadata(t *testing.T) {
	for name, show := range map[string]string{
		"missing": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":8192,`,
			"",
			1,
		),
		"string": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":8192`,
			`"fictional-architecture.context_length":"8192"`,
			1,
		),
		"decimal": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":8192`,
			`"fictional-architecture.context_length":8192.0`,
			1,
		),
		"below roleplay floor": strings.Replace(
			roleplayRawProviderShow,
			`"fictional-architecture.context_length":8192`,
			`"fictional-architecture.context_length":4096`,
			1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DeriveRoleplayCompletionContextLimit([]byte(show), 8192); err == nil {
				t.Fatal("unsupported roleplay model context metadata was accepted")
			}
		})
	}
}

func TestRoleplayRawProviderPolicyStillRequiresRegisteredStructuralProfile(t *testing.T) {
	selection, evidence := exactProviderProfileEvidence(t, "story-model:latest", roleplayRawProviderShow)

	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("strict provider identity policy accepted an unregistered model profile")
	}
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("roleplay policy bypassed structural profile attestation")
	}

	selection, evidence = exactProviderProfileEvidence(
		t, "story-model:latest", exactProviderQwen35Show,
	)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	if expected.TokenizerProfile != ExactPreparedTokenizerProfile {
		t.Fatalf("unexpected roleplay tokenizer profile %q", expected.TokenizerProfile)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil {
		t.Fatal(err)
	}
	if settings.NativeTemplate || settings.SeparateThinking || settings.SeparateSystem ||
		!settings.NaturalOutputCeiling || settings.Temperature == nil || *settings.Temperature != 0 {
		t.Fatalf("unexpected roleplay local transport settings: %+v", settings)
	}

	reconstructed, err := ProviderIdentitySelectionForExpectation(expected)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.Model != selection.Model ||
		reconstructed.NativeContextLimit != selection.NativeContextLimit ||
		reconstructed.ProfilePolicy != "" {
		t.Fatalf("strict expectation reconstructed invalid selection: %+v", reconstructed)
	}
}

func TestRoleplayRawProviderUsesItsStructurallyAttestedTransport(t *testing.T) {
	selection, evidence := exactProviderProfileEvidence(t, "story-model:latest", exactProviderQwen35Show)
	selection.ProfilePolicy = ProviderIdentityProfileRoleplayRawCompletion
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	temperature := ExactPreparedTemperature(0)
	prepared := PreparedModel{
		Protocol:  ExactPreparedProtocolRawTextV2,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "write one fictional response", PromptHint: MinimalGeneratePrompt,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		Temperature:     &temperature, RawTextStopSequence: ExactPreparedRawChatEndV1,
		ProviderIdentityExpectation:  &expected,
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
	if !request.Raw || request.System != "" ||
		!strings.HasPrefix(request.Prompt, ExactPreparedRawChatUserPrefixV1) ||
		request.Think == nil || *request.Think || request.Options.NumPredict != 8192 {
		t.Fatalf("roleplay request bypassed its attested raw transport: %s", raw)
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
