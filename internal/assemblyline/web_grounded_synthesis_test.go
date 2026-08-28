package assemblyline

import (
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
		MaxParagraphs: 2, MaxParagraphBytes: 500,
	}
}

func TestWebSynthesisLeavesSeparateCoverageParagraphAndEvidenceRelation(t *testing.T) {
	base := webSynthesisFixture()
	leafInput := WebSynthesisParagraphLeafInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		Evidence: base.Evidence, AcceptedParagraphs: []WebGroundedParagraph{},
		MaxParagraphs: base.MaxParagraphs, MaxParagraphBytes: base.MaxParagraphBytes,
	}
	coverageJob, err := NewWebSynthesisParagraphCoverageJob(leafInput)
	if err != nil {
		t.Fatal(err)
	}
	if coverageJob.Kind != WorkWebSynthesisParagraphCoverage {
		t.Fatalf("kind=%q", coverageJob.Kind)
	}
	coverage, err := DecodeWebSynthesisParagraphCoverageDecision(
		leafInput, string(WebSynthesisParagraphRemains),
	)
	if err != nil || coverage.Coverage != WebSynthesisParagraphRemains {
		t.Fatalf("coverage=%+v err=%v", coverage, err)
	}
	paragraph, err := DecodeWebSynthesisParagraphDecision(
		leafInput, "Version 2 is current.",
	)
	if err != nil {
		t.Fatal(err)
	}
	relationInput := WebSynthesisEvidenceRelationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: paragraph.Text, Evidence: base.Evidence[1],
	}
	relationJob, err := NewWebSynthesisEvidenceRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkWebSynthesisEvidenceRelation {
		t.Fatalf("kind=%q", relationJob.Kind)
	}
	prompt, err := BuildWebSynthesisEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, relationInput.Evidence.EvidenceID) {
		t.Fatalf("pairwise evidence relation exposed code-owned ID: %q", prompt)
	}
	relation, err := DecodeWebSynthesisEvidenceRelationDecision(
		relationInput, string(WebEvidenceSupportsParagraph),
	)
	if err != nil || relation.Relation != WebEvidenceSupportsParagraph {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}
}

func TestWebSynthesisLeafDecodersRejectStructuredAndCompositeResults(t *testing.T) {
	base := webSynthesisFixture()
	input := WebSynthesisParagraphLeafInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		Evidence: base.Evidence, AcceptedParagraphs: []WebGroundedParagraph{},
		MaxParagraphs: base.MaxParagraphs, MaxParagraphBytes: base.MaxParagraphBytes,
	}
	for _, raw := range []string{`{"text":"Version 2 is current."}`, `"Version 2 is current."`, "Version 2 [1]", "See https://example.test"} {
		if _, err := DecodeWebSynthesisParagraphDecision(input, raw); err == nil {
			t.Fatalf("invalid paragraph leaf accepted: %q", raw)
		}
	}
	for _, raw := range []string{"unknown", "PARAGRAPH_REMAINS\nreason", `{"coverage":"NO_UNCOVERED_PARAGRAPH"}`} {
		if _, err := DecodeWebSynthesisParagraphCoverageDecision(input, raw); err == nil {
			t.Fatalf("invalid coverage leaf accepted: %q", raw)
		}
	}
}

func TestWebSynthesisCodeAssemblyEnforcesBoundEvidence(t *testing.T) {
	input := webSynthesisFixture()
	decision := WebGroundedSynthesisDecision{
		Schema: WebGroundedSynthesisSchemaV1,
		Paragraphs: []WebGroundedParagraph{{
			Text: "Version 2 is current.", EvidenceIDs: []string{"E31"},
		}},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	decision.Paragraphs[0].EvidenceIDs = []string{"E99"}
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("assembled synthesis accepted unprojected evidence")
	}
}
