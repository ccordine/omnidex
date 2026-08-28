package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (stations *PortableStations) Correct(
	ctx context.Context,
	call GroundedSynthesisCorrectionCall,
) (GroundedSynthesisCorrectionDecision, error) {
	input, err := portableSynthesisCorrectionInput(call)
	if err != nil {
		return GroundedSynthesisCorrectionDecision{}, err
	}
	job, err := assemblyline.NewWebGroundedSynthesisCorrectionJob(input)
	if err != nil {
		return GroundedSynthesisCorrectionDecision{}, fmt.Errorf("build web grounded synthesis correction job: %w", err)
	}
	result, err := stations.run(ctx, job)
	if err != nil {
		return GroundedSynthesisCorrectionDecision{}, err
	}
	decision, err := assemblyline.DecodeWebGroundedSynthesisCorrectionDecision(input, result.Candidate)
	if finalizeErr := stations.finalize(ctx, job, result, err); finalizeErr != nil {
		return GroundedSynthesisCorrectionDecision{}, combinePortableStationErrors(err, finalizeErr)
	}
	if err != nil {
		return GroundedSynthesisCorrectionDecision{}, err
	}
	return GroundedSynthesisCorrectionDecision{Text: decision.Text, SemanticCalls: 1}, nil
}

func portableSynthesisCorrectionInput(
	call GroundedSynthesisCorrectionCall,
) (assemblyline.WebGroundedSynthesisCorrectionInput, error) {
	if err := validatePortableSynthesisCorrectionCall(call); err != nil {
		return assemblyline.WebGroundedSynthesisCorrectionInput{}, err
	}
	paragraphs := make([]assemblyline.WebReviewParagraph, len(call.Paragraphs))
	for index, paragraph := range call.Paragraphs {
		ids := make([]string, len(paragraph.EvidenceIDs))
		for idIndex, id := range paragraph.EvidenceIDs {
			ids[idIndex] = string(id)
		}
		paragraphs[index] = assemblyline.WebReviewParagraph{
			ParagraphID: fmt.Sprintf("P%d", index+1), Text: paragraph.Text, EvidenceIDs: ids,
		}
	}
	evidence := make([]assemblyline.WebGroundedEvidence, len(call.Evidence))
	for index, item := range call.Evidence {
		evidence[index] = assemblyline.WebGroundedEvidence{
			EvidenceID: string(item.EvidenceID), Title: item.Title,
			Snippet: item.Snippet, Content: item.Content,
		}
	}
	issueIDs := make([]string, len(call.Issue.EvidenceIDs))
	for index, id := range call.Issue.EvidenceIDs {
		issueIDs[index] = string(id)
	}
	return assemblyline.WebGroundedSynthesisCorrectionInput{
		ExactQuestion: call.Question,
		Context:       assemblyline.CloneObjectiveContext(call.Context),
		Paragraphs:    paragraphs, Evidence: evidence,
		Issue: assemblyline.WebClaimEvidenceReviewDecision{
			Schema:      assemblyline.WebClaimEvidenceReviewSchemaV1,
			Outcome:     assemblyline.WebClaimEvidenceReviewOutcome(call.Issue.Outcome),
			ParagraphID: string(call.Issue.ParagraphID), EvidenceIDs: issueIDs,
			IssueKind: assemblyline.WebClaimEvidenceIssueKind(call.Issue.IssueKind), Detail: call.Issue.Detail,
		},
		MaxParagraphBytes: call.MaxParagraphBytes,
	}, nil
}

func assemblylineCorrectionDecision(
	decision GroundedSynthesisCorrectionDecision,
) assemblyline.WebGroundedSynthesisCorrectionDecision {
	return assemblyline.WebGroundedSynthesisCorrectionDecision{Text: decision.Text}
}

func validatePortableSynthesisCorrectionCall(call GroundedSynthesisCorrectionCall) error {
	if len(call.Paragraphs) < 1 || len(call.Paragraphs) > maxPortableSynthesisParagraphs {
		return fmt.Errorf("portable synthesis correction requires bounded retained paragraphs")
	}
	if call.MaxParagraphBytes < 1 || call.MaxParagraphBytes > maxPortableSynthesisParagraphBytes {
		return fmt.Errorf("portable synthesis correction paragraph byte bound is invalid")
	}
	if call.Issue.Outcome != ClaimEvidenceReviewIssue || call.Issue.EvidenceIDs == nil {
		return fmt.Errorf("portable synthesis correction requires one exact review issue")
	}
	return validatePortableSynthesisCall(GroundedSynthesisCall{
		Question:      call.Question,
		Context:       call.Context,
		Evidence:      call.Evidence,
		MaxParagraphs: len(call.Paragraphs), MaxParagraphBytes: call.MaxParagraphBytes,
	})
}
