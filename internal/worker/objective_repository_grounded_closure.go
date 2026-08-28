package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type objectiveRepositoryGroundingStation interface {
	Answer(context.Context, assemblyline.GroundedAnswerInput) (
		assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error,
	)
	Review(context.Context, assemblyline.RepositoryGroundedReviewInput) (
		assemblyline.RepositoryGroundedReviewDecision, objectiveStationReceipt, error,
	)
	Correct(context.Context, assemblyline.RepositoryGroundedCorrectionInput) (
		assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error,
	)
}

type objectiveRepositoryGroundingPreflight interface {
	ValidateRepositoryGrounding() error
}

type objectiveRepositoryGroundedResult struct {
	Answer          assemblyline.GroundedAnswerDecision
	ModelCalls      int
	ReviewCalls     int
	CorrectionCalls int
}

func runObjectiveRepositoryGroundedClosure(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
	stations objectiveRepositoryGroundingStation,
) (objectiveRepositoryGroundedResult, error) {
	if ctx == nil || stations == nil {
		return objectiveRepositoryGroundedResult{}, fmt.Errorf("repository grounded closure requires context and exact stations")
	}
	if err := ctx.Err(); err != nil {
		return objectiveRepositoryGroundedResult{}, err
	}
	if err := input.Validate(); err != nil {
		return objectiveRepositoryGroundedResult{}, err
	}
	if preflight, ok := stations.(objectiveRepositoryGroundingPreflight); ok {
		if err := preflight.ValidateRepositoryGrounding(); err != nil {
			return objectiveRepositoryGroundedResult{}, err
		}
	}
	answer, receipt, err := stations.Answer(ctx, cloneGroundedAnswerInput(input))
	if err != nil {
		return objectiveRepositoryGroundedResult{}, err
	}
	result := objectiveRepositoryGroundedResult{ModelCalls: receipt.Calls}
	if err := validateObjectiveGroundedAnswerCalls(receipt.Calls, input); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := answer.ValidateFor(input); err != nil {
		return result, err
	}

	reviewInput, err := objectiveRepositoryReviewInput(input, answer)
	if err != nil {
		return result, err
	}
	if err := reviewInput.Validate(); err != nil {
		return result, err
	}
	issue, reviewReceipt, err := stations.Review(ctx, cloneRepositoryReviewInput(reviewInput))
	result.ModelCalls += reviewReceipt.Calls
	result.ReviewCalls++
	if err != nil {
		return result, err
	}
	if err := validateObjectiveRepositoryReviewCalls("review", reviewReceipt.Calls); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := issue.ValidateFor(reviewInput); err != nil {
		return result, err
	}
	if issue.Outcome == assemblyline.RepositoryGroundedReviewNone {
		result.Answer = answer
		return result, nil
	}

	correctionInput := assemblyline.RepositoryGroundedCorrectionInput{
		RequirementID: input.RequirementID, ExactRequirement: input.ExactRequirement,
		Context:     assemblyline.CloneObjectiveContext(input.Context),
		CurrentText: answer.Text, EvidenceIDs: append([]string(nil), answer.EvidenceIDs...),
		Evidence: cloneGroundedEvidence(reviewInput.Evidence), Issue: issue,
	}
	if _, err := assemblyline.NewRepositoryGroundedCorrectionJob(correctionInput); err != nil {
		return result, err
	}
	correction, correctionReceipt, err := stations.Correct(ctx, cloneRepositoryCorrectionInput(correctionInput))
	result.ModelCalls += correctionReceipt.Calls
	result.CorrectionCalls++
	if err != nil {
		return result, err
	}
	if err := validateObjectiveRepositoryStationCalls("correction", correctionReceipt.Calls); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := correction.ValidateFor(correctionInput); err != nil {
		return result, err
	}
	answer.Text = correction.Text
	if err := answer.ValidateFor(input); err != nil {
		return result, fmt.Errorf("repository grounded correction broke retained answer authority: %w", err)
	}

	reviewInput.AnswerText = answer.Text
	secondReview, secondReceipt, err := stations.Review(ctx, cloneRepositoryReviewInput(reviewInput))
	result.ModelCalls += secondReceipt.Calls
	result.ReviewCalls++
	if err != nil {
		return result, err
	}
	if err := validateObjectiveRepositoryReviewCalls("second review", secondReceipt.Calls); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := secondReview.ValidateFor(reviewInput); err != nil {
		return result, err
	}
	if secondReview.Outcome != assemblyline.RepositoryGroundedReviewNone {
		return result, fmt.Errorf(
			"repository grounded answer remained unsupported after its one bounded text correction: %s",
			secondReview.Detail,
		)
	}
	result.Answer = answer
	return result, nil
}

