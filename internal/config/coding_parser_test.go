package config

import (
	"os"
	"testing"
)

func TestLocalParserDoesNotInheritLargeNativeInferenceContext(t *testing.T) {
	t.Setenv("INFERENCE_CONTEXT_TOKENS", "131072")
	t.Setenv("CODING_LEDGER_PARSER_CONTEXT_TOKENS", "")
	if err := os.Unsetenv("CODING_LEDGER_PARSER_CONTEXT_TOKENS"); err != nil {
		t.Fatal(err)
	}
	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.InferenceContextTokens != "131072" || configuration.CodingParserContextTokens != "8192" {
		t.Fatalf("local parser inherited another route's context allocation: native=%s parser=%s", configuration.InferenceContextTokens, configuration.CodingParserContextTokens)
	}
}
