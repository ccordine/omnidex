package llm

import "testing"

func TestExactLlama32PreparedRequestUsesDeterministicNaturalCompletion(t *testing.T) {
	t.Parallel()
	expected := ProviderIdentityExpectation{
		Backend: ExactPreparedProviderBackend, BackendVersion: "0.24.0",
		Model:        "opaque-llama:local",
		Digest:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Quantization: "Q4_K_M", NativeContextLimit: 8192,
		TokenizerProfile: ExactPreparedTokenizerProfileLlama32,
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("llama-request-contract", expected)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
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
	want := `{"model":"opaque-llama:local","options":{"num_ctx":8192,"temperature":0},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","truncate":false}`
	if string(wire) != want {
		t.Fatalf("Llama exact request changed:\n got %s\nwant %s", wire, want)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil || settings.Temperature == nil || *settings.Temperature != 0 {
		t.Fatalf("Llama transport settings=%+v error=%v", settings, err)
	}
}
