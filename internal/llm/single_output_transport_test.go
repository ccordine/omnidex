package llm

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestEveryThinkingCapableProfileDisablesProviderThinking(t *testing.T) {
	t.Parallel()
	for _, profile := range exactProviderModelProfiles {
		profile := profile
		if !slices.Contains(profile.capabilities, "thinking") {
			continue
		}
		t.Run(profile.tokenizerProfile, func(t *testing.T) {
			t.Parallel()
			expected := ProviderIdentityExpectation{
				Backend: ExactPreparedProviderBackend, BackendVersion: ExactPreparedProviderVersion,
				Model: "opaque-single-output:local", Digest: strings.Repeat("a", 64),
				Quantization: "Q4_K_M", NativeContextLimit: 8192,
				TokenizerProfile: profile.tokenizerProfile,
			}
			challenge, err := DeriveProviderIdentityObservationChallenge(
				"single-output-profile:"+profile.tokenizerProfile, expected,
			)
			if err != nil {
				t.Fatal(err)
			}
			prepared := PreparedModel{
				Protocol: ExactPreparedProtocolRawTextV2, BaseModel: expected.Model,
				ContextModel: expected.Model, Prompt: "return one semantic leaf",
				PromptHint: MinimalGeneratePrompt, MaxOutputTokens: 1024,
				OutputLimitMode: ExactPreparedOutputLimitNatural, ContextTokens: 8192,
				ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
			}
			if profile.requestTemperatureSet {
				temperature := profile.requestTemperature
				prepared.Temperature = &temperature
			}
			if profile.transport == exactPreparedTransportRaw {
				prepared.RawTextStopSequence = ExactPreparedRawChatEndV1
			}
			wire, err := ExactPreparedRequestBytes(prepared)
			if err != nil {
				t.Fatal(err)
			}
			var request exactPreparedRequest
			if err := json.Unmarshal(wire, &request); err != nil {
				t.Fatal(err)
			}
			if request.Think == nil || *request.Think {
				t.Fatalf("thinking-capable profile did not send think=false: %s", wire)
			}
		})
	}
}
