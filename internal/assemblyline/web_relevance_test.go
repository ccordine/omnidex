package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func webRelevanceFixture() WebRelevanceInput {
	return WebRelevanceInput{
		ExactQuestion: " Which release is current? ",
		Candidates: []WebRelevanceCandidate{
			{CandidateID: "C17", Title: "Old", Snippet: "Prior release", Excerpt: "Version 1 was superseded."},
			{CandidateID: "C31", Title: "Current", Snippet: "Stable release", Excerpt: "Version 2 is current."},
		},
		MaxSelections: 1,
	}
}

func TestWebRelevancePortableContractReturnsOnlyProjectedIDs(t *testing.T) {
	input := webRelevanceFixture()
	job, err := NewWebRelevanceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"exact_question":" Which release is current? "`) {
		t.Fatal("exact question whitespace was not preserved")
	}
	properties := schema["properties"].(map[string]any)
	ids := properties["candidate_ids"].(map[string]any)
	if ids["minItems"] != 0 || ids["maxItems"] != 1 {
		t.Fatalf("candidate ID bounds=%v", ids)
	}
	raw := fmt.Sprintf(`{"schema":%q,"candidate_ids":["C31"]}`, WebRelevanceSchemaV1)
	decision, err := DecodeWebRelevanceDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.CandidateIDs) != 1 || decision.CandidateIDs[0] != "C31" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebRelevanceResponseSchemaUsesOnlyBoundedIDsWithEmptyMeaningNone(t *testing.T) {
	input := webRelevanceFixture()
	schema, err := WebRelevanceResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	if len(properties) != 2 || properties["outcome"] != nil {
		t.Fatalf("web relevance schema exposes a redundant control outcome: %#v", schema)
	}
	selectedIDs := properties["candidate_ids"].(map[string]any)
	if selectedIDs["minItems"] != 0 || selectedIDs["maxItems"] != input.MaxSelections {
		t.Fatalf("candidate ID schema does not permit bounded explicit none: %#v", schema)
	}
}

func TestWebRelevancePortableContractReturnsExplicitEmptyIDsForNone(t *testing.T) {
	input := webRelevanceFixture()
	raw := fmt.Sprintf(`{"schema":%q,"candidate_ids":[]}`, WebRelevanceSchemaV1)
	decision, err := DecodeWebRelevanceDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if decision.CandidateIDs == nil || len(decision.CandidateIDs) != 0 {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestWebRelevanceRejectsAmbiguousAndOutOfSetIDs(t *testing.T) {
	input := webRelevanceFixture()
	validSchema := fmt.Sprintf(`"schema":%q`, WebRelevanceSchemaV1)
	tests := map[string]string{
		"duplicate field": `{` + validSchema + `,"candidate_ids":["C31"],"candidate_ids":["C17"]}`,
		"case alias":      `{` + validSchema + `,"Candidate_Ids":["C31"]}`,
		"unknown field":   `{` + validSchema + `,"candidate_ids":["C31"],"tool":"browser"}`,
		"trailing value":  `{` + validSchema + `,"candidate_ids":["C31"]} []`,
		"markdown fence":  "```\n{" + validSchema + `,"candidate_ids":["C31"]}` + "\n```",
		"out of set":      `{` + validSchema + `,"candidate_ids":["C99"]}`,
		"duplicate ID":    `{` + validSchema + `,"candidate_ids":["C31","C31"]}`,
		"over selection":  `{` + validSchema + `,"candidate_ids":["C17","C31"]}`,
		"null IDs":        `{` + validSchema + `,"candidate_ids":null}`,
		"control outcome": `{` + validSchema + `,"outcome":"none","candidate_ids":[]}`,
		"oversized raw":   strings.Repeat("x", maxPortableCandidateBytes+1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWebRelevanceDecision(input, raw); err == nil {
				t.Fatal("invalid relevance result was accepted")
			}
		})
	}
}

func TestWebRelevanceRejectsDuplicateAndOversizedCandidateProjection(t *testing.T) {
	input := webRelevanceFixture()
	input.Candidates[1].CandidateID = "C17"
	if _, err := NewWebRelevanceJob(input); err == nil {
		t.Fatal("duplicate candidate IDs were accepted")
	}
	input = webRelevanceFixture()
	input.Candidates[0].Excerpt = strings.Repeat("x", maxWebCandidateSummaryBytes+1)
	if _, err := NewWebRelevanceJob(input); err == nil {
		t.Fatal("oversized candidate field was accepted")
	}
}
