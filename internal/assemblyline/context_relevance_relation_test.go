package assemblyline

import (
	"strings"
	"testing"
)

func TestContextRelevanceRelationProjectsOneCandidateWithoutCodeOwnedIdentity(t *testing.T) {
	t.Parallel()
	candidate := contextCandidateFixture(
		t, "conversation", "CTX_7", "  Yesterday we tuned the telescope.\n",
	)
	input := ContextRelevanceRelationInput{
		ExactInstruction:   "Do it again.",
		Candidate:          candidate,
		Scope:              ContextScopeRoleplaySimulation,
		KnownArtifactPaths: []string{},
	}
	job, err := NewContextRelevanceRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkContextRelevanceRelation {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"candidate_content":"  Yesterday we tuned the telescope.\n"`) {
		t.Fatalf("relevance projection=%s", prompt)
	}
	for _, hidden := range []string{
		candidate.CandidateID,
		candidate.Namespace,
		candidate.ContentSHA256,
		string(ContextScopeRoleplaySimulation),
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("relevance prompt leaked code-owned candidate authority %q", hidden)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("portable relevance payload lost candidate authority %q", hidden)
		}
	}
	for _, forbidden := range []string{
		`"candidate_id"`, `"namespace"`, `"content_sha256"`, `"scope"`,
		"NO_RELEVANT_CANDIDATE", "most necessary", "not-yet-accepted",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relevance prompt exposed forbidden selection concept %q", forbidden)
		}
	}
}

func TestContextRelevanceRelationReturnsOnlyOneRegisteredBoundResult(t *testing.T) {
	t.Parallel()
	input := contextRelevanceRelationFixture(t, "CTX_7", "The greenhouse beds were watered.")
	result, err := DecodeContextRelevanceRelationResult(
		input, ContextCandidateDirectlyRelevant,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Relation != ContextCandidateDirectlyRelevant ||
		result.Schema != ContextRelevanceRelationSchemaV1 ||
		result.AuthoritySHA256 == "" {
		t.Fatalf("result=%+v", result)
	}
	for name, raw := range map[string]string{
		"old candidate ID": "CTX_7",
		"old completion":   "NO_RELEVANT_CANDIDATE",
		"JSON":             `{"relation":"DIRECTLY_RELEVANT_TO_EXACT_INSTRUCTION"}`,
		"quoted":           `"DIRECTLY_RELEVANT_TO_EXACT_INSTRUCTION"`,
		"unknown":          "RELEVANT",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeContextRelevanceRelationResult(input, raw); err == nil {
				t.Fatal("invalid context relevance relation was accepted")
			}
		})
	}
	other := contextRelevanceRelationFixture(t, "CTX_8", "Use rye flour for morning loaves.")
	if err := result.ValidateFor(other); err == nil || !strings.Contains(err.Error(), "authority hash") {
		t.Fatalf("cross-candidate result validation error=%v", err)
	}
}

func TestContextRelevanceRelationRejectsInvalidSingleCandidateAuthority(t *testing.T) {
	t.Parallel()
	base := contextRelevanceRelationFixture(t, "CTX_1", "The vents were opened.")
	tests := map[string]func(*ContextRelevanceRelationInput){
		"namespace": func(input *ContextRelevanceRelationInput) {
			input.Candidate.Namespace = "Conversation History"
		},
		"ID": func(input *ContextRelevanceRelationInput) {
			input.Candidate.CandidateID = "memory:1"
		},
		"hash": func(input *ContextRelevanceRelationInput) {
			input.Candidate.ContentSHA256 = strings.Repeat("0", 64)
		},
		"oversized content": func(input *ContextRelevanceRelationInput) {
			input.Candidate.Content = strings.Repeat("x", MaxContextCandidateContentBytes+1)
			input.Candidate.ContentSHA256 = ExactObjectiveContextSHA(input.Candidate.Content)
		},
		"missing provenance": func(input *ContextRelevanceRelationInput) {
			input.KnownArtifactPaths = nil
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := NewContextRelevanceRelationJob(input); err == nil {
				t.Fatal("invalid context relevance authority was accepted")
			}
		})
	}
}

func contextRelevanceRelationFixture(
	t *testing.T,
	id string,
	content string,
) ContextRelevanceRelationInput {
	t.Helper()
	return ContextRelevanceRelationInput{
		ExactInstruction:   "Do it again.",
		Candidate:          contextCandidateFixture(t, "conversation", id, content),
		KnownArtifactPaths: []string{},
	}
}

func contextCandidateFixture(
	t *testing.T,
	namespace string,
	id string,
	content string,
) ContextCandidateAuthority {
	t.Helper()
	authority, err := NewContextCandidateAuthority(namespace, id, content)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
