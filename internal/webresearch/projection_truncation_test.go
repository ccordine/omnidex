package webresearch

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestBuildProjectionCarriesExplicitTruncationAuthority(t *testing.T) {
	evidence := []Evidence{{
		ID: "E1", CandidateID: websearch.CandidateID("C1"),
		Title: "Exact title", Snippet: "Exact snippet", Content: strings.Repeat("evidence ", 200),
	}}
	projected, err := buildProjection(evidence, 256)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || !projected[0].Truncated ||
		!strings.HasSuffix(projected[0].Content, projectionTruncationMarker) {
		t.Fatalf("projection did not expose truncation: %#v", projected)
	}
	if projectionBytes(projected[0]) > 256 {
		t.Fatalf("projection bytes=%d, want <=256", projectionBytes(projected[0]))
	}
	marked, err := applyProjectionTruncation(evidence, projected)
	if err != nil {
		t.Fatal(err)
	}
	if len(marked) != 1 || !marked[0].Truncated {
		t.Fatalf("acquisition authority did not retain projection truncation: %#v", marked)
	}
}

func TestBuildProjectionPreservesUntruncatedEvidence(t *testing.T) {
	evidence := []Evidence{{
		ID: "E1", CandidateID: websearch.CandidateID("C1"),
		Title: "Title", Snippet: "Snippet", Content: "Exact evidence.",
	}}
	projected, err := buildProjection(evidence, 256)
	if err != nil {
		t.Fatal(err)
	}
	if projected[0].Truncated || projected[0].Content != evidence[0].Content {
		t.Fatalf("exact projection changed: %#v", projected[0])
	}
}

func TestBuildRelevanceCandidatesMarksEveryClippedField(t *testing.T) {
	candidates := buildRelevanceCandidates([]Evidence{{
		CandidateID: websearch.CandidateID("C1"),
		Title:       strings.Repeat("t", 100), Snippet: strings.Repeat("s", 100),
		Content: strings.Repeat("c", 100),
	}}, 90)
	if len(candidates) != 1 {
		t.Fatalf("candidates=%#v", candidates)
	}
	for name, value := range map[string]string{
		"title": candidates[0].Title, "snippet": candidates[0].Snippet, "excerpt": candidates[0].Excerpt,
	} {
		if !strings.HasSuffix(value, relevanceTruncationMarker) {
			t.Fatalf("%s truncation was silent: %q", name, value)
		}
	}
}
