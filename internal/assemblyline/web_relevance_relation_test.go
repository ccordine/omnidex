package assemblyline

import (
	"strings"
	"testing"
)

func webRelevanceFixture() WebRelevanceInput {
	return WebRelevanceInput{
		ExactQuestion: " Which release is current? ",
		Candidates: []WebRelevanceCandidate{
			{CandidateID: "C17", Title: "Old", Snippet: "Prior release", Excerpt: "Version 1 was superseded."},
			{CandidateID: "C31", Title: "Current", Snippet: "Stable release", Excerpt: "Version 2 is current."},
		},
		MaxSelections: 1,
	}
}

func TestWebRelevanceUsesOneRawCandidateRelationWithoutModelVisibleID(t *testing.T) {
	t.Parallel()
	base := webRelevanceFixture()
	input := WebRelevanceRelationInput{
		ExactQuestion: base.ExactQuestion,
		Context:       base.Context,
		Candidate:     base.Candidates[1],
	}
	job, err := NewWebRelevanceRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkWebRelevanceRelation {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.ExactQuestion) ||
		!strings.Contains(prompt, input.Candidate.Excerpt) {
		t.Fatalf("relation prompt lost exact bounded authority: %s", prompt)
	}
	for _, forbidden := range []string{
		input.Candidate.CandidateID, `"candidate_id"`, `"candidate_ids"`, `"schema"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relation prompt exposes code-owned field %q: %s", forbidden, prompt)
		}
	}
	decision, err := DecodeWebRelevanceRelationLeaf(input, string(WebCandidateRelevant))
	if err != nil || decision.Relation != WebCandidateRelevant {
		t.Fatalf("relation=%+v error=%v", decision, err)
	}
	assembled, err := AssembleWebRelevanceDecision(base, []string{input.Candidate.CandidateID})
	if err != nil {
		t.Fatal(err)
	}
	if assembled.Schema != WebRelevanceSchemaV1 || len(assembled.CandidateIDs) != 1 ||
		assembled.CandidateIDs[0] != input.Candidate.CandidateID {
		t.Fatalf("code-owned decision=%+v", assembled)
	}
	none, err := AssembleWebRelevanceDecision(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.CandidateIDs == nil || len(none.CandidateIDs) != 0 {
		t.Fatalf("code-owned empty decision=%+v", none)
	}
}

func TestWebRelevanceRelationRejectsStructuredAndUnsupportedResults(t *testing.T) {
	t.Parallel()
	base := webRelevanceFixture()
	input := WebRelevanceRelationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		Candidate: base.Candidates[0],
	}
	for _, raw := range []string{
		`{"relation":"RELEVANT"}`,
		`"RELEVANT"`,
		" RELEVANT ",
		"RELEVANT\nNOT_RELEVANT",
		"MAYBE",
	} {
		if _, err := DecodeWebRelevanceRelationLeaf(input, raw); err == nil {
			t.Fatalf("invalid raw relevance relation accepted: %q", raw)
		}
	}
	invalid := input
	invalid.Candidate.Excerpt = ""
	if _, err := NewWebRelevanceRelationJob(invalid); err == nil {
		t.Fatal("relevance relation without evidence excerpt was accepted")
	}
}
