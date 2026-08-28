package assemblyline

import (
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

func TestWebReviewLeavesSeparateClaimVerdictEvidenceAndDetail(t *testing.T) {
	base := webClaimEvidenceReviewFixture()
	claimInput := WebReviewClaimLeafInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, AcceptedClaims: []string{},
	}
	coverage, err := DecodeWebReviewClaimCoverageDecision(
		claimInput, string(WebReviewClaimRemains),
	)
	if err != nil || coverage.Coverage != WebReviewClaimRemains {
		t.Fatalf("coverage=%+v err=%v", coverage, err)
	}
	claim, err := DecodeWebReviewClaimDecision(claimInput, "Version 2 is current.")
	if err != nil {
		t.Fatal(err)
	}
	verdictInput := WebReviewClaimVerdictInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: claim.Claim, Evidence: base.Evidence,
	}
	verdict, err := DecodeWebReviewClaimVerdictDecision(
		verdictInput, string(WebReviewClaimInsufficient),
	)
	if err != nil {
		t.Fatal(err)
	}
	issueKind, issue := verdict.IssueKind()
	if !issue || issueKind != WebClaimEvidenceInsufficientSupport {
		t.Fatalf("verdict=%+v kind=%q issue=%v", verdict, issueKind, issue)
	}
	relationInput := WebReviewIssueEvidenceRelationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: claim.Claim,
		IssueKind: issueKind, Evidence: base.Evidence[0],
	}
	relation, err := DecodeWebReviewIssueEvidenceRelationDecision(
		relationInput, string(WebReviewEvidenceImplicated),
	)
	if err != nil || relation.Relation != WebReviewEvidenceImplicated {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}
	detailInput := WebReviewIssueDetailInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: claim.Claim,
		IssueKind: issueKind, Evidence: base.Evidence,
	}
	detail, err := DecodeWebReviewIssueDetailDecision(
		detailInput, "The cited evidence does not establish the claim.",
	)
	if err != nil || detail.Detail == "" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

func TestWebReviewPairwisePromptsHideOpaqueEvidenceIDs(t *testing.T) {
	base := webClaimEvidenceReviewFixture()
	verdictInput := WebReviewClaimVerdictInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: "Version 2 is current.",
		Evidence: base.Evidence,
	}
	prompt, err := BuildWebReviewClaimVerdictPrompt(verdictInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, base.Evidence[0].EvidenceID) ||
		!strings.Contains(prompt, base.Evidence[0].Content) {
		t.Fatalf("verdict prompt exposed ID or lost evidence content: %q", prompt)
	}
	relationInput := WebReviewIssueEvidenceRelationInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: "Version 2 is current.",
		IssueKind: WebClaimEvidenceContradictedSupport, Evidence: base.Evidence[0],
	}
	prompt, err = BuildWebReviewIssueEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, base.Evidence[0].EvidenceID) {
		t.Fatalf("issue evidence relation prompt exposed code-owned ID: %q", prompt)
	}
}

func TestWebReviewLeafDecodersRejectStructuredOrCombinedResults(t *testing.T) {
	base := webClaimEvidenceReviewFixture()
	claimInput := WebReviewClaimLeafInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, AcceptedClaims: []string{},
	}
	for _, raw := range []string{`{"claim":"Version 2 is current."}`, `"Version 2 is current."`, "claim one\nclaim two"} {
		if _, err := DecodeWebReviewClaimDecision(claimInput, raw); err == nil {
			t.Fatalf("invalid claim leaf accepted: %q", raw)
		}
	}
	verdictInput := WebReviewClaimVerdictInput{
		ExactQuestion: base.ExactQuestion, Context: base.Context,
		ParagraphText: base.Paragraph.Text, Claim: "Version 2 is current.",
		Evidence: base.Evidence,
	}
	for _, raw := range []string{`{"verdict":"SUPPORTED"}`, "SUPPORTED\nE31", "none"} {
		if _, err := DecodeWebReviewClaimVerdictDecision(verdictInput, raw); err == nil {
			t.Fatalf("invalid verdict leaf accepted: %q", raw)
		}
	}
}

func TestWebReviewDecisionIsAssembledWithCodeOwnedBindings(t *testing.T) {
	input := webClaimEvidenceReviewFixture()
	decision := WebClaimEvidenceReviewDecision{
		Schema:      WebClaimEvidenceReviewSchemaV1,
		Outcome:     WebClaimEvidenceReviewIssue,
		ParagraphID: input.Paragraph.ParagraphID,
		EvidenceIDs: []string{"E31"},
		IssueKind:   WebClaimEvidenceInsufficientSupport,
		Detail:      "The cited evidence does not establish the claim.",
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	decision.ParagraphID = "P9"
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("assembled issue accepted a model-invented paragraph identity")
	}
}
