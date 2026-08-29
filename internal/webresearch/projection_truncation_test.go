package webresearch

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestBuildProjectionCarriesExplicitTruncationAuthority(t *testing.T) {
	evidence := []Evidence{{
		ID: "E1", CandidateID: websearch.CandidateID("C1"),
		Title: "Exact title", Snippet: "Exact snippet", Content: strings.Repeat("evidence ", 200),
	}}
	projected, err := buildProjection(
		evidence, 256, assemblyline.ArtifactIdentityProvenance{},
	)
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

func TestWebModelProjectionRedactsPathsButLeavesURLInCodeOwnedEvidence(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{
		"internal/private/owner.go", "docs/private.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{
		ID: "E1", CandidateID: websearch.CandidateID("C1"),
		URL: "https://example.test/source/path", Title: "owner.go reference",
		Snippet: "Compare /etc/passwd.", Content: "The details are in docs/private.md.",
	}}
	candidates, err := buildRelevanceCandidates(evidence, 512, provenance)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := buildProjection(evidence, 2048, provenance)
	if err != nil {
		t.Fatal(err)
	}
	modelVisible := strings.Join([]string{
		candidates[0].Title, candidates[0].Snippet, candidates[0].Excerpt,
		projected[0].Title, projected[0].Snippet, projected[0].Content,
	}, "\n")
	for _, leaked := range []string{"owner.go", "/etc/passwd", "docs/private.md", evidence[0].URL} {
		if strings.Contains(modelVisible, leaked) {
			t.Fatalf("web model projection leaked %q: %s", leaked, modelVisible)
		}
	}
	if strings.Count(modelVisible, "ARTIFACT_REF") < 3 {
		t.Fatalf("web paths were not explicitly redacted: %s", modelVisible)
	}
	if evidence[0].URL != "https://example.test/source/path" {
		t.Fatalf("code-owned citation URL changed: %#v", evidence[0])
	}
}

func TestBuildProjectionPreservesUntruncatedEvidence(t *testing.T) {
	evidence := []Evidence{{
		ID: "E1", CandidateID: websearch.CandidateID("C1"),
		Title: "Title", Snippet: "Snippet", Content: "Exact evidence.",
	}}
	projected, err := buildProjection(
		evidence, 256, assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if projected[0].Truncated || projected[0].Content != evidence[0].Content {
		t.Fatalf("exact projection changed: %#v", projected[0])
	}
}

func TestBuildRelevanceCandidatesMarksEveryClippedField(t *testing.T) {
	candidates, err := buildRelevanceCandidates([]Evidence{{
		CandidateID: websearch.CandidateID("C1"),
		Title:       strings.Repeat("t", 100), Snippet: strings.Repeat("s", 100),
		Content: strings.Repeat("c", 100),
	}}, 90, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		t.Fatal(err)
	}
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
