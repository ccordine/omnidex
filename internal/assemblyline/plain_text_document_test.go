package assemblyline

import (
	"strings"
	"testing"
)

func TestComposePlainTextDocumentPreservesValidatedBlockBytesAndSpans(t *testing.T) {
	t.Parallel()

	document := plainTextDocumentFixture()
	blueprint := SourceBlueprint{Documents: []SourceDocument{document}}
	if err := ValidatePlainTextSourceBlueprint(blueprint); err != nil {
		t.Fatal(err)
	}
	generated := "Deterministic proof: café ✓  \n"
	composed, err := ComposePlainTextDocument(document, SourceComposition{
		Generated:  map[string]string{"proof.statement": generated},
		Interfaces: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Proof record\n" + generated
	if composed.ID != document.ID || composed.Path != document.Path || composed.Source != want {
		t.Fatalf("composed=%+v want_source=%q", composed, want)
	}
	if got := composed.Spans["proof.heading"]; got != (SourceSpan{StartLine: 1, EndLine: 1}) {
		t.Fatalf("heading span=%+v", got)
	}
	if got := composed.Spans["proof.statement"]; got != (SourceSpan{StartLine: 2, EndLine: 2}) {
		t.Fatalf("statement span=%+v", got)
	}
}

func TestPlainTextDocumentRejectsInvalidStructure(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*SourceDocument){
		"unsupported path": func(document *SourceDocument) {
			document.Path = "proof.bin"
		},
		"preamble": func(document *SourceDocument) {
			document.Preamble = "header"
		},
		"scoped preamble": func(document *SourceDocument) {
			document.ScopedPreambles = []SourcePreamble{{TaskID: "task_001", Source: "header"}}
		},
		"postamble": func(document *SourceDocument) {
			document.Postamble = "footer"
		},
		"wrong generated signature": func(document *SourceDocument) {
			document.Blocks[1].Signature = "whole_document"
		},
		"dependency": func(document *SourceDocument) {
			document.Blocks[1].DependsOn = []string{"proof.heading"}
		},
		"capability": func(document *SourceDocument) {
			document.Blocks[1].Capabilities = []string{"proof.heading"}
		},
		"global": func(document *SourceDocument) {
			document.Blocks[1].Globals = []string{"value"}
		},
		"export": func(document *SourceDocument) {
			document.Blocks[1].Export = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := plainTextDocumentFixture()
			document.Blocks = append([]SourceBlock(nil), document.Blocks...)
			mutate(&document)
			if err := ValidatePlainTextSourceBlueprint(SourceBlueprint{
				Documents: []SourceDocument{document},
			}); err == nil {
				t.Fatal("invalid plain-text document was accepted")
			}
		})
	}
}

func TestComposePlainTextDocumentRejectsInvalidGeneratedText(t *testing.T) {
	t.Parallel()

	document := plainTextDocumentFixture()
	for name, generated := range map[string]map[string]string{
		"missing": {},
		"NUL":     {"proof.statement": "proof\x00\n"},
		"CRLF":    {"proof.statement": "proof\r\n"},
		"no LF":   {"proof.statement": "proof"},
		"two LFs": {"proof.statement": "proof\n\n"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ComposePlainTextDocument(document, SourceComposition{
				Generated: generated, Interfaces: map[string]string{},
			})
			if err == nil {
				t.Fatal("invalid generated plain text was composed")
			}
		})
	}
}

func TestPlainTextBlueprintAcceptsRegisteredStableTextPaths(t *testing.T) {
	t.Parallel()

	for _, artifactPath := range []string{"proof.txt", "docs/NOTICE.TXT", ".gitignore", ".dockerignore"} {
		document := plainTextDocumentFixture()
		document.Path = artifactPath
		if err := ValidatePlainTextSourceBlueprint(SourceBlueprint{
			Documents: []SourceDocument{document},
		}); err != nil {
			t.Fatalf("path %q rejected: %v", artifactPath, err)
		}
	}
}

func plainTextDocumentFixture() SourceDocument {
	return SourceDocument{
		ID: "proof_text", Path: "proof.txt", AdapterID: "plain_text",
		Blocks: []SourceBlock{
			{
				ID: "proof.heading", Static: "Proof record\n",
				API: "plain UTF-8 text node",
			},
			{
				ID: "proof.statement", Signature: TextFragmentSignature,
				Contract: "State that the deterministic proof completed successfully.",
				API:      "plain UTF-8 text node", TaskID: "task_001",
				Role: SourceBlockTaskImplementation,
			},
		},
	}
}

func TestPlainTextComposerDoesNotNormalizeAcceptedWhitespace(t *testing.T) {
	t.Parallel()

	document := plainTextDocumentFixture()
	accepted := "  leading and trailing spaces are content  \n"
	composed, err := ComposePlainTextDocument(document, SourceComposition{
		Generated: map[string]string{"proof.statement": accepted}, Interfaces: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(composed.Source, accepted) {
		t.Fatalf("composer normalized accepted whitespace: %q", composed.Source)
	}
}
