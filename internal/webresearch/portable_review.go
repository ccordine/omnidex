package webresearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (stations *PortableStations) Review(
	ctx context.Context,
	call ClaimEvidenceReviewCall,
) (ClaimEvidenceReviewDecision, error) {
	if err := validatePortableClaimEvidenceReviewCall(call); err != nil {
		return ClaimEvidenceReviewDecision{}, err
	}
	evidence := make([]assemblyline.WebReviewEvidence, len(call.Evidence))
	evidenceIDs := make([]string, len(call.Evidence))
	for index, item := range call.Evidence {
		evidence[index] = assemblyline.WebReviewEvidence{
			EvidenceID: string(item.EvidenceID), Title: item.Title,
			Snippet: item.Snippet, Content: item.Content,
		}
		evidenceIDs[index] = string(item.EvidenceID)
	}
	input := assemblyline.WebClaimEvidenceReviewInput{
		ExactQuestion: call.Question,
		Context:       assemblyline.CloneObjectiveContext(call.Context),
		Paragraph: assemblyline.WebReviewParagraph{
			ParagraphID: string(call.ParagraphID), Text: call.ParagraphText, EvidenceIDs: evidenceIDs,
		},
		Evidence: evidence,
	}
	job, err := assemblyline.NewWebClaimEvidenceReviewJob(input)
	if err != nil {
		return ClaimEvidenceReviewDecision{}, fmt.Errorf("build web claim-evidence review job: %w", err)
	}
	result, err := stations.run(ctx, job)
	if err != nil {
		return ClaimEvidenceReviewDecision{}, err
	}
	decision, err := assemblyline.DecodeWebClaimEvidenceReviewDecision(input, result.Candidate)
	if finalizeErr := stations.finalize(ctx, job, result, err); finalizeErr != nil {
		return ClaimEvidenceReviewDecision{}, combinePortableStationErrors(err, finalizeErr)
	}
	if err != nil {
		return ClaimEvidenceReviewDecision{}, err
	}
	ids := make([]EvidenceID, len(decision.EvidenceIDs))
	for index, id := range decision.EvidenceIDs {
		ids[index] = EvidenceID(id)
	}
	return ClaimEvidenceReviewDecision{
		Outcome: ClaimEvidenceReviewOutcome(decision.Outcome), ParagraphID: ParagraphID(decision.ParagraphID),
		EvidenceIDs: ids, IssueKind: ClaimEvidenceIssueKind(decision.IssueKind), Detail: decision.Detail,
	}, nil
}

func validatePortableClaimEvidenceReviewCall(call ClaimEvidenceReviewCall) error {
	if err := validatePortableQuestion(call.Question); err != nil {
		return err
	}
	if err := validatePortableObjectiveContext(call.Question, call.Context); err != nil {
		return err
	}
	if err := validatePortableIdentity(string(call.ParagraphID)); err != nil {
		return err
	}
	if err := validatePortableField(call.ParagraphText, maxPortableSynthesisParagraphBytes); err != nil ||
		strings.TrimSpace(call.ParagraphText) == "" {
		return fmt.Errorf("portable claim-evidence review paragraph is invalid")
	}
	if len(call.Evidence) < 1 || len(call.Evidence) > maxPortableReviewEvidence {
		return fmt.Errorf("portable claim-evidence review requires 1..%d cited evidence capsules", maxPortableReviewEvidence)
	}
	total := 0
	seen := make(map[EvidenceID]struct{}, len(call.Evidence))
	for _, item := range call.Evidence {
		if err := validatePortableIdentity(string(item.EvidenceID)); err != nil {
			return err
		}
		if _, duplicate := seen[item.EvidenceID]; duplicate {
			return fmt.Errorf("portable claim-evidence review evidence ID %q is duplicated", item.EvidenceID)
		}
		seen[item.EvidenceID] = struct{}{}
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("portable claim-evidence review evidence %q has no content", item.EvidenceID)
		}
		for _, value := range []string{item.Title, item.Snippet, item.Content} {
			if err := validatePortableField(value, maxPortableEvidenceFieldBytes); err != nil {
				return err
			}
		}
		total += len(item.EvidenceID) + len(item.Title) + len(item.Snippet) + len(item.Content)
	}
	if total > maxPortableEvidenceProjection {
		return fmt.Errorf("portable claim-evidence review projection exceeds %d bytes", maxPortableEvidenceProjection)
	}
	return nil
}