func objectiveRepositoryReviewInput(
	input assemblyline.GroundedAnswerInput,
	answer assemblyline.GroundedAnswerDecision,
) (assemblyline.RepositoryGroundedReviewInput, error) {
	byID := make(map[string]assemblyline.GroundedEvidenceCapsule, len(input.Evidence))
	for _, evidence := range input.Evidence {
		byID[evidence.ID] = evidence
	}
	cited := make([]assemblyline.GroundedEvidenceCapsule, len(answer.EvidenceIDs))
	for index, id := range answer.EvidenceIDs {
		evidence, exists := byID[id]
		if !exists {
			return assemblyline.RepositoryGroundedReviewInput{}, fmt.Errorf("repository grounded answer cites unavailable evidence %q", id)
		}
		cited[index] = evidence
	}
	review := assemblyline.RepositoryGroundedReviewInput{
		RequirementID: input.RequirementID, ExactRequirement: input.ExactRequirement,
		Context:    assemblyline.CloneObjectiveContext(input.Context),
		AnswerText: answer.Text, EvidenceIDs: append([]string(nil), answer.EvidenceIDs...),
		Evidence: cited,
	}
	if err := review.Validate(); err != nil {
		return assemblyline.RepositoryGroundedReviewInput{}, err
	}
	return review, nil
}

func validateObjectiveRepositoryStationCalls(label string, calls int) error {
	if calls < 1 || calls > maxTypedWorkerAttempts {
		return fmt.Errorf("repository grounded %s station reported %d calls outside the bounded correction budget", label, calls)
	}
	return nil
}

func validateObjectiveRepositoryReviewCalls(label string, calls int) error {
	maximum := 2 * maxTypedWorkerAttempts
	if calls < 1 || calls > maximum {
		return fmt.Errorf(
			"repository grounded %s reported %d calls outside the two-leaf correction budget",
			label, calls,
		)
	}
	return nil
}

func validateObjectiveGroundedAnswerCalls(
	calls int,
	input assemblyline.GroundedAnswerInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	maximum := (1 + len(input.Evidence)) * maxTypedWorkerAttempts
	if calls < 1 || calls > maximum {
		return fmt.Errorf(
			"repository grounded answer station reported %d calls outside the %d-leaf correction budget",
			calls, maximum,
		)
	}
	return nil
}

func cloneGroundedEvidence(items []assemblyline.GroundedEvidenceCapsule) []assemblyline.GroundedEvidenceCapsule {
	return append([]assemblyline.GroundedEvidenceCapsule(nil), items...)
}

func cloneRepositoryReviewInput(input assemblyline.RepositoryGroundedReviewInput) assemblyline.RepositoryGroundedReviewInput {
	copy := input
	copy.Context = assemblyline.CloneObjectiveContext(input.Context)
	copy.EvidenceIDs = append([]string(nil), input.EvidenceIDs...)
	copy.Evidence = cloneGroundedEvidence(input.Evidence)
	return copy
}

func cloneRepositoryCorrectionInput(input assemblyline.RepositoryGroundedCorrectionInput) assemblyline.RepositoryGroundedCorrectionInput {
	copy := input
	copy.Context = assemblyline.CloneObjectiveContext(input.Context)
	copy.EvidenceIDs = append([]string(nil), input.EvidenceIDs...)
	copy.Evidence = cloneGroundedEvidence(input.Evidence)
	return copy
}
