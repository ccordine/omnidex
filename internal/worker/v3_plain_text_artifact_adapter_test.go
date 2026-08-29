package worker

import (
	"testing"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPlainTextArtifactAdapterRecognizesNormalizedUnrelatedTextPathsWithoutOverlap(t *testing.T) {
	t.Parallel()

	for _, artifactPath := range []string{
		"docs/release-notes.txt",
		"fixtures/astronomy.txt",
		"legal/NOTICE.txt",
		".gitignore",
		"containers/.dockerignore",
	} {
		t.Run(artifactPath, func(t *testing.T) {
			matches := make([]string, 0, 1)
			var kind assemblyline.TargetArtifactKind
			for _, adapter := range registeredDirectCodingArtifactAdapters() {
				candidateKind, recognized := adapter.Recognize(artifactPath)
				if !recognized {
					continue
				}
				matches = append(matches, adapter.ID)
				kind = candidateKind
			}
			if len(matches) != 1 || matches[0] != assemblyline.PlainTextAdapterID {
				t.Fatalf("path %q adapter matches=%v", artifactPath, matches)
			}
			if kind != assemblyline.TargetArtifactImplementation {
				t.Fatalf("path %q kind=%q", artifactPath, kind)
			}
		})
	}
}

func TestPlainTextArtifactRecognizerRejectsUnnormalizedAndUnrelatedPaths(t *testing.T) {
	t.Parallel()

	for _, artifactPath := range []string{
		"README.md",
		"notes.text",
		"notes.txt.bak",
		".txt",
		"../notes.txt",
		"/tmp/notes.txt",
		"docs//notes.txt",
		"docs/./notes.txt",
		"docs/notes.txt/",
		" docs/notes.txt",
		`docs\notes.txt`,
	} {
		t.Run(artifactPath, func(t *testing.T) {
			if kind, recognized := plainTextArtifactRecognizer(artifactPath); recognized || kind != "" {
				t.Fatalf("unsafe or unrelated path %q recognized as %q", artifactPath, kind)
			}
		})
	}
}

func TestPlainTextArtifactAdapterRegistersOneComposer(t *testing.T) {
	t.Parallel()

	registered := 0
	var adapter directCodingArtifactAdapter
	for _, candidate := range registeredDirectCodingArtifactAdapters() {
		if candidate.ID != assemblyline.PlainTextAdapterID {
			continue
		}
		registered++
		adapter = candidate
	}
	if registered != 1 {
		t.Fatalf("plain-text adapter registrations=%d", registered)
	}
	if adapter.ComposeDocument == nil {
		t.Fatal("plain-text adapter has no document composer")
	}
	if adapter.Validation.Kind != directCodingArtifactStructural ||
		adapter.Validation.Execute == nil {
		t.Fatalf("plain-text validation=%+v", adapter.Validation)
	}

	document := sourceDocumentForPlainTextAdapterTest()
	composed, err := adapter.ComposeDocument(document, assemblyline.SourceComposition{
		Generated:  map[string]string{"notice.statement": "Astronomy fixture ready.\n"},
		Interfaces: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if composed.Path != document.Path || composed.Source != "Astronomy fixture ready.\n" {
		t.Fatalf("composed=%+v", composed)
	}
}

func TestPlainTextArtifactValidationUsesExactTextNodePolicy(t *testing.T) {
	t.Parallel()

	adapter, err := directCodingArtifactAdapterByID(assemblyline.PlainTextAdapterID)
	if err != nil {
		t.Fatal(err)
	}
	valid := []byte("Astronomy: caf\xc3\xa9 \xe2\x9c\x93  \n")
	if err := adapter.Validation.Execute("fixtures/astronomy.txt", valid); err != nil {
		t.Fatalf("valid text rejected by adapter validator: %v", err)
	}
	if err := validateDirectCodingArtifactSource(
		adapter, "fixtures/astronomy.txt", valid,
	); err != nil {
		t.Fatalf("valid text rejected by full adapter boundary: %v", err)
	}

	invalidUTF8 := []byte{'o', 'k', 0xff, '\n'}
	if utf8.Valid(invalidUTF8) {
		t.Fatal("invalid UTF-8 fixture is valid")
	}
	for name, source := range map[string][]byte{
		"invalid UTF-8":         invalidUTF8,
		"NUL":                   []byte("notice\x00\n"),
		"CRLF":                  []byte("notice\r\n"),
		"missing terminal LF":   []byte("notice"),
		"multiple terminal LFs": []byte("notice\n\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := adapter.Validation.Execute("legal/NOTICE.txt", source); err == nil {
				t.Fatal("invalid text passed the adapter validator")
			}
			if err := validateDirectCodingArtifactSource(
				adapter, "legal/NOTICE.txt", source,
			); err == nil {
				t.Fatal("invalid text passed the full adapter boundary")
			}
		})
	}
}

func sourceDocumentForPlainTextAdapterTest() assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "astronomy_notice", Path: "fixtures/astronomy.txt",
		AdapterID: assemblyline.PlainTextAdapterID,
		Blocks: []assemblyline.SourceBlock{{
			ID: "notice.statement", Signature: assemblyline.TextFragmentSignature,
			Contract: "State that the astronomy fixture is ready.",
			API:      "plain UTF-8 text node", TaskID: "task_001",
			Role: assemblyline.SourceBlockTaskImplementation,
		}},
	}
}
