package cognitiongauntlet

import "fmt"

func validateFullCognitionScaleRequests(
	cases []FullCognitionScaleCaseRequest,
	first FullCognitionRunRequest,
) error {
	repetitions := make([]int, len(cases[0].Runs))
	for index, run := range cases[0].Runs {
		repetitions[index] = run.Repetition
	}
	for caseIndex, item := range cases {
		if len(item.Runs) != len(repetitions) || len(item.Runs) == 0 {
			return fmt.Errorf("full cognition scale repetitions differ at world %d", item.WorldSize)
		}
		for runIndex, run := range item.Runs {
			if err := run.Validate(); err != nil {
				return fmt.Errorf("full cognition scale case %d run %d: %w", caseIndex+1, runIndex+1, err)
			}
			if run.Surface != SurfaceSymbolic || run.Repetition != repetitions[runIndex] ||
				run.RatGeneration != first.RatGeneration ||
				run.RuntimeFingerprint != first.RuntimeFingerprint {
				return fmt.Errorf("full cognition scale changed surface, Rat identity, runtime, or repetition")
			}
			if runIndex > 0 && repetitions[runIndex] <= repetitions[runIndex-1] {
				return fmt.Errorf("full cognition scale repetitions must be positive and sorted")
			}
		}
	}
	return nil
}
