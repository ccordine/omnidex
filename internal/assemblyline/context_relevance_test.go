package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestContextRelevanceGreetingSelectsNoArchiveAuthority(t *testing.T) {
	t.Parallel()
	input := ContextRelevanceInput{
		ExactInstruction:  "Hello.",
		RetrievalConcepts: []string{"prior telescope tuning"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_1", "  Yesterday we tuned the telescope.\n"),
			contextCandidateFixture(t, "roleplay.canon", "CTX_2", "The northern gate opens at dawn."),
		},
		MaxSelections: 2,
	}
	job, err := NewContextRelevanceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkContextRelevance {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"candidate_id":"CTX_2"`) ||
		!strings.Contains(prompt, `"retrieval_concepts":["prior telescope tuning"]`) ||
		!strings.Contains(prompt, `"content":"  Yesterday we tuned the telescope.\n"`) ||
		strings.Index(prompt, "telescope") < 0 || schema["additionalProperties"] != false {
		t.Fatalf("relevance projection/schema=%s %#v", prompt, schema)
	}
	for _, hidden := range []string{
		input.CandidateAuthorities[0].Namespace,
		input.CandidateAuthorities[0].ContentSHA256,
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("relevance prompt leaked code-owned candidate provenance %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("portable relevance payload lost candidate provenance %q: %s", hidden, job.Payload)
		}
	}
	for _, forbidden := range []string{`"namespace"`, `"content_sha256"`, `"provider"`, `"source"`} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relevance prompt exposed forbidden code authority field %q: %s", forbidden, prompt)
		}
	}
	decision, err := DecodeContextRelevanceDecision(
		input,
		fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":[]}`, ContextRelevanceSchemaV1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.ReferencedCandidateIDs == nil || len(decision.ReferencedCandidateIDs) != 0 {
		t.Fatalf("greeting selection=%#v", decision.ReferencedCandidateIDs)
	}
}

func TestContextRelevanceAnaphoraReturnsOnlyProjectedOpaqueID(t *testing.T) {
	t.Parallel()
	input := ContextRelevanceInput{
		ExactInstruction:  "Do it again.",
		RetrievalConcepts: []string{"previous action"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_7", "USER: Water the greenhouse beds.\nASSISTANT: The greenhouse beds were watered."),
			contextCandidateFixture(t, "memory", "CTX_3", "Use rye flour for the bakery's morning loaves."),
		},
		MaxSelections: 1,
	}
	decision, err := DecodeContextRelevanceDecision(
		input,
		fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":["CTX_7"]}`, ContextRelevanceSchemaV1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.ReferencedCandidateIDs) != 1 || decision.ReferencedCandidateIDs[0] != "CTX_7" {
		t.Fatalf("selection=%#v", decision)
	}
	for name, raw := range map[string]string{
		"unknown ID":    fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":["CTX_9"]}`, ContextRelevanceSchemaV1),
		"duplicate ID":  fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":["CTX_7","CTX_7"]}`, ContextRelevanceSchemaV1),
		"null IDs":      fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":null}`, ContextRelevanceSchemaV1),
		"control field": fmt.Sprintf(`{"schema":%q,"referenced_candidate_ids":[],"action":"continue"}`, ContextRelevanceSchemaV1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeContextRelevanceDecision(input, raw); err == nil {
				t.Fatal("invalid relevance decision was accepted")
			}
		})
	}
}

func TestContextRelevanceRejectsInvalidAuthorityIdentityHashAndDuplicates(t *testing.T) {
	t.Parallel()
	base := ContextRelevanceInput{
		ExactInstruction:  "Repeat the previous greenhouse operation.",
		RetrievalConcepts: []string{"previous greenhouse operation"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_1", "The vents were opened."),
			contextCandidateFixture(t, "memory", "CTX_2", "The irrigation cycle ran for ten minutes."),
		},
		MaxSelections: 2,
	}
	for name, concepts := range map[string][]string{
		"nil":        nil,
		"mixed case": {"Previous action"},
		"unsorted":   {"zulu", "alpha"},
		"duplicate":  {"previous action", "previous action"},
		"over bound": {"a", "b", "c", "d"},
	} {
		t.Run("retrieval concepts "+name, func(t *testing.T) {
			input := base
			input.RetrievalConcepts = concepts
			if _, err := NewContextRelevanceJob(input); err == nil {
				t.Fatal("noncanonical retrieval concepts were accepted")
			}
		})
	}
	emptyConceptInput := base
	emptyConceptInput.RetrievalConcepts = []string{}
	emptyConceptJob, err := NewContextRelevanceJob(emptyConceptInput)
	if err != nil {
		t.Fatalf("explicit empty retrieval concepts were rejected: %v", err)
	}
	emptyConceptPrompt, _, err := RenderPortableJob(emptyConceptJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emptyConceptPrompt, `"retrieval_concepts":[]`) ||
		strings.Contains(emptyConceptPrompt, `"retrieval_concepts":null`) {
		t.Fatalf("empty retrieval concepts lost explicit-array authority: %s", emptyConceptPrompt)
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
		"invalid UTF-8": func(input *ContextRelevanceInput) {
			input.CandidateAuthorities[0].Content = string([]byte{0xff})
			input.CandidateAuthorities[0].ContentSHA256 = ExactObjectiveContextSHA(input.CandidateAuthorities[0].Content)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := base
			input.CandidateAuthorities = append([]ContextCandidateAuthority(nil), base.CandidateAuthorities...)
			mutate(&input)
			if _, err := NewContextRelevanceJob(input); err == nil {
				t.Fatal("invalid candidate authority was accepted")
			}
		})
	}
	for name, maxSelections := range map[string]int{"zero": 0, "over candidates": 3, "over fixed bound": MaxContextRelevanceSelections + 1} {
		t.Run("max selections "+name, func(t *testing.T) {
			input := base
			input.MaxSelections = maxSelections
			if _, err := NewContextRelevanceJob(input); err == nil {
				t.Fatal("invalid max selections was accepted")
			}
		})
	}
}

func TestContextRelevanceEnforcesOnlyItsPerCallCandidateBudget(t *testing.T) {
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
		t.Fatalf("valid selection was incorrectly limited by a downstream per-call budget: %v", err)
	}

	input.MaxSelections = 4
	input.CandidateAuthorities = append(input.CandidateAuthorities, contextCandidateFixture(
		t, "conversation", "CTX_4", strings.Repeat("4", 2_000),
	))
	if _, err := NewContextRelevanceJob(input); err == nil {
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
