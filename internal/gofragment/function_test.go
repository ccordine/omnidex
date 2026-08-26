package gofragment

import (
	"strings"
	"testing"
)

func TestParseFunctionPreservesSignatureAndCapabilityBoundary(t *testing.T) {
	t.Parallel()
	contract := Contract{
		Signature: "func Value() int", Current: "func Value() int { return 1 }",
		PermittedSymbols: []string{"func Helper() int"},
	}
	parsed, err := ParseFunction(contract, "func Value() int { return Helper() }")
	if err != nil {
		t.Fatal(err)
	}
	if parsed != "func Value() int {\n\treturn Helper()\n}" {
		t.Fatalf("parsed=%q", parsed)
	}
	if _, err := ParseFunction(contract, "func Value() string { return \"x\" }"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("changed signature error=%v", err)
	}
	if _, err := ParseFunction(contract, "func Value() int { return Danger() }"); err == nil || !strings.Contains(err.Error(), "undeclared capability") {
		t.Fatalf("undeclared capability error=%v", err)
	}
}

func TestParseFunctionDiagnosticsArePathFreeAndCommentsAreForbidden(t *testing.T) {
	t.Parallel()
	contract := Contract{Signature: "func Value() int", Current: "func Value() int { return 1 }"}
	_, err := ParseFunction(contract, "func Value() int { return + }")
	if err == nil || strings.Contains(err.Error(), ".go") || strings.Contains(err.Error(), "/") {
		t.Fatalf("path-bearing or missing Go fragment diagnostic: %v", err)
	}
	for _, candidate := range []string{
		"// generated explanation\nfunc Value() int { return 2 }",
		"//go:noinline\nfunc Value() int { return 2 }",
		"//line /tmp/authority.go:20\nfunc Value() int { return 2 }",
		"func Value() int { /* hidden behavior */ return 2 }",
	} {
		if _, err := ParseFunction(contract, candidate); err == nil || !strings.Contains(err.Error(), "comments") {
			t.Fatalf("comment-bearing candidate %q error=%v", candidate, err)
		}
	}
}

func TestParseNewFunctionUsesImmutableCodeOwnedSignature(t *testing.T) {
	t.Parallel()
	parsed, err := ParseNewFunction(
		"func Added(value int) int", nil,
		"func Added(value int) int { return value + 1 }",
	)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != "func Added(value int) int {\n\treturn value + 1\n}" {
		t.Fatalf("parsed=%q", parsed)
	}
	if _, err := ParseNewFunction(
		"func Added(value int) int", nil,
		"func Renamed(value int) int { return value + 1 }",
	); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("changed signature error=%v", err)
	}
}

func TestSelfContainedNewFunctionSignatureRejectsUnresolvedImportBeforeGeneration(t *testing.T) {
	t.Parallel()
	if err := RequireSelfContainedNewFunctionSignature("func Added(time.Time) string"); err == nil ||
		!strings.Contains(err.Error(), "unresolved import/type authority") {
		t.Fatalf("qualified signature error=%v", err)
	}
	if err := RequireSelfContainedNewFunctionSignature("func Added(value int) string"); err != nil {
		t.Fatal(err)
	}
}

func TestParseNewFunctionOwnsSyntaxInsteadOfArtifactIdentity(t *testing.T) {
	t.Parallel()
	for _, candidate := range []string{
		`func Added() string { return "Node.js" }`,
		`func Added() string { return "./config/value.json" }`,
	} {
		if _, err := ParseNewFunction("func Added() string", nil, candidate); err != nil {
			t.Fatalf("syntax parser applied an artifact-identity policy to %q: %v", candidate, err)
		}
	}
}
