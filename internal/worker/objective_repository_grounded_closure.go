package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type objectiveRepositoryGroundedResult struct {
	Answer     assemblyline.GroundedAnswerDecision
	ModelCalls int
}

func runObjectiveRepositoryGroundedClosure(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
	station objectiveAnswerStation,
) (objectiveRepositoryGroundedResult, error) {
	if ctx == nil || station == nil {
		return objectiveRepositoryGroundedResult{}, fmt.Errorf(
			"repository grounded closure requires context and one grounded-answer station",
		)
	}
	if err := ctx.Err(); err != nil {
		return objectiveRepositoryGroundedResult{}, err
	}
	if err := input.Validate(); err != nil {
		return objectiveRepositoryGroundedResult{}, err
	}
	answer, receipt, err := station.Answer(ctx, cloneGroundedAnswerInput(input))
	result := objectiveRepositoryGroundedResult{ModelCalls: receipt.Calls}
	if err != nil {
		return result, err
	}
	if err := validateObjectiveGroundedAnswerReceipt(receipt, input); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := answer.ValidateFor(input); err != nil {
		return result, err
	}
	result.Answer = answer
	return result, nil
}

func validateObjectiveGroundedAnswerReceipt(
	receipt objectiveStationReceipt,
	input assemblyline.GroundedAnswerInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	maximum := (1 + assemblyline.MaxGroundedAnswerParagraphCandidates*(len(input.Evidence)+1)) *
		exactSemanticLeafCalls
	return validateObjectiveBoundedStationReceipt(
		"repository grounded answer station", receipt, maximum,
	)
}
