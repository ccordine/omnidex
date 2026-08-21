package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestContextMinificationReturnsOneBoundedLeafForAnaphora(t *testing.T) {
	t.Parallel()
	input := ContextMinificationInput{
		ExactInstruction: "Do it again.",
		SelectedAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_4", "USER: Set the starship course for Europa.\nASSISTANT: I set the course for Europa."),
			contextCandidateFixture(t, "mission.fact", "CTX_9", "The Europa route requires a gravity assist around Ganymede."),
		},
	}
	job, err := NewContextMinificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkContextMinification {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.ExactInstruction) || !strings.Contains(prompt, "Europa") ||
		schema["additionalProperties"] != false {
		t.Fatalf("minification projection/schema=%s %#v", prompt, schema)
	}
	properties := schema["properties"].(map[string]any)
	minimalContextSchema := properties["minimal_context"].(map[string]any)
	if _, providerHostileBound := minimalContextSchema["maxLength"]; providerHostileBound {
		t.Fatalf(
			"context minification schema contains a provider-hostile grammar repetition: %#v",
			minimalContextSchema,
		)
	}
	for _, hidden := range []string{
		input.SelectedAuthorities[0].Namespace,
		input.SelectedAuthorities[0].CandidateID,
		input.SelectedAuthorities[0].ContentSHA256,
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("minification prompt leaked code-owned candidate provenance %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("portable minification payload lost candidate provenance %q: %s", hidden, job.Payload)
		}
	}
	for _, forbidden := range []string{`"retrieval_concepts"`, `"search_terms"`} {
		if strings.Contains(prompt, forbidden) || strings.Contains(string(job.Payload), forbidden) {
			t.Fatalf("minification received relevance-only field %q", forbidden)
		}
	}
	raw := fmt.Sprintf(
		`{"schema":%q,"minimal_context":"The prior action set the starship course for Europa, using the required Ganymede gravity assist."}`,
		ContextMinificationSchemaV1,
	)
	decision, err := DecodeContextMinificationDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decision.MinimalContext, "Europa") || strings.Contains(decision.MinimalContext, "bakery") {
		t.Fatalf("minimal context=%q", decision.MinimalContext)
	}
}

func TestContextMinificationRejectsEmptyOversizedOrExpandedResponse(t *testing.T) {
	t.Parallel()
	input := ContextMinificationInput{
		ExactInstruction: "Prepare it the same way.",
		SelectedAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_2", "The pastry was folded twice and chilled for twenty minutes."),
		},
	}
	schema := fmt.Sprintf(`"schema":%q`, ContextMinificationSchemaV1)
	for name, raw := range map[string]string{
		"empty":         `{` + schema + `,"minimal_context":""}`,
		"untrimmed":     `{` + schema + `,"minimal_context":" folded twice "}`,
		"oversized":     `{` + schema + `,"minimal_context":"` + strings.Repeat("x", MaxContextMinifiedBytes+1) + `"}`,
		"invalid UTF-8": "{" + schema + `,"minimal_context":"` + string([]byte{0xff}) + `"}`,
		"extra answer":  `{` + schema + `,"minimal_context":"Folded twice.","answer":"Done."}`,
		"trailing":      `{` + schema + `,"minimal_context":"Folded twice."} []`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeContextMinificationDecision(input, raw); err == nil {
				t.Fatal("invalid minification leaf was accepted")
			}
		})
	}
}

func TestContextMinificationRequiresSelectedExactAuthoritiesWithinBudget(t *testing.T) {
	t.Parallel()
	input := ContextMinificationInput{ExactInstruction: "Do that again."}
	if _, err := NewContextMinificationJob(input); err == nil {
		t.Fatal("minification without selected authority was accepted")
	}
	input.SelectedAuthorities = []ContextCandidateAuthority{
		contextCandidateFixture(t, "conversation", "CTX_1", "The kiln was heated to 900 degrees."),
	}
	input.SelectedAuthorities[0].ContentSHA256 = strings.Repeat("f", 64)
	if _, err := NewContextMinificationJob(input); err == nil {
		t.Fatal("minification with changed exact-authority hash was accepted")
	}

	authorities := make([]ContextCandidateAuthority, MaxContextMinificationAuthorities+1)
	for index := range authorities {
		authorities[index] = contextCandidateFixture(
			t, "conversation", fmt.Sprintf("CTX_%d", index+1), fmt.Sprintf("Distinct kiln observation %d.", index+1),
		)
	}
	input.SelectedAuthorities = authorities
	if _, err := NewContextMinificationJob(input); err == nil {
		t.Fatal("minification authority count beyond the code-owned bound was accepted")
	}
}
