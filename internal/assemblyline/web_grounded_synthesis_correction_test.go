package assemblyline

import (
	"strings"
	"testing"
)

func webGroundedSynthesisCorrectionFixture() WebGroundedSynthesisCorrectionInput {
	return WebGroundedSynthesisCorrectionInput{
		ExactQuestion: "Which release is current?",
		Paragraphs: []WebReviewParagraph{
			{ParagraphID: "P1", Text: "Version 3 is current.", EvidenceIDs: []string{"E31"}},
			{ParagraphID: "P2", Text: "Version 2 remains supported.", EvidenceIDs: []string{"E32"}},
		},
		Issue: WebClaimEvidenceReviewDecision{
			Schema: WebClaimEvidenceReviewSchemaV1, Outcome: WebClaimEvidenceReviewIssue,
			ParagraphID: "P1", EvidenceIDs: []string{"E31"},
			IssueKind: WebClaimEvidenceContradictedSupport,
			Detail:    "The cited evidence says version 2, not version 3.",
		},
		Evidence: []WebGroundedEvidence{
			{EvidenceID: "E31", Title: "Current", Content: "Version 2 is current."},
			{EvidenceID: "E32", Title: "Support", Content: "Version 2 remains supported."},
		},
		MaxParagraphBytes: 500,
	}
}

func TestWebGroundedSynthesisCorrectionReturnsOneExactBoundParagraph(t *testing.T) {
	input := webGroundedSynthesisCorrectionFixture()
	job, err := NewWebGroundedSynthesisCorrectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkWebGroundedSynthesisCorrection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{`"paragraph_id":"P1"`, `"paragraph_id":"P2"`, `"detail":"The cited evidence says version 2, not version 3."`} {
		if !strings.Contains(prompt, exact) {
			t.Fatalf("prompt lost retained correction input %s: %q", exact, prompt)
		}
	}
	for _, contract := range []string{
		"return its exact text unchanged",
		"exact zero delta is a valid semantic result",
		"Never write an opaque evidence ID",
	} {
		if !strings.Contains(prompt, contract) {
			t.Fatalf("prompt lost zero-delta correction contract %q: %q", contract, prompt)
		}
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("correction schema is open: %#v", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 1 || properties["text"] == nil {
		t.Fatalf("correction output is not one text leaf: %#v", schema)
	}
	textSchema := properties["text"].(map[string]any)
	if _, providerHostileBound := textSchema["maxLength"]; providerHostileBound {
		t.Fatalf("web synthesis correction schema contains a provider-hostile grammar repetition: %#v", textSchema)
	}
	raw := `{"text":"Version 2 is current."}`
	decision, err := DecodeWebGroundedSynthesisCorrectionDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Text != "Version 2 is current." {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebGroundedSynthesisCorrectionAcceptsExactZeroDelta(t *testing.T) {
	input := webGroundedSynthesisCorrectionFixture()
	decision, err := DecodeWebGroundedSynthesisCorrectionDecision(input, `{"text":"Version 3 is current."}`)
	if err != nil || decision.Text != input.Paragraphs[0].Text {
		t.Fatalf("zero-delta decision=%+v error=%v", decision, err)
	}
}

func TestWebGroundedSynthesisCorrectionProjectsZeroDeltaContractForUnrelatedFixtures(t *testing.T) {
	fixtures := []struct {
		question string
		text     string
		evidence string
	}{
		{question: "When does the museum open?", text: "The museum opens at nine.", evidence: "The doors open at 9:00."},
		{question: "What temperature melts the alloy?", text: "The alloy melts at 600 degrees.", evidence: "Melting point: 600 degrees."},
	}
	for index, fixture := range fixtures {
		input := WebGroundedSynthesisCorrectionInput{
			ExactQuestion: fixture.question,
			Paragraphs: []WebReviewParagraph{{
				ParagraphID: "P1", Text: fixture.text, EvidenceIDs: []string{"E1"},
			}},
			Issue: WebClaimEvidenceReviewDecision{
				Schema: WebClaimEvidenceReviewSchemaV1, Outcome: WebClaimEvidenceReviewIssue,
				ParagraphID: "P1", EvidenceIDs: []string{"E1"},
				IssueKind: WebClaimEvidenceInsufficientSupport, Detail: "The claim lacks support.",
			},
			Evidence:          []WebGroundedEvidence{{EvidenceID: "E1", Content: fixture.evidence}},
			MaxParagraphBytes: 500,
		}
		prompt, err := BuildWebGroundedSynthesisCorrectionPrompt(input)
		if err != nil {
			t.Fatalf("fixture %d prompt: %v", index, err)
		}
		if !strings.Contains(prompt, "exact zero delta is a valid semantic result") {
			t.Fatalf("fixture %d prompt lost zero-delta authority: %q", index, prompt)
		}
		decision, err := DecodeWebGroundedSynthesisCorrectionDecision(input, `{"text":"`+fixture.text+`"}`)
		if err != nil || decision.Text != fixture.text {
			t.Fatalf("fixture %d zero delta=%+v error=%v", index, decision, err)
		}
	}
}

func TestWebGroundedSynthesisCorrectionRejectsUnboundResults(t *testing.T) {
	input := webGroundedSynthesisCorrectionFixture()
	tests := map[string]string{
		"citation syntax": `{"text":"Version 2 is current. [1]"}`,
		"embedded ID":     `{"text":"Version 2 is current according to E31."}`,
		"null text":       `{"text":null}`,
		"paragraph ID":    `{"text":"Version 2 is current.","paragraph_id":"P1"}`,
		"evidence IDs":    `{"text":"Version 2 is current.","evidence_ids":["E31"]}`,
		"schema field":    `{"schema":"omnidex.web-grounded-synthesis-correction.v1","text":"Version 2 is current."}`,
		"extra authority": `{"text":"Version 2 is current.","complete":true}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWebGroundedSynthesisCorrectionDecision(input, raw); err == nil {
				t.Fatal("invalid synthesis correction was accepted")
			}
		})
	}
}

func TestWebGroundedSynthesisCorrectionRejectsInvalidExactIssue(t *testing.T) {
	input := webGroundedSynthesisCorrectionFixture()
	input.Issue.ParagraphID = "P9"
	if _, err := NewWebGroundedSynthesisCorrectionJob(input); err == nil {
		t.Fatal("correction issue outside retained paragraphs was accepted")
	}
	input = webGroundedSynthesisCorrectionFixture()
	input.ExactQuestion = strings.Repeat("x", maxWebQuestionBytes+1)
	if _, err := NewWebGroundedSynthesisCorrectionJob(input); err == nil {
		t.Fatal("oversized correction input was accepted")
	}
}
