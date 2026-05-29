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
