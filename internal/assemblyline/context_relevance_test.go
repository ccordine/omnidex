package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestContextRelevanceSelectionPromptHidesCandidateProvenance(t *testing.T) {
	t.Parallel()
	authority := ContextRelevanceInput{
		ExactInstruction:  "Hello.",
		RetrievalConcepts: []string{"prior telescope tuning"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_1", "  Yesterday we tuned the telescope.\n"),
			contextCandidateFixture(t, "roleplay.canon", "CTX_2", "The northern gate opens at dawn."),
		},
		MaxSelections: 2,
	}
	input := ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{},
	}
	job, err := NewContextRelevanceSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkContextRelevanceSelection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"candidate_id":"CTX_2"`) ||
		!strings.Contains(prompt, `"retrieval_concepts":["prior telescope tuning"]`) ||
		!strings.Contains(prompt, `"content":"  Yesterday we tuned the telescope.\n"`) {
		t.Fatalf("relevance projection=%s", prompt)
	}
	for _, hidden := range []string{
		authority.CandidateAuthorities[0].Namespace,
		authority.CandidateAuthorities[0].ContentSHA256,
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("relevance prompt leaked code-owned candidate provenance %q", hidden)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("portable relevance payload lost candidate provenance %q", hidden)
		}
	}
	for _, forbidden := range []string{`"namespace"`, `"content_sha256"`, `"provider"`, `"source"`} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relevance prompt exposed forbidden authority field %q", forbidden)
		}
	}
	leaf, err := DecodeContextRelevanceSelectionDecision(
		input, ContextRelevanceNoCandidate,
	)
	if err != nil || leaf.CandidateID != ContextRelevanceNoCandidate {
		t.Fatalf("leaf=%+v error=%v", leaf, err)
	}
	decision, err := AssembleContextRelevanceDecision(authority, []string{})
	if err != nil || decision.ReferencedCandidateIDs == nil {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestContextRelevanceSelectionReturnsOnlyOneProjectedOpaqueID(t *testing.T) {
	t.Parallel()
	authority := ContextRelevanceInput{
		ExactInstruction:  "Do it again.",
		RetrievalConcepts: []string{"previous action"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_7", "The greenhouse beds were watered."),
			contextCandidateFixture(t, "memory", "CTX_3", "Use rye flour for morning loaves."),
		},
		MaxSelections: 1,
	}
	input := ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{},
	}
	leaf, err := DecodeContextRelevanceSelectionDecision(input, "CTX_7")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := AssembleContextRelevanceDecision(authority, []string{leaf.CandidateID})
	if err != nil || len(decision.ReferencedCandidateIDs) != 1 ||
		decision.ReferencedCandidateIDs[0] != "CTX_7" {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
	for name, raw := range map[string]string{
		"unknown ID": "CTX_9",
		"JSON":       `{"candidate_id":"CTX_7"}`,
		"quoted":     `"CTX_7"`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeContextRelevanceSelectionDecision(input, raw); err == nil {
				t.Fatal("invalid relevance leaf was accepted")
			}
		})
	}
	input.AcceptedCandidateIDs = []string{"CTX_7"}
	if _, err := DecodeContextRelevanceSelectionDecision(input, "CTX_7"); err == nil {
		t.Fatal("accepted candidate was selected twice")
	}
}

func TestContextRelevanceSelectionRejectsInvalidAuthority(t *testing.T) {
	t.Parallel()
	base := ContextRelevanceInput{
		ExactInstruction:  "Repeat the previous greenhouse operation.",
		RetrievalConcepts: []string{"previous greenhouse operation"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_1", "The vents were opened."),
			contextCandidateFixture(t, "memory", "CTX_2", "The irrigation cycle ran."),
		},
		MaxSelections: 2,
	}
	newJob := func(input ContextRelevanceInput) error {
		_, err := NewContextRelevanceSelectionJob(ContextRelevanceSelectionInput{
			Authority: input, AcceptedCandidateIDs: []string{},
		})
		return err
	}
	for name, concepts := range map[string][]string{
		"nil": nil, "mixed case": {"Previous action"}, "unsorted": {"zulu", "alpha"},
		"duplicate": {"previous action", "previous action"}, "over bound": {"a", "b", "c", "d"},
	} {
		t.Run("retrieval concepts "+name, func(t *testing.T) {
			input := base
			input.RetrievalConcepts = concepts
			if err := newJob(input); err == nil {
				t.Fatal("noncanonical retrieval concepts were accepted")
			}
		})
	}
	emptyConceptInput := base
	emptyConceptInput.RetrievalConcepts = []string{}
	emptyConceptJob, err := NewContextRelevanceSelectionJob(ContextRelevanceSelectionInput{
		Authority: emptyConceptInput, AcceptedCandidateIDs: []string{},
	})
	if err != nil {
		t.Fatalf("explicit empty retrieval concepts were rejected: %v", err)
	}
	emptyPrompt, err := RenderPortableJob(emptyConceptJob)
	if err != nil || !strings.Contains(emptyPrompt, `"retrieval_concepts":[]`) ||
		strings.Contains(emptyPrompt, `"retrieval_concepts":null`) {
		t.Fatalf("empty concepts prompt=%s error=%v", emptyPrompt, err)
	}
	tests := map[string]func(*ContextRelevanceInput){
		"namespace": func(input *ContextRelevanceInput) { input.CandidateAuthorities[0].Namespace = "Conversation History" },
		"ID":        func(input *ContextRelevanceInput) { input.CandidateAuthorities[0].CandidateID = "memory:1" },
		"hash": func(input *ContextRelevanceInput) {
			input.CandidateAuthorities[0].ContentSHA256 = strings.Repeat("0", 64)
		},
		"duplicate ID": func(input *ContextRelevanceInput) { input.CandidateAuthorities[1].CandidateID = "CTX_1" },
		"duplicate content": func(input *ContextRelevanceInput) {
			input.CandidateAuthorities[1].Content = input.CandidateAuthorities[0].Content
			input.CandidateAuthorities[1].ContentSHA256 = input.CandidateAuthorities[0].ContentSHA256
		},
		"oversized content": func(input *ContextRelevanceInput) {
			input.CandidateAuthorities[0].Content = strings.Repeat("x", MaxContextCandidateContentBytes+1)
			input.CandidateAuthorities[0].ContentSHA256 = ExactObjectiveContextSHA(input.CandidateAuthorities[0].Content)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := base
			input.CandidateAuthorities = append([]ContextCandidateAuthority(nil), base.CandidateAuthorities...)
			mutate(&input)
			if err := newJob(input); err == nil {
				t.Fatal("invalid candidate authority was accepted")
			}
		})
	}
	for name, maximum := range map[string]int{
		"zero": 0, "over candidates": 3, "over fixed bound": MaxContextRelevanceSelections + 1,
	} {
		t.Run("max selections "+name, func(t *testing.T) {
			input := base
			input.MaxSelections = maximum
			if err := newJob(input); err == nil {
				t.Fatal("invalid max selections was accepted")
			}
		})
	}
}

func TestContextRelevanceSelectionEnforcesPerCallCandidateBudget(t *testing.T) {
	t.Parallel()
	input := ContextRelevanceInput{
		ExactInstruction: "Repeat the prior procedure.", RetrievalConcepts: []string{"prior procedure"},
		MaxSelections: 3,
	}
	for index := 1; index <= 3; index++ {
		input.CandidateAuthorities = append(input.CandidateAuthorities, contextCandidateFixture(
			t, "conversation", fmt.Sprintf("CTX_%d", index),
			strings.Repeat(fmt.Sprintf("%d", index), 2_000),
		))
	}
	decision := ContextRelevanceDecision{
		Schema: ContextRelevanceSchemaV1, ReferencedCandidateIDs: []string{"CTX_1", "CTX_2", "CTX_3"},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatalf("valid selection was limited incorrectly: %v", err)
	}
	input.MaxSelections = 4
	input.CandidateAuthorities = append(input.CandidateAuthorities, contextCandidateFixture(
		t, "conversation", "CTX_4", strings.Repeat("4", 2_000),
	))
	if _, err := NewContextRelevanceSelectionJob(ContextRelevanceSelectionInput{
		Authority: input, AcceptedCandidateIDs: []string{},
	}); err == nil {
		t.Fatal("candidate projection beyond its byte budget was accepted")
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
