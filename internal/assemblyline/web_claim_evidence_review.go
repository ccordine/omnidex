package assemblyline

import (
	"fmt"
	"strings"
)

const WebClaimEvidenceReviewSchemaV1 = "omnidex.web-claim-evidence-review.v1"

type WebClaimEvidenceReviewOutcome string
type WebClaimEvidenceIssueKind string

const (
	WebClaimEvidenceReviewNone  WebClaimEvidenceReviewOutcome = "none"
	WebClaimEvidenceReviewIssue WebClaimEvidenceReviewOutcome = "issue"

	WebClaimEvidenceInsufficientSupport WebClaimEvidenceIssueKind = "insufficient_support"
	WebClaimEvidenceContradictedSupport WebClaimEvidenceIssueKind = "contradicted_support"
	WebClaimEvidenceQuestionMismatch    WebClaimEvidenceIssueKind = "question_mismatch"
)

type WebReviewParagraph struct {
	ParagraphID string   `json:"paragraph_id"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type WebReviewEvidence struct {
	EvidenceID string `json:"evidence_id"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	Content    string `json:"content"`
}

type WebClaimEvidenceReviewInput struct {
	ExactQuestion string              `json:"exact_question"`
	Context       ObjectiveContext    `json:"objective_context"`
	Paragraph     WebReviewParagraph  `json:"paragraph"`
	Evidence      []WebReviewEvidence `json:"evidence"`
}

type WebClaimEvidenceReviewDecision struct {
	Schema      string                        `json:"schema"`
	Outcome     WebClaimEvidenceReviewOutcome `json:"outcome"`
	ParagraphID string                        `json:"paragraph_id"`
	EvidenceIDs []string                      `json:"evidence_ids"`
	IssueKind   WebClaimEvidenceIssueKind     `json:"issue_kind"`
	Detail      string                        `json:"detail"`
}

func (input WebClaimEvidenceReviewInput) validate() error {
	if err := validateExactWebQuestion(input.ExactQuestion); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if err := validateWebLine("paragraph ID", input.Paragraph.ParagraphID, maxWebEvidenceIDBytes); err != nil {
		return err
	}
	if err := validateWebText("paragraph text", input.Paragraph.Text, maxWebSynthesisParagraphBytes, true); err != nil {
		return err
	}
	if input.Paragraph.Text != strings.TrimSpace(input.Paragraph.Text) || webModelCitationSyntax.MatchString(input.Paragraph.Text) {
		return fmt.Errorf("web claim-evidence review paragraph must be an exact unrendered synthesis paragraph")
	}
	if input.Paragraph.EvidenceIDs == nil || len(input.Paragraph.EvidenceIDs) < 1 ||
		len(input.Paragraph.EvidenceIDs) > maxWebEvidenceIDsPerParagraph {
		return fmt.Errorf("web claim-evidence review paragraph requires 1..%d evidence IDs", maxWebEvidenceIDsPerParagraph)
	}
	if len(input.Evidence) != len(input.Paragraph.EvidenceIDs) {
		return fmt.Errorf("web claim-evidence review must project exactly the paragraph evidence")
	}
	available := make(map[string]struct{}, len(input.Evidence))
	total := 0
	for index, evidence := range input.Evidence {
		if err := validateWebLine("evidence ID", evidence.EvidenceID, maxWebEvidenceIDBytes); err != nil {
			return fmt.Errorf("web review evidence %d: %w", index, err)
		}
		if _, duplicate := available[evidence.EvidenceID]; duplicate {
			return fmt.Errorf("web review evidence ID %q is duplicated", evidence.EvidenceID)
		}
		available[evidence.EvidenceID] = struct{}{}
		for _, field := range []struct {
			label string
			value string
		}{
			{label: "title", value: evidence.Title},
			{label: "snippet", value: evidence.Snippet},
			{label: "content", value: evidence.Content},
		} {
			if err := validateWebText(field.label, field.value, maxWebReviewEvidenceProjectionBytes, false); err != nil {
				return fmt.Errorf("web review evidence %s: %w", evidence.EvidenceID, err)
			}
		}
		if strings.TrimSpace(evidence.Content) == "" {
			return fmt.Errorf("web review evidence %q has no content", evidence.EvidenceID)
		}
		total += len(evidence.EvidenceID) + len(evidence.Title) + len(evidence.Snippet) + len(evidence.Content)
	}
	if total > maxWebReviewEvidenceProjectionBytes {
		return fmt.Errorf("web claim-evidence review projection exceeds %d bytes", maxWebReviewEvidenceProjectionBytes)
	}
	seen := make(map[string]struct{}, len(input.Paragraph.EvidenceIDs))
	for _, id := range input.Paragraph.EvidenceIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("web review paragraph evidence ID %q was not projected", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("web review paragraph evidence ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (decision WebClaimEvidenceReviewDecision) ValidateFor(input WebClaimEvidenceReviewInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != WebClaimEvidenceReviewSchemaV1 {
		return fmt.Errorf("web claim-evidence review schema must be %q", WebClaimEvidenceReviewSchemaV1)
	}
	if decision.Outcome == WebClaimEvidenceReviewNone {
		if decision.ParagraphID != "" || decision.EvidenceIDs != nil || decision.IssueKind != "" || decision.Detail != "" {
			return fmt.Errorf("web claim-evidence review NONE must omit all issue fields")
		}
		return nil
	}
	if decision.Outcome != WebClaimEvidenceReviewIssue {
		return fmt.Errorf("web claim-evidence review outcome %q is unsupported", decision.Outcome)
	}
	if decision.EvidenceIDs == nil {
		return fmt.Errorf("web claim-evidence issue evidence IDs must be an explicit array")
	}
	if decision.ParagraphID != input.Paragraph.ParagraphID {
		return fmt.Errorf("web claim-evidence issue is not bound to the reviewed paragraph")
	}
	if len(decision.EvidenceIDs) < 1 || len(decision.EvidenceIDs) > len(input.Evidence) {
		return fmt.Errorf("web claim-evidence issue requires 1..%d evidence IDs", len(input.Evidence))
	}
	available := make(map[string]struct{}, len(input.Evidence))
	for _, evidence := range input.Evidence {
		available[evidence.EvidenceID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.EvidenceIDs))
	for _, id := range decision.EvidenceIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("web claim-evidence issue cites unprojected evidence %q", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("web claim-evidence issue duplicates evidence %q", id)
		}
		seen[id] = struct{}{}
	}
	switch decision.IssueKind {
	case WebClaimEvidenceInsufficientSupport, WebClaimEvidenceContradictedSupport, WebClaimEvidenceQuestionMismatch:
	default:
		return fmt.Errorf("web claim-evidence issue kind %q is unsupported", decision.IssueKind)
	}
	if decision.Detail == "" || decision.Detail != strings.TrimSpace(decision.Detail) || strings.ContainsAny(decision.Detail, "\r\n") {
		return fmt.Errorf("web claim-evidence issue detail must be one non-empty trimmed line")
	}
	return validateWebText("issue detail", decision.Detail, maxWebReviewIssueDetailBytes, true)
}
