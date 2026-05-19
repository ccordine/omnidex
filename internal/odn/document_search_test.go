package odn

import (
	"fmt"
	"strings"
	"testing"
)

func TestDocumentSearchNeedleInGeneratedHaystack(t *testing.T) {
	needle := "NEEDLE: subsystem=relay checksum=7f3a exact-section=137"
	document := buildHaystackDocument(240, 137, needle)
	expectedOffset := strings.Index(document, needle)
	if expectedOffset < 0 {
		t.Fatal("test document does not contain needle")
	}
	expectedLine, expectedColumn := lineColumnAtOffset(document, expectedOffset)

	result, err := SearchLargeDocument(document, needle, DocumentSearchConfig{
		ChunkChars:   900,
		ChunkOverlap: 120,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.Found {
		t.Fatal("needle was not found")
	}
	if result.ChunkCount < 10 {
		t.Fatalf("chunk count = %d, want a real haystack split", result.ChunkCount)
	}
	if len(result.Workers) != result.ChunkCount {
		t.Fatalf("workers = %d, chunks = %d", len(result.Workers), result.ChunkCount)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("hits = %d, want 1: %#v", len(result.Hits), result.Hits)
	}

	hit := result.Hits[0]
	if hit.StartOffset != expectedOffset {
		t.Fatalf("offset = %d, want %d", hit.StartOffset, expectedOffset)
	}
	if hit.Line != expectedLine || hit.Column != expectedColumn {
		t.Fatalf("line/column = %d/%d, want %d/%d", hit.Line, hit.Column, expectedLine, expectedColumn)
	}
	if !strings.Contains(hit.Excerpt, needle) {
		t.Fatalf("excerpt does not include needle: %q", hit.Excerpt)
	}
	if hit.ChunkID == "" {
		t.Fatal("hit did not record chunk id")
	}

	workerHitCount := 0
	for _, worker := range result.Workers {
		workerHitCount += len(worker.Hits)
		if len(worker.Hits) > 0 && worker.Chunk.ID != hit.ChunkID {
			t.Fatalf("unexpected worker chunk hit: worker=%s chunk=%s hit_chunk=%s", worker.WorkerID, worker.Chunk.ID, hit.ChunkID)
		}
	}
	if workerHitCount == 0 {
		t.Fatal("no worker reported the hit")
	}
}

func TestDocumentSearchFindsNeedleAcrossChunkBoundaryWithOverlap(t *testing.T) {
	prefix := strings.Repeat("a", 95)
	needle := "BOUNDARY-NEEDLE-EXACT"
	document := prefix + needle + strings.Repeat("b", 100)

	result, err := SearchLargeDocument(document, needle, DocumentSearchConfig{
		ChunkChars:   100,
		ChunkOverlap: len(needle) + 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || len(result.Hits) != 1 {
		t.Fatalf("boundary needle not found exactly once: %#v", result.Hits)
	}
	if result.Hits[0].StartOffset != len(prefix) {
		t.Fatalf("offset = %d, want %d", result.Hits[0].StartOffset, len(prefix))
	}
}

func TestDocumentSearchReportsNoHit(t *testing.T) {
	document := buildHaystackDocument(80, -1, "")
	result, err := SearchLargeDocument(document, "needle that is not present", DocumentSearchConfig{ChunkChars: 500, ChunkOverlap: 50})
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Fatalf("found = true, want false: %#v", result.Hits)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("hits = %#v, want none", result.Hits)
	}
	if len(result.Workers) != result.ChunkCount {
		t.Fatalf("workers = %d, chunks = %d", len(result.Workers), result.ChunkCount)
	}
}

func buildHaystackDocument(sections int, needleSection int, needle string) string {
	var b strings.Builder
	for i := 1; i <= sections; i++ {
		b.WriteString(fmt.Sprintf("## Section %03d\n", i))
		for paragraph := 0; paragraph < 4; paragraph++ {
			b.WriteString(fmt.Sprintf("This is generated paragraph %d for section %03d. It contains routine filler text about architecture, workers, context windows, verification, and command transcripts.\n", paragraph+1, i))
		}
		if i == needleSection {
			b.WriteString(needle)
			b.WriteString("\n")
		}
	}
	return b.String()
}
