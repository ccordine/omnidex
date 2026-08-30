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

func TestWebSynthesisInventoryIsOneBoundedUntrustedCandidateCollection(t *testing.T) {
	base := webSynthesisFixture()
	job, err := NewWebSynthesisParagraphInventoryJob(base)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkWebSynthesisParagraphInventory {
		t.Fatalf("kind=%q", job.Kind)
	}
	inventory, err := DecodeWebSynthesisParagraphInventory(
		base,
		"Version 2 is current.\nVersion 2 superseded version 1.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 2 || inventory.Candidates[0] != "Version 2 is current." {
		t.Fatalf("inventory=%+v", inventory)
	}
	if err := inventory.ValidateFor(base); err != nil {
		t.Fatal(err)
	}

	absence, err := DecodeWebSynthesisParagraphInventory(base, WebNoSynthesisParagraphCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if absence.Candidates == nil || len(absence.Candidates) != 0 {
		t.Fatalf("absence inventory=%+v", absence)
	}

	// Exact duplicates remain untrusted inventory data. Queue code, rather than
	// another semantic review, deterministically prevents duplicate processing.
	duplicate, err := DecodeWebSynthesisParagraphInventory(
		base, "Version 2 is current.\nVersion 2 is current.",
	)
	if err != nil || len(duplicate.Candidates) != 2 {
		t.Fatalf("duplicate inventory=%+v err=%v", duplicate, err)
	}
}

func TestWebSynthesisInventoryAndRelationsProjectOnlySemanticAuthority(t *testing.T) {
	base := webSynthesisFixture()
	inventoryPrompt, err := BuildWebSynthesisParagraphInventoryPrompt(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"E17", "E31"} {
		if strings.Contains(inventoryPrompt, hidden) {
			t.Fatalf("inventory exposed code-owned evidence ID %q: %s", hidden, inventoryPrompt)
		}
	}
	for _, visible := range []string{`"max_paragraphs":2`, `"max_paragraph_bytes":500`} {
		if !strings.Contains(inventoryPrompt, visible) {
			t.Fatalf("inventory omitted exact bound %q: %s", visible, inventoryPrompt)
		}
	}

	relationInput := WebSynthesisEvidenceRelationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: "Version 2 is current.", Evidence: base.Evidence[1],
	}
	relationPrompt, err := BuildWebSynthesisEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(relationPrompt, relationInput.Evidence.EvidenceID) {
		t.Fatalf("pairwise evidence relation exposed code-owned ID: %q", relationPrompt)
	}
	relation, err := DecodeWebSynthesisEvidenceRelationDecision(
		relationInput, string(WebEvidenceSupportsParagraph),
	)
	if err != nil || relation.Relation != WebEvidenceSupportsParagraph {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}

	authorizationInput := WebSynthesisParagraphAuthorizationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: "Version 2 is current.", Evidence: []WebGroundedEvidence{base.Evidence[1]},
		MaxParagraphBytes: base.MaxParagraphBytes,
	}
	authorizationJob, err := NewWebSynthesisParagraphAuthorizationJob(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	if authorizationJob.Kind != WorkWebSynthesisParagraphAuthorization {
		t.Fatalf("authorization kind=%q", authorizationJob.Kind)
	}
	authorizationPrompt, err := BuildWebSynthesisParagraphAuthorizationPrompt(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(authorizationPrompt, base.Evidence[1].EvidenceID) {
		t.Fatalf("paragraph authorization exposed code-owned evidence ID: %q", authorizationPrompt)
	}
	for _, required := range []string{
		"complete exact paragraph", "exact question", "every factual claim", "sole factual authority",
	} {
		if !strings.Contains(authorizationPrompt, required) {
			t.Fatalf("paragraph authorization omitted %q: %s", required, authorizationPrompt)
		}
	}
	authorization, err := DecodeWebSynthesisParagraphAuthorizationDecision(
		authorizationInput, string(WebParagraphResponsiveAndFullySupported),
	)
	if err != nil || authorization.Relation != WebParagraphResponsiveAndFullySupported {
		t.Fatalf("authorization=%+v err=%v", authorization, err)
	}
}

func TestWebSynthesisInventoryAndAuthorizationRejectInvalidRawLeaves(t *testing.T) {
	base := webSynthesisFixture()
	for _, raw := range []string{
		`{"text":"Version 2 is current."}`,
		`"Version 2 is current."`,
		"Version 2 [1]",
		"See https://example.test",
		"Version 2 is current.\n\nVersion 1 was superseded.",
	} {
		if _, err := DecodeWebSynthesisParagraphInventory(base, raw); err == nil {
			t.Fatalf("invalid paragraph inventory accepted: %q", raw)
		}
	}
	authorizationInput := WebSynthesisParagraphAuthorizationInput{
		ExactQuestion:     base.ExactQuestion,
		ParagraphText:     "Version 2 is current.",
		Evidence:          []WebGroundedEvidence{base.Evidence[1]},
		MaxParagraphBytes: base.MaxParagraphBytes,
	}
	for _, raw := range []string{
		"unknown",
		string(WebParagraphResponsiveAndFullySupported) + "\nreason",
		`{"relation":"RESPONSIVE_AND_FULLY_SUPPORTED"}`,
	} {
		if _, err := DecodeWebSynthesisParagraphAuthorizationDecision(
			authorizationInput, raw,
		); err == nil {
			t.Fatalf("invalid paragraph authorization accepted: %q", raw)
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
