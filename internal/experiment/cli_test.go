package experiment

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestStreamCopyPreservesLargeOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buffer := &boundedBuffer{cancel: cancel}
	// Hide WriterTo so io.Copy can choose a destination ReaderFrom method.
	// Embedding bytes.Buffer here would bypass our Write bound completely.
	reader := struct{ io.Reader }{strings.NewReader(strings.Repeat("x", MaxStreamBytes+1))}
	if _, err := io.Copy(buffer, reader); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != MaxStreamBytes+1 || buffer.overflow || ctx.Err() != nil {
		t.Fatalf("stream copy lost output or canceled: bytes=%d overflow=%t context=%v", buffer.Len(), buffer.overflow, ctx.Err())
	}
}
