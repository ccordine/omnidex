package modelcontext

import (
	"reflect"
	"testing"
)

func TestQualifiedPathLexerRejectsCrossPlatformIdentities(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"/", "/workspace/generated", "/mnt/data", "/Users/alice", "foo/bar", "./", "../",
		"~/work", "~alice/work", "~build", `C:\work\value`, `C:work\value`, `C:relative`,
		`\\server\share\value`, `\\?\C:\private\value`, `\\.\PhysicalDrive0`,
		"file:///private/value", "https://example.com/resource", "application/json",
	} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if !ContainsPathIdentity(value) {
				t.Fatalf("qualified path %q was accepted", value)
			}
		})
	}
}

func TestQualifiedPathLexerRetainsBareSemanticDottedNames(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"Use Node.js with Vue.js.", "Use http.Client and time.Time.",
		"expected 3 but received 2", "version 1.25 remains valid",
	} {
		if ContainsPathIdentity(value) {
			t.Fatalf("path-free semantic text %q was rejected", value)
		}
	}
}

func TestPathIdentityProvenanceMatchesKnownPathsContainingSpaces(t *testing.T) {
	t.Parallel()
	provenance, err := NewArtifactIdentityProvenance([]string{"My Files/a.js"})
	if err != nil {
		t.Fatal(err)
	}
	identities := PathIdentities("Preserve My Files/a.js exactly.", provenance)
	if len(identities) != 1 || identities[0].Value != "My Files/a.js" {
		t.Fatalf("spaced path identities=%+v", identities)
	}
}

func TestLexicalPathTokensGrantNoIdentityButPreserveExactTokenBytes(t *testing.T) {
	t.Parallel()
	input := `Create "Docs/Proof Record.TXT" and mention Node.js.`
	tokens := LexicalPathTokens(input)
	values := make([]string, len(tokens))
	for index := range tokens {
		values[index] = tokens[index].Value
		if input[tokens[index].Start:tokens[index].End] != tokens[index].Value {
			t.Fatalf("token %d lost exact byte provenance", index)
		}
	}
	want := []string{"Create", "Docs/Proof Record.TXT", "and", "mention", "Node.js"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("tokens=%q want=%q", values, want)
	}
	if len(PathIdentities(input, ArtifactIdentityProvenance{})) != 1 {
		t.Fatal("lexical tokenization changed path-identity authority")
	}
}

func TestQualifiedPathLexerKeepsQuotedSpacedPathAtomic(t *testing.T) {
	t.Parallel()
	identities := PathIdentities(
		`Preserve "My Files/a.js" exactly.`, ArtifactIdentityProvenance{},
	)
	if len(identities) != 1 || identities[0].Value != "My Files/a.js" {
		t.Fatalf("quoted spaced path identities=%+v", identities)
	}
}

func TestPathIdentityProvenanceRecognizesOnlyExactKnownArtifacts(t *testing.T) {
	t.Parallel()
	provenance, err := NewArtifactIdentityProvenance([]string{
		"docs/REQUEST.md", "internal/transport.go", "ui/Node.js", "web/index.js", "worker/index.js",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"REQUEST.md", "transport.go", "Node.js", "docs/REQUEST.md"} {
		if !ContainsPathIdentityWithProvenance(value, provenance) {
			t.Fatalf("proven artifact %q was accepted", value)
		}
	}
	if ContainsPathIdentityWithProvenance("Vue.js", provenance) {
		t.Fatal("unproven Vue.js was treated as an artifact")
	}
	if ContainsPathIdentityWithProvenance("index.js", provenance) {
		t.Fatal("ambiguous basename was granted artifact provenance")
	}
	identities := PathIdentities("Update transport.go.", provenance)
	if len(identities) != 1 || identities[0].Value != "internal/transport.go" {
		t.Fatalf("resolved identities=%+v", identities)
	}
}

func TestArtifactIdentityProvenanceRejectsInvalidAuthority(t *testing.T) {
	t.Parallel()
	for _, paths := range [][]string{
		{"../secret"}, {"/absolute"}, {`windows\path`}, {"same", "same"},
	} {
		if _, err := NewArtifactIdentityProvenance(paths); err == nil {
			t.Fatalf("accepted invalid provenance %q", paths)
		}
	}
}
