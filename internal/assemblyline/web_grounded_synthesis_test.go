package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func webSynthesisFixture() WebGroundedSynthesisInput {
	return WebGroundedSynthesisInput{
		ExactQuestion: "\nWhich release is current?  ",
		Evidence: []WebGroundedEvidence{
			{EvidenceID: "E17", Title: "History", Snippet: "Old", Content: "Version 1 was superseded."},
			{EvidenceID: "E31", Title: "Release", Snippet: "Current", Content: "Version 2 is current."},
		},
		MaxParagraphs:     2,
		MaxParagraphBytes: 500,
	}
}

func TestWebGroundedSynthesisPortableContractUsesOpaqueEvidenceIDs(t *testing.T) {
	input := webSynthesisFixture()
	job, err := NewWebGroundedSynthesisJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"exact_question":"\nWhich release is current?  "`) {
		t.Fatal("exact question whitespace was not preserved")
	}
	if schema["additionalProperties"] != false {
		t.Fatal("synthesis schema permits extra authority fields")
	}
	raw := fmt.Sprintf(
		`{"schema":%q,"paragraphs":[{"text":"Version 2 is current.","evidence_ids":["E31"]}]}`,
		WebGroundedSynthesisSchemaV1,
	)
	decision, err := DecodeWebGroundedSynthesisDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Paragraphs) != 1 || decision.Paragraphs[0].EvidenceIDs[0] != "E31" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebGroundedSynthesisRejectsAmbiguousOrUngroundedResults(t *testing.T) {
	input := webSynthesisFixture()
	schema := fmt.Sprintf(`"schema":%q`, WebGroundedSynthesisSchemaV1)
	validParagraph := `{"text":"Version 2 is current.","evidence_ids":["E31"]}`
	tests := map[string]string{
		"duplicate root":      `{` + schema + `,"paragraphs":[` + validParagraph + `],"paragraphs":[` + validParagraph + `]}`,
		"nested duplicate":    `{` + schema + `,"paragraphs":[{"text":"first","text":"second","evidence_ids":["E31"]}]}`,
		"case alias":          `{` + schema + `,"Paragraphs":[` + validParagraph + `]}`,
		"unknown field":       `{` + schema + `,"paragraphs":[{"text":"answer","evidence_ids":["E31"],"plan":"next"}]}`,
		"trailing value":      `{` + schema + `,"paragraphs":[` + validParagraph + `]} {}`,
		"markdown fence":      "```json\n{" + schema + `,"paragraphs":[` + validParagraph + `]}` + "\n```",
		"out of set":          `{` + schema + `,"paragraphs":[{"text":"answer","evidence_ids":["E99"]}]}`,
		"duplicate ID":        `{` + schema + `,"paragraphs":[{"text":"answer","evidence_ids":["E31","E31"]}]}`,
		"model URL":           `{` + schema + `,"paragraphs":[{"text":"See https://example.com","evidence_ids":["E31"]}]}`,
		"model citation":      `{` + schema + `,"paragraphs":[{"text":"Version 2 [1]","evidence_ids":["E31"]}]}`,
		"oversized paragraph": `{` + schema + `,"paragraphs":[{"text":"` + strings.Repeat("x", input.MaxParagraphBytes+1) + `","evidence_ids":["E31"]}]}`,
		"oversized raw":       strings.Repeat("x", maxPortableCandidateBytes+1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWebGroundedSynthesisDecision(input, raw); err == nil {
				t.Fatal("invalid grounded synthesis was accepted")
			}
		})
	}
}

func TestWebGroundedSynthesisRejectsInputBeforeOversizedProjectionCopy(t *testing.T) {
	input := webSynthesisFixture()
	input.Evidence[0].Content = strings.Repeat("x", maxWebEvidenceProjectionBytes+1)
	if _, err := NewWebGroundedSynthesisJob(input); err == nil {
		t.Fatal("oversized evidence projection was accepted")
	}
	input = webSynthesisFixture()
	input.Evidence[1].EvidenceID = "E17"
	if _, err := NewWebGroundedSynthesisJob(input); err == nil {
		t.Fatal("duplicate evidence identities were accepted")
	}
}

func TestWebGroundedSynthesisBoundsEvidenceIDsPerParagraph(t *testing.T) {
	input := webSynthesisFixture()
	input.Evidence = append(input.Evidence,
		WebGroundedEvidence{EvidenceID: "E32", Content: "third"},
		WebGroundedEvidence{EvidenceID: "E33", Content: "fourth"},
		WebGroundedEvidence{EvidenceID: "E34", Content: "fifth"},
	)
	raw := fmt.Sprintf(
		`{"schema":%q,"paragraphs":[{"text":"Claim.","evidence_ids":["E17","E31","E32","E33","E34"]}]}`,
		WebGroundedSynthesisSchemaV1,
	)
	if _, err := DecodeWebGroundedSynthesisDecision(input, raw); err == nil {
		t.Fatal("unbounded per-paragraph evidence IDs were accepted")
	}
	schema, err := WebGroundedSynthesisResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	paragraphs := properties["paragraphs"].(map[string]any)
	paragraph := paragraphs["items"].(map[string]any)
	paragraphProperties := paragraph["properties"].(map[string]any)
	evidenceIDs := paragraphProperties["evidence_ids"].(map[string]any)
	if evidenceIDs["maxItems"] != maxWebEvidenceIDsPerParagraph {
		t.Fatalf("maxItems=%v", evidenceIDs["maxItems"])
	}
}
