package worker

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedObjectiveEvidenceTextBoundsBeforeProjection(t *testing.T) {
	value, err := boundedObjectiveEvidenceText(64, "Title", "Snippet", strings.Repeat("界", 10_000))
	if err != nil {
		t.Fatal(err)
	}
	if len(value) > 64 || !utf8.ValidString(value) || !strings.HasSuffix(value, objectiveEvidenceTruncationMarker) {
		t.Fatalf("projection bytes=%d valid=%v value=%q", len(value), utf8.ValidString(value), value)
	}
	exact, err := boundedObjectiveEvidenceText(64, "Title", "Snippet", "Content")
	if err != nil || exact != "Title\nSnippet\nContent" {
		t.Fatalf("exact projection=%q err=%v", exact, err)
	}
}
