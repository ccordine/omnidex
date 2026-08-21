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
	if !strings.Contains(prompt, `"paragraph_id":"P1"`) || schema["type"] != "object" {
		t.Fatalf("prompt/schema lost closed review authority: %q %#v", prompt, schema)
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("review schema lacks exact NONE/ISSUE variants: %#v", schema)
	}
	noneVariant := variants[0].(map[string]any)
	noneProperties := noneVariant["properties"].(map[string]any)
	if noneProperties["outcome"].(map[string]any)["const"] != "none" ||
		len(noneProperties) != 2 || noneVariant["additionalProperties"] != false {
		t.Fatalf("NONE schema is not structurally empty: %#v", noneVariant)
	}
	none := fmt.Sprintf(
		`{"schema":%q,"outcome":"none"}`,
		WebClaimEvidenceReviewSchemaV1,
	)
	decision, err := DecodeWebClaimEvidenceReviewDecision(input, none)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != WebClaimEvidenceReviewNone || decision.EvidenceIDs != nil {
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

func TestWebClaimEvidenceReviewTreatsCitedEvidenceAsCollectiveSupport(t *testing.T) {
	fixtures := []WebClaimEvidenceReviewInput{
		{
			ExactQuestion: "How are municipal elections certified?",
			Paragraph: WebReviewParagraph{
				ParagraphID: "P7",
				Text:        "The clerk canvasses returns before the council certifies the result.",
				EvidenceIDs: []string{"E-municipal-process", "E-council-minutes"},
			},
			Evidence: []WebReviewEvidence{
				{EvidenceID: "E-municipal-process", Title: "Election procedure", Content: "The clerk canvasses returns."},
				{EvidenceID: "E-council-minutes", Title: "Council record", Content: "The council certifies the result after canvassing."},
			},
		},
		{
			ExactQuestion: "What conditions help this plant germinate?",
			Paragraph: WebReviewParagraph{
				ParagraphID: "P4",
				Text:        "The seed germinates in moist soil after a period of cold stratification.",
				EvidenceIDs: []string{"E-moisture-study", "E-stratification-guide"},
			},
			Evidence: []WebReviewEvidence{
				{EvidenceID: "E-moisture-study", Title: "Moisture trial", Content: "Germination requires moist soil."},
				{EvidenceID: "E-stratification-guide", Title: "Propagation guide", Content: "Cold stratification precedes germination."},
			},
		},
	}
	for _, input := range fixtures {
		prompt, err := BuildWebClaimEvidenceReviewPrompt(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"Evaluate the cited evidence collectively.",
			"every capsule does not need to repeat every claim",
			"Report insufficient_support only when no cited capsule supports the material claim.",
			"For NONE return only schema and outcome",
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("collective-evidence contract %q missing from prompt: %q", required, prompt)
			}
		}
		for _, evidence := range input.Evidence {
			if !strings.Contains(prompt, evidence.EvidenceID) || !strings.Contains(prompt, evidence.Content) {
				t.Fatalf("prompt lost evidence capsule %q: %q", evidence.EvidenceID, prompt)
			}
		}
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
		"unknown authority":      `{` + schema + `,"outcome":"none","complete":true}`,
		"null evidence IDs":      `{` + schema + `,"outcome":"none","evidence_ids":null}`,
		"duplicate root field":   `{` + schema + `,"outcome":"none","outcome":"issue"}`,
		"trailing JSON":          `{` + schema + `,"outcome":"none"} {}`,
		"markdown fence":         "```json\n{" + schema + `,"outcome":"none"}` + "\n```",
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
