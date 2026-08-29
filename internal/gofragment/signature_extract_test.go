package gofragment

import (
	"strings"
	"testing"
)

func TestExtractUniqueNewFunctionSignatureUsesOnlyExplicitSyntax(t *testing.T) {
	t.Parallel()
	got, err := ExtractUniqueNewFunctionSignature("Add func Added(value int) (int, error) so callers can use it.")
	if err != nil {
		t.Fatal(err)
	}
	if got.Canonical != "func Added(value int) (int, error)" || got.Name != "Added" {
		t.Fatalf("signature=%+v", got)
	}
	if got.Source != "func Added(value int) (int, error)" || got.StartByte < 1 || got.EndByte <= got.StartByte {
		t.Fatalf("signature source authority=%+v", got)
	}
}

func TestExtractUniqueNewFunctionSignatureRejectsSemanticGuessAndAmbiguity(t *testing.T) {
	t.Parallel()
	for _, authority := range []string{
		"Add a helper that returns two.",
		"Add func First() and func Second().",
		strings.Repeat("x", maxGoSignatureAuthorityBytes+1),
	} {
		if _, err := ExtractUniqueNewFunctionSignature(authority); err == nil {
			t.Fatalf("invalid signature authority accepted: %q", authority)
		}
	}
}
