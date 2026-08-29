package llm

import (
	"strings"
	"testing"
)

func TestExactRawChatFramingIdentityIsFrozenToQwen35V2Profile(t *testing.T) {
	t.Parallel()
	if got := exactPreparedRawChatFramingSHA256(); got != ExactPreparedRawChatFramingSHA256V1 {
		t.Fatalf("raw ChatML framing sha256=%s want %s", got, ExactPreparedRawChatFramingSHA256V1)
	}
	expected := providerIdentityTestExpectation()
	identity, err := ExactPreparedResponseFramingIdentity(expected)
	if err != nil {
		t.Fatal(err)
	}
	want := ExactPreparedRawChatFramingV1 + ":" +
		ExactPreparedRawChatFramingSHA256V1
	if identity != want ||
		expected.TokenizerProfile != "ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2" {
		t.Fatalf("raw framing identity=%q profile=%q", identity, expected.TokenizerProfile)
	}

	native := expected
	native.TokenizerProfile = ExactPreparedTokenizerProfileLlama32
	identity, err = ExactPreparedResponseFramingIdentity(native)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(identity, exactPreparedNativeTemplateFramingV1+":") {
		t.Fatalf("native framing identity=%q", identity)
	}
}
