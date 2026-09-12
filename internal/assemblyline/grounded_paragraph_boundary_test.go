package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompoundGroundedParagraphAuthorizationKindsAreUnavailable(t *testing.T) {
	for _, retired := range []WorkKind{
		"grounded_answer_paragraph_authorization",
		"roleplay_grounded_response_paragraph_authorization",
	} {
		job, err := newPortableJob(retired, struct{}{})
		if err != nil {
			t.Fatal(err)
		}
		if err := job.Validate(); err == nil {
			t.Errorf("compound paragraph authorization %q remains callable", retired)
		}
	}
}

func TestGroundedParagraphRelationsHaveSeparateInputBoundaries(t *testing.T) {
	for _, fixture := range []struct{ question, paragraph, evidence string }{
		{"When is the inspection?", "The inspection occurs Monday.", "The schedule records Monday for the inspection."},
		{"When does the eclipse begin?", "The eclipse begins at midnight.", "The almanac records a midnight eclipse start."},
	} {
		context := plainTextPromptObjectiveContext("The question concerns the next scheduled occurrence.")
		relevanceInput := GroundedParagraphRelevanceInput{
			ExactQuestion: fixture.question, Context: context, ParagraphText: fixture.paragraph,
		}
		relevanceJob, err := NewGroundedParagraphRelevanceJob(relevanceInput)
		if err != nil {
			t.Fatal(err)
		}
		relevancePrompt, err := RenderPortableJob(relevanceJob)
		if err != nil {
			t.Fatal(err)
		}
		assertPromptContains(t, relevancePrompt, fixture.question, fixture.paragraph, context.Capsules[0].Content)
		assertPromptOmitsPacketState(t, relevancePrompt, fixture.evidence, "factual claim", "voice", "evidence:")
		for _, domain := range []GroundedClaimDomain{GroundedAllFactualClaims, GroundedRealWorldClaims} {
			supportInput := GroundedParagraphSupportInput{
				ParagraphText: fixture.paragraph, ClaimDomain: domain,
				Evidence: []GroundedEvidenceCapsule{{ID: "evidence-private", Text: fixture.evidence}},
			}
			supportJob, err := NewGroundedParagraphSupportJob(supportInput)
			if err != nil {
				t.Fatal(err)
			}
			supportPrompt, err := RenderPortableJob(supportJob)
			if err != nil {
				t.Fatal(err)
			}
			assertPromptContains(t, supportPrompt, fixture.paragraph, fixture.evidence)
			assertPromptOmitsPacketState(t, supportPrompt, fixture.question, context.Capsules[0].Content, "evidence-private", "voice", "character", "responsive")
			for raw, want := range map[string]GroundedParagraphSupport{"A": GroundedParagraphFullySupported, "B": GroundedParagraphNotFullySupported} {
				got, err := DecodeGroundedParagraphSupport(supportInput, raw)
				if err != nil || got != want {
					t.Fatalf("support choice %q = %q, %v", raw, got, err)
				}
			}
			for _, raw := range []string{`{"supported":true}`, "FULLY_SUPPORTED", "A\nB", "approve"} {
				if _, err := DecodeGroundedParagraphSupport(supportInput, raw); err == nil {
					t.Errorf("support accepted non-leaf output %q", raw)
				}
			}
			assertGroundedParagraphRejectsExtraFields(t, supportJob, "exact_question", "objective_context", "roleplay_identity")
			maximum, err := PortableResponseMaximumBytesForJob(supportJob)
			if err != nil || maximum != 1 {
				t.Fatalf("support response maximum=%d err=%v", maximum, err)
			}
		}
		for raw, want := range map[string]GroundedParagraphRelevance{"A": GroundedParagraphRelevant, "B": GroundedParagraphNotRelevant} {
			got, err := DecodeGroundedParagraphRelevance(relevanceInput, raw)
			if err != nil || got != want {
				t.Fatalf("relevance choice %q = %q, %v", raw, got, err)
			}
		}
		for _, raw := range []string{`{"relevant":true}`, "RELEVANT", "A\nB", "accept"} {
			if _, err := DecodeGroundedParagraphRelevance(relevanceInput, raw); err == nil {
				t.Errorf("relevance accepted non-leaf output %q", raw)
			}
		}
		assertGroundedParagraphRejectsExtraFields(t, relevanceJob, "evidence", "claim_domain", "roleplay_identity")
		maximum, err := PortableResponseMaximumBytesForJob(relevanceJob)
		if err != nil || maximum != 1 {
			t.Fatalf("relevance response maximum=%d err=%v", maximum, err)
		}
	}
}

func assertGroundedParagraphRejectsExtraFields(t *testing.T, job PortableJob, fields ...string) {
	t.Helper()
	for _, field := range fields {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		payload[field] = json.RawMessage(`"unrelated input"`)
		changed := job
		var err error
		changed.Payload, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RenderPortableJob(changed); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("%s accepted unrelated input field %q: %v", job.Kind, field, err)
		}
	}
}

func TestGroundedParagraphRelationsRejectInvalidLocalAuthority(t *testing.T) {
	support := GroundedParagraphSupportInput{
		ParagraphText: "The inspection occurs Monday.", ClaimDomain: "guessed-scope",
		Evidence: []GroundedEvidenceCapsule{{ID: "evidence-private", Text: "The inspection is scheduled for Monday."}},
	}
	if _, err := BuildGroundedParagraphSupportPrompt(support); err == nil {
		t.Fatal("unsupported factual scope was rendered")
	}
	if _, err := DecodeGroundedParagraphSupport(support, "A"); err == nil {
		t.Fatal("unsupported factual scope accepted a result")
	}
	relevance := GroundedParagraphRelevanceInput{ParagraphText: support.ParagraphText}
	if _, err := BuildGroundedParagraphRelevancePrompt(relevance); err == nil {
		t.Fatal("relevance rendered without a question")
	}
	if _, err := DecodeGroundedParagraphRelevance(relevance, "A"); err == nil {
		t.Fatal("relevance accepted a result without a question")
	}
}
