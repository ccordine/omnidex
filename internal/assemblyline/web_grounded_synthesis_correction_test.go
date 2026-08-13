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
	if schema["additionalProperties"] != false {
		t.Fatalf("correction schema is open: %#v", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 1 || properties["text"] == nil {
		t.Fatalf("correction output is not one text leaf: %#v", schema)
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

func TestWebGroundedSynthesisCorrectionRejectsUnboundOrNoopResults(t *testing.T) {
	input := webGroundedSynthesisCorrectionFixture()
	tests := map[string]string{
		"no-op":           `{"text":"Version 3 is current."}`,
		"citation syntax": `{"text":"Version 2 is current. [1]"}`,
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
