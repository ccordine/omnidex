package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func webClaimEvidenceReviewFixture() WebClaimEvidenceReviewInput {
	return WebClaimEvidenceReviewInput{
		ExactQuestion: "Which release is current?",
		Paragraph: WebReviewParagraph{
			ParagraphID: "P1", Text: "Version 2 is current.", EvidenceIDs: []string{"E31"},
		},
		Evidence: []WebReviewEvidence{{
			EvidenceID: "E31", Title: "Release", Snippet: "Current", Content: "Version 2 is current.",
		}},
	}
}

func TestWebClaimEvidenceReviewReturnsOnlyNoneOrOneBoundIssue(t *testing.T) {
	input := webClaimEvidenceReviewFixture()
	job, err := NewWebClaimEvidenceReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"paragraph_id":"P1"`) || schema["additionalProperties"] != false {
		t.Fatalf("prompt/schema lost closed review authority: %q %#v", prompt, schema)
	}
	none := fmt.Sprintf(
		`{"schema":%q,"outcome":"none","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":""}`,
		WebClaimEvidenceReviewSchemaV1,
	)
	decision, err := DecodeWebClaimEvidenceReviewDecision(input, none)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != WebClaimEvidenceReviewNone || decision.EvidenceIDs == nil {
		t.Fatalf("decision=%+v", decision)
	}
	issue := fmt.Sprintf(
		`{"schema":%q,"outcome":"issue","paragraph_id":"P1","evidence_ids":["E31"],"issue_kind":"insufficient_support","detail":"The evidence does not establish that this is the current release."}`,
		WebClaimEvidenceReviewSchemaV1,
	)
	decision, err = DecodeWebClaimEvidenceReviewDecision(input, issue)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != WebClaimEvidenceReviewIssue || decision.ParagraphID != "P1" || len(decision.EvidenceIDs) != 1 {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebClaimEvidenceReviewRejectsAmbiguousOrUnboundResults(t *testing.T) {
	input := webClaimEvidenceReviewFixture()
	schema := fmt.Sprintf(`"schema":%q`, WebClaimEvidenceReviewSchemaV1)
	tests := map[string]string{
		"none with issue":        `{` + schema + `,"outcome":"none","paragraph_id":"P1","evidence_ids":["E31"],"issue_kind":"insufficient_support","detail":"Issue."}`,
		"issue without detail":   `{` + schema + `,"outcome":"issue","paragraph_id":"P1","evidence_ids":["E31"],"issue_kind":"insufficient_support","detail":""}`,
		"wrong paragraph":        `{` + schema + `,"outcome":"issue","paragraph_id":"P9","evidence_ids":["E31"],"issue_kind":"contradicted_support","detail":"Contradicted."}`,
		"unknown evidence":       `{` + schema + `,"outcome":"issue","paragraph_id":"P1","evidence_ids":["E99"],"issue_kind":"contradicted_support","detail":"Contradicted."}`,
		"duplicate evidence":     `{` + schema + `,"outcome":"issue","paragraph_id":"P1","evidence_ids":["E31","E31"],"issue_kind":"contradicted_support","detail":"Contradicted."}`,
		"unknown issue kind":     `{` + schema + `,"outcome":"issue","paragraph_id":"P1","evidence_ids":["E31"],"issue_kind":"rewrite","detail":"Rewrite it."}`,
		"unknown authority":      `{` + schema + `,"outcome":"none","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":"","complete":true}`,
		"null evidence IDs":      `{` + schema + `,"outcome":"none","paragraph_id":"","evidence_ids":null,"issue_kind":"","detail":""}`,
		"duplicate root field":   `{` + schema + `,"outcome":"none","outcome":"issue","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":""}`,
		"trailing JSON":          `{` + schema + `,"outcome":"none","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":""} {}`,
		"markdown fence":         "```json\n{" + schema + `,"outcome":"none","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":""}` + "\n```",
		"oversized raw response": strings.Repeat("x", maxPortableCandidateBytes+1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWebClaimEvidenceReviewDecision(input, raw); err == nil {
				t.Fatal("invalid claim-evidence review was accepted")
			}
		})
	}
}

func TestWebClaimEvidenceReviewRejectsUnboundOrOversizedInput(t *testing.T) {
	input := webClaimEvidenceReviewFixture()
	input.Paragraph.EvidenceIDs = []string{"E99"}
	if _, err := NewWebClaimEvidenceReviewJob(input); err == nil {
		t.Fatal("paragraph citation outside review evidence was accepted")
	}
	input = webClaimEvidenceReviewFixture()
	input.Evidence[0].Content = strings.Repeat("x", maxWebReviewEvidenceProjectionBytes+1)
	if _, err := NewWebClaimEvidenceReviewJob(input); err == nil {
		t.Fatal("oversized review evidence was accepted")
	}
}
