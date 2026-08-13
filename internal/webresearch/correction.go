package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (machine *Machine) correctSynthesis(
	ctx context.Context,
	paragraphs []GroundedParagraph,
	projected []ProjectedEvidence,
	issue ClaimEvidenceReviewDecision,
	result *Result,
) ([]GroundedParagraph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call := GroundedSynthesisCorrectionCall{
		Question:   machine.objective.Question,
		Context:    assemblyline.CloneObjectiveContext(machine.objective.Context),
		Paragraphs: cloneParagraphs(paragraphs),
		Issue:      cloneClaimEvidenceReviewDecision(issue), Evidence: cloneProjection(projected),
		MaxParagraphBytes: machine.config.MaxSynthesisParagraphBytes,
	}
	decision, err := machine.correction.Correct(ctx, cloneGroundedSynthesisCorrectionCall(call))
	result.SynthesisCorrectionCalls++
	if err != nil {
		return nil, fmt.Errorf("grounded synthesis correction station: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateGroundedSynthesisCorrectionDecision(decision, call); err != nil {
		return nil, err
	}
	corrected := cloneParagraphs(paragraphs)
	index := paragraphIndexForID(call.Issue.ParagraphID, len(corrected))
	if index < 0 {
		return nil, fmt.Errorf("%w: corrected paragraph ID is unbound", ErrInvalidSynthesisCorrection)
	}
	retainedEvidenceIDs := append([]EvidenceID{}, corrected[index].EvidenceIDs...)
	corrected[index].Text = decision.Text
	corrected[index].EvidenceIDs = retainedEvidenceIDs
	return corrected, nil
}

func validateGroundedSynthesisCorrectionDecision(
	decision GroundedSynthesisCorrectionDecision,
	call GroundedSynthesisCorrectionCall,
) error {
	input, err := portableSynthesisCorrectionInput(call)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSynthesisCorrection, err)
	}
	assemblyDecision := assemblylineCorrectionDecision(decision)
	if err := assemblyDecision.ValidateFor(input); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSynthesisCorrection, err)
	}
	return nil
}

func paragraphIndexForID(id ParagraphID, length int) int {
	for index := 0; index < length; index++ {
		if id == ParagraphID(fmt.Sprintf("P%d", index+1)) {
			return index
		}
	}
	return -1
}
