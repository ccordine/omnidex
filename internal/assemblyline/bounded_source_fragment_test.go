package assemblyline

import (
	"strings"
	"testing"
)

func TestBoundedSourceFragmentsEnforceExactSignatureAndOneDeclaration(t *testing.T) {
	for _, fixture := range boundedSourceFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			accepted, err := fixture.fragment(fixture.document.Blocks[1].Signature, fixture.candidate)
			if err != nil {
				t.Fatal(err)
			}
			if accepted != fixture.candidate {
				t.Fatalf("accepted declaration changed:\n%s", accepted)
			}

			changed := changedBoundedSourceDeclaration(fixture)
			if _, err := fixture.fragment(fixture.document.Blocks[1].Signature, changed); err == nil ||
				!strings.Contains(err.Error(), "does not match required signature") {
				t.Fatalf("changed signature error=%v", err)
			}
			if _, err := fixture.fragment(
				fixture.document.Blocks[1].Signature, fixture.candidate+"\n"+fixture.extra,
			); err == nil || !strings.Contains(err.Error(), "exactly one top-level declaration") {
				t.Fatalf("extra declaration error=%v", err)
			}
			broken := strings.TrimSuffix(fixture.candidate, "}")
			if _, err := fixture.fragment(fixture.document.Blocks[1].Signature, broken); err == nil ||
				!strings.Contains(err.Error(), "syntax rejected") {
				t.Fatalf("syntax error=%v", err)
			}
		})
	}
}

func TestBoundedSourceFragmentsRejectModelAuthoredDocumentWrappers(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		signature string
		candidate string
		validate  func(string, string) (string, error)
	}{
		{
			name: "javascript import", signature: "function feature()",
			candidate: "import value from 'module';\nfunction feature() {}",
			validate:  ValidateJavaScriptFragment,
		},
		{
			name: "java package", signature: "public int feature()",
			candidate: "package example;\npublic int feature() {}",
			validate:  ValidateJavaFragment,
		},
		{
			name: "rust use", signature: "fn feature()",
			candidate: "use std::fmt;\nfn feature() {}",
			validate:  ValidateRustFragment,
		},
		{
			name: "php tag", signature: "function feature(): void",
			candidate: "<?php\nfunction feature(): void {}",
			validate:  ValidatePHPFragment,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := testCase.validate(testCase.signature, testCase.candidate); err == nil {
				t.Fatal("model-authored document wrapper was accepted")
			}
		})
	}
}

func TestBoundedSourceFragmentsRejectBroaderDeclarationAuthority(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		signature string
		candidate string
		validate  func(string, string) (string, error)
	}{
		{name: "javascript export", signature: "function feature()", candidate: "export function feature() {}", validate: ValidateJavaScriptFragment},
		{name: "javascript class", signature: "class Feature", candidate: "class Feature {}", validate: ValidateJavaScriptFragment},
		{name: "java class", signature: "public class Feature", candidate: "public class Feature {}", validate: ValidateJavaFragment},
		{name: "rust impl", signature: "impl Feature", candidate: "impl Feature {}", validate: ValidateRustFragment},
		{name: "php class", signature: "class Feature", candidate: "class Feature {}", validate: ValidatePHPFragment},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := testCase.validate(testCase.signature, testCase.candidate); err == nil {
				t.Fatal("broader declaration authority was accepted")
			}
		})
	}
}

func TestJavaFragmentAcceptsOneExactMethodDeclaration(t *testing.T) {
	signature := "public static int add(int left, int right)"
	candidate := signature + " { return left + right; }"
	accepted, err := ValidateJavaFragment(signature, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != candidate {
		t.Fatalf("accepted Java method=%q", accepted)
	}
}

func TestBoundedSourceFragmentRejectsInvalidCodeOwnedSignature(t *testing.T) {
	for _, fixture := range boundedSourceFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			if _, err := fixture.fragment("not a declaration", fixture.candidate); err == nil ||
				!strings.Contains(err.Error(), "invalid code-owned") {
				t.Fatalf("invalid signature error=%v", err)
			}
			if _, err := fixture.fragment("line one\nline two", fixture.candidate); err == nil ||
				!strings.Contains(err.Error(), "one trimmed line") {
				t.Fatalf("multiline signature error=%v", err)
			}
		})
	}
}

func changedBoundedSourceDeclaration(fixture boundedSourceFixture) string {
	switch fixture.name {
	case "javascript":
		return strings.Replace(fixture.candidate, "Feature", "Changed", 1)
	case "java":
		return strings.Replace(fixture.candidate, "value", "changed", 1)
	case "rust":
		return strings.Replace(fixture.candidate, "feature", "changed", 1)
	case "php":
		return strings.Replace(fixture.candidate, "feature", "changed", 1)
	default:
		panic("unknown bounded source fixture")
	}
}
