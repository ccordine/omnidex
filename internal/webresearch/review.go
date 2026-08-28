package webresearch

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (machine *Machine) reviewSynthesis(
	ctx context.Context,
	paragraphs []GroundedParagraph,
	projected []ProjectedEvidence,
	result *Result,
) (*ClaimEvidenceReviewDecision, error) {
	byID := make(map[EvidenceID]ProjectedEvidence, len(projected))
	for _, evidence := range projected {
		byID[evidence.EvidenceID] = evidence
	}
	for index, paragraph := range paragraphs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		call := ClaimEvidenceReviewCall{
			Question:      machine.objective.Question,
			Context:       assemblyline.CloneObjectiveContext(machine.objective.Context),
			ParagraphID:   ParagraphID(fmt.Sprintf("P%d", index+1)),
			ParagraphText: paragraph.Text,
			Evidence:      make([]ProjectedEvidence, 0, len(paragraph.EvidenceIDs)),
		}
		for _, evidence := range projected {
			if paragraphCites(paragraph, evidence.EvidenceID) {
				call.Evidence = append(call.Evidence, evidence)
			}
		}
		if len(call.Evidence) != len(paragraph.EvidenceIDs) {
			return nil, fmt.Errorf("%w: paragraph %s lost cited evidence", ErrInvalidClaimEvidenceReview, call.ParagraphID)
		}
		for _, id := range paragraph.EvidenceIDs {
			if _, exists := byID[id]; !exists {
				return nil, fmt.Errorf("%w: paragraph %s cites unprojected evidence %q", ErrInvalidClaimEvidenceReview, call.ParagraphID, id)
			}
		}
		decision, err := machine.review.Review(ctx, cloneClaimEvidenceReviewCall(call))
		if err != nil {
			return nil, fmt.Errorf("claim-evidence review station: %w", err)
		}
		if decision.SemanticCalls < 1 {
			return nil, fmt.Errorf("%w: review reported no semantic calls", ErrInvalidClaimEvidenceReview)
		}
		result.ClaimEvidenceReviewCalls++
		result.SemanticCalls += decision.SemanticCalls
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validateClaimEvidenceReviewDecision(decision, call); err != nil {
			return nil, err
		}
		if decision.Outcome == ClaimEvidenceReviewIssue {
			owned := cloneClaimEvidenceReviewDecision(decision)
			return &owned, nil
		}
	}
	return nil, nil
}

func claimEvidenceIssueError(decision ClaimEvidenceReviewDecision) error {
	return fmt.Errorf(
		"%w after one bounded correction: paragraph=%s evidence=%v kind=%s detail=%s",
		ErrClaimEvidenceInadequate, decision.ParagraphID, decision.EvidenceIDs,
		decision.IssueKind, decision.Detail,
	)
}

func paragraphCites(paragraph GroundedParagraph, id EvidenceID) bool {
	for _, cited := range paragraph.EvidenceIDs {
		if cited == id {
			return true
		}
	}
	return false
}

func validateClaimEvidenceReviewDecision(
	decision ClaimEvidenceReviewDecision,
	call ClaimEvidenceReviewCall,
) error {
	if decision.SemanticCalls < 1 {
		return fmt.Errorf("%w: semantic call count must be positive", ErrInvalidClaimEvidenceReview)
	}
	if decision.EvidenceIDs == nil {
		return fmt.Errorf("%w: evidence IDs must be an explicit array", ErrInvalidClaimEvidenceReview)
	}
	if decision.Outcome == ClaimEvidenceReviewNone {
		if decision.ParagraphID != "" || len(decision.EvidenceIDs) != 0 ||
			decision.IssueKind != "" || decision.Detail != "" {
			return fmt.Errorf("%w: NONE must contain no issue fields", ErrInvalidClaimEvidenceReview)
		}
		return nil
	}
	if decision.Outcome != ClaimEvidenceReviewIssue {
		return fmt.Errorf("%w: outcome %q is unsupported", ErrInvalidClaimEvidenceReview, decision.Outcome)
	}
	if decision.ParagraphID != call.ParagraphID {
		return fmt.Errorf("%w: issue is not bound to reviewed paragraph", ErrInvalidClaimEvidenceReview)
	}
	if len(decision.EvidenceIDs) < 1 || len(decision.EvidenceIDs) > len(call.Evidence) {
		return fmt.Errorf("%w: issue evidence cardinality is invalid", ErrInvalidClaimEvidenceReview)
	}
	available := make(map[EvidenceID]struct{}, len(call.Evidence))
	for _, item := range call.Evidence {
		available[item.EvidenceID] = struct{}{}
	}
	seen := make(map[EvidenceID]struct{}, len(decision.EvidenceIDs))
	for _, id := range decision.EvidenceIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("%w: issue cites unprojected evidence %q", ErrInvalidClaimEvidenceReview, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: issue duplicates evidence %q", ErrInvalidClaimEvidenceReview, id)
		}
		seen[id] = struct{}{}
	}
	switch decision.IssueKind {
	case ClaimEvidenceInsufficientSupport, ClaimEvidenceContradictedSupport, ClaimEvidenceQuestionMismatch:
	default:
		return fmt.Errorf("%w: issue kind %q is unsupported", ErrInvalidClaimEvidenceReview, decision.IssueKind)
	}
	if decision.Detail == "" || decision.Detail != strings.TrimSpace(decision.Detail) ||
		trimOneLine(decision.Detail) == "" || len(decision.Detail) > maxPortableReviewDetailBytes ||
		!utf8.ValidString(decision.Detail) {
		return fmt.Errorf("%w: issue detail must be one bounded trimmed line", ErrInvalidClaimEvidenceReview)
	}
	return nil
}

func trimOneLine(value string) string {
	for _, character := range value {
		if character == '\r' || character == '\n' || character == '\x00' {
			return ""
		}
	}
	return value
}
