package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func validateObjectiveGroundedAnswerCalls(
	dispatches int,
	input assemblyline.GroundedAnswerInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	maximum := (1 + assemblyline.MaxGroundedAnswerParagraphCandidates*(len(input.Evidence)+2)) *
		exactSemanticLeafCalls
	return validateObjectiveCallCount(
		"grounded answer station", dispatches, maximum,
	)
}
