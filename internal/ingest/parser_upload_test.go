package ingest

import (
	"os"
	"strings"
	"testing"
)

func TestParseUploadText(t *testing.T) {
	parsed, err := ParseUpload("notes.md", []byte("# Title\n\nSome content here."))
	if err != nil {
		t.Fatalf("parse upload: %v", err)
	}
	if !strings.Contains(parsed.Content, "Some content here.") {
		t.Fatalf("unexpected content: %q", parsed.Content)
	}
}

func TestDocumentFormatTagHasNoFilenameSemanticTokens(t *testing.T) {
	tag, err := DocumentFormatTag("markdown")
	if err != nil || tag != "document-format:markdown" {
		t.Fatalf("format tag=%q error=%v", tag, err)
	}
	for _, value := range []string{"", " markdown", "MARKDOWN", "postgres", "react"} {
		if _, err := DocumentFormatTag(value); err == nil {
			t.Fatalf("unknown/inexact document format %q was accepted", value)
		}
	}
	raw, err := os.ReadFile("parser.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"InferTagsFromPath", "strings.FieldsFunc(base", "parts :="} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("document ingest retains filename semantic tagging %q", forbidden)
		}
	}
}

func TestParseUploadPDFUsesTempFile(t *testing.T) {
	if _, err := ParseUpload("sample.pdf", []byte("%PDF-1.4\n")); err == nil {
		t.Fatal("expected invalid pdf to fail")
	}
}

func TestChunkTextProducesMultipleChunks(t *testing.T) {
	content := strings.Repeat("word ", 400)
	chunks := ChunkText(content, 100, 10)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestParseFileUnsupported(t *testing.T) {
	tmp, err := os.CreateTemp("", "omni-ingest-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()
	if _, err := ParseFile(tmp.Name() + ".bin"); err == nil {
		t.Fatal("expected unsupported extension error")
	}
}
