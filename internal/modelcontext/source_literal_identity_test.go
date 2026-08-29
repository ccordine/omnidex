package modelcontext

import "testing"

func TestSourcePathLexerPreservesInterpretedNonPathEscapes(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"notification suffix": `func notification(title string) string {
  return "Notice: " + title + "\n"
}`,
		"terminal formatting": `function decorate(value) {
  return "\"" + value + "\"\r\n\t";
}`,
		"encoded label": `String label() {
  return "\x41\u0042\U00000043\u{44}";
}`,
		"continued source line": "const label = \"left\\\nright\";",
	} {
		source := source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if identities := SourcePathIdentities(source, ArtifactIdentityProvenance{}); len(identities) != 0 {
				t.Fatalf("non-path source escapes produced identities %+v in %q", identities, source)
			}
		})
	}
}

func TestSourcePathLexerRejectsEscapedAndEncodedPathSeparators(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]struct {
		source   string
		expected string
	}{
		"escaped Windows path": {
			source: `const value = "C:\\work\\file.txt";`, expected: `C:\\work\\file.txt`,
		},
		"escaped UNC path": {
			source: `const value = "\\\\server\\share\\file.txt";`, expected: `\\\\server\\share\\file.txt`,
		},
		"escaped forward separator": {
			source: `const value = "foo\/bar";`, expected: `foo\/bar`,
		},
		"hex encoded separators": {
			source: `const value = "\x2fprivate\x2fvalue";`, expected: `\x2fprivate\x2fvalue`,
		},
		"unicode encoded separators": {
			source: `const value = "\u002fprivate\u002fvalue";`, expected: `\u002fprivate\u002fvalue`,
		},
		"braced unicode encoded separators": {
			source: `const value = "\u{2f}srv\u{2f}data";`, expected: `\u{2f}srv\u{2f}data`,
		},
		"octal encoded separators": {
			source: `const value = "\057var\057data";`, expected: `\057var\057data`,
		},
		"encoded Windows drive path": {
			source: `const value = "C\u003a\u005cwork\u005cfile";`, expected: `C\u003a\u005cwork\u005cfile`,
		},
		"escaped backslash remains a separator": {
			source: `const value = "\\n";`, expected: `\\n`,
		},
	} {
		fixture := fixture
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			identities := SourcePathIdentities(fixture.source, ArtifactIdentityProvenance{})
			if len(identities) == 0 {
				t.Fatalf("path-bearing source %q was accepted", fixture.source)
			}
			identity := identities[0]
			if got := fixture.source[identity.Start:identity.End]; got != fixture.expected {
				t.Fatalf("raw source identity=%q, want %q (identity=%+v)", got, fixture.expected, identity)
			}
		})
	}
}

func TestSourcePathLexerMapsEncodedKnownArtifactToRawSource(t *testing.T) {
	t.Parallel()
	provenance, err := NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	source := `const value = "transport\u002ego";`
	identities := SourcePathIdentities(source, provenance)
	if len(identities) != 1 {
		t.Fatalf("encoded artifact identities=%+v", identities)
	}
	identity := identities[0]
	if identity.Value != "internal/transport.go" {
		t.Fatalf("resolved artifact identity=%q", identity.Value)
	}
	if got := source[identity.Start:identity.End]; got != `transport\u002ego` {
		t.Fatalf("raw artifact source identity=%q", got)
	}
}
