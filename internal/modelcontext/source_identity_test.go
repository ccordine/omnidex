package modelcontext

import "testing"

func TestSourcePathLexerPreservesDivisionAndRegularExpressions(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`function ratio(left, right) { return left / right; }`,
		`const digits = /\d+\/\d+/;`,
		`const quoted = /["']foo\/bar/;`,
		`const privatePathPattern = /^\/private\/value$/;`,
		`function pattern() { return /\.\.\/private\/value/; }`,
		"const label = `${left / right}`;",
		`if value / denominator > 1 { return value; }`,
	} {
		if identities := SourcePathIdentities(source, ArtifactIdentityProvenance{}); len(identities) != 0 {
			t.Fatalf("parser-proven source %q produced identities %+v", source, identities)
		}
	}
}

func TestSourcePathLexerRejectsPathBearingLiteralsSpecifiersAndComments(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`import value from "foo/bar";`,
		`const location = "/workspace/generated";`,
		`$location = 'C:\\private\\value';`,
		"const location = `../private`;",
		"function f() { return `label \\` /private/value`; }",
		`const current = ".";`,
		`const parent = "..";`,
		"const rawParent = `..`;",
		`value := 1 // stored at /mnt/data`,
	} {
		if identities := SourcePathIdentities(source, ArtifactIdentityProvenance{}); len(identities) == 0 {
			t.Fatalf("path-bearing source %q was accepted", source)
		}
	}
}
