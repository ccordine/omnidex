package assemblyline

import (
	"strings"
	"testing"
)

func TestContextRelevanceSelectionReturnsOneRawOpaqueLeaf(t *testing.T) {
	authority := contextRelevanceSelectionFixture(t)
	input := ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{},
	}
	job, err := NewContextRelevanceSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "NO_RELEVANT_CANDIDATE") ||
		strings.Contains(prompt, "Return only the opaque candidate IDs") {
		t.Fatalf("prompt=%q", prompt)
	}
	decision, err := DecodeContextRelevanceSelectionDecision(
		input, authority.CandidateAuthorities[0].CandidateID,
	)
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := AssembleContextRelevanceDecision(
		authority, []string{decision.CandidateID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(assembled.ReferencedCandidateIDs) != 1 ||
		assembled.ReferencedCandidateIDs[0] != decision.CandidateID {
		t.Fatalf("assembled=%+v", assembled)
	}
}

func TestContextRelevanceSelectionRejectsStructuredDuplicateAndUnknownLeaves(t *testing.T) {
	authority := contextRelevanceSelectionFixture(t)
	accepted := authority.CandidateAuthorities[0].CandidateID
	input := ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{accepted},
	}
	for _, raw := range []string{
		`{"candidate_id":"` + authority.CandidateAuthorities[1].CandidateID + `"}`,
		accepted,
		"CTX_999999",
	} {
		if _, err := DecodeContextRelevanceSelectionDecision(input, raw); err == nil {
			t.Fatalf("accepted invalid leaf %q", raw)
		}
	}
	if decision, err := DecodeContextRelevanceSelectionDecision(
		input, ContextRelevanceNoCandidate,
	); err != nil || decision.CandidateID != ContextRelevanceNoCandidate {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func contextRelevanceSelectionFixture(t *testing.T) ContextRelevanceInput {
	t.Helper()
	return ContextRelevanceInput{
		ExactInstruction:  "Do it again.",
		RetrievalConcepts: []string{"previous action"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(
				t, "conversation", "CTX_7",
				"USER: Water the greenhouse beds. ASSISTANT: Done.",
			),
			contextCandidateFixture(
				t, "memory", "CTX_3",
				"Use rye flour for the bakery's morning loaves.",
			),
		},
		MaxSelections: 2,
	}
}
