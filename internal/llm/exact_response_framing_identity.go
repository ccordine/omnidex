package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	ExactPreparedRawChatFramingV1        = "omnidex.qwen35-raw-chatml-framing.v1"
	ExactPreparedRawChatFramingSHA256V1  = "85b4cab8356c7571d3061504af0e74f0126dbf8644d5925e4a6cad4a90717a26"
	exactPreparedNativeTemplateFramingV1 = "omnidex.provider-native-template-framing.v1"
)

// ExactPreparedResponseFramingIdentity binds code-owned raw framing, or one
// structurally attested provider-native template profile, into discovery
// challenges. There is no wildcard or evidence-hash-only framing profile.
func ExactPreparedResponseFramingIdentity(
	expected ProviderIdentityExpectation,
) (string, error) {
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return "", err
	}
	profile, err := exactProviderModelProfileByID(expected.TokenizerProfile)
	if err != nil {
		return "", err
	}
	if profile.transport == exactPreparedTransportRaw {
		if expected.TokenizerProfile != ExactPreparedTokenizerProfile {
			return "", fmt.Errorf("raw provider profile has no registered response framing")
		}
		return ExactPreparedRawChatFramingV1 + ":" +
			ExactPreparedRawChatFramingSHA256V1, nil
	}
	return exactPreparedNativeTemplateFramingV1 + ":" + expected.TokenizerProfile, nil
}

func exactPreparedRawChatFramingSHA256() string {
	hash := sha256.New()
	for index, value := range []string{
		ExactPreparedRawChatFramingV1,
		ExactPreparedRawChatUserPrefixV1,
		ExactPreparedRawChatAssistantBoundaryV1,
		ExactPreparedRawChatEndV1,
	} {
		if index > 0 {
			_, _ = hash.Write([]byte{0})
		}
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
