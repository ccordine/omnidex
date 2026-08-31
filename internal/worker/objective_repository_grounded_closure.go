package worker

import "github.com/gryph/omnidex/internal/assemblyline"

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
		"grounded answer station", receipt, maximum,
	)
}
