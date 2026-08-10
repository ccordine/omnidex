package cognitiongauntlet

import (
	"context"
	"fmt"
)

func RunFullCognitionScale(
	ctx context.Context,
	base MicrogauntletCase,
	request FullCognitionScaleRequest,
) (FullCognitionScaleResult, error) {
	if ctx == nil {
		return FullCognitionScaleResult{}, fmt.Errorf("full cognition scale context is nil")
	}
	fixtures, _, authority, err := prepareFullCognitionScale(base, request)
	if err != nil {
		return FullCognitionScaleResult{}, err
	}
	results := make([]FullCognitionScaleCaseResult, len(fixtures))
	measurements := make([]ScaleMeasurement, len(fixtures))
	for caseIndex, fixture := range fixtures {
		runs := make([]FullCognitionRunResult, len(request.Cases[caseIndex].Runs))
		for runIndex, runRequest := range request.Cases[caseIndex].Runs {
			run, runErr := RunFullCognition(ctx, fixture, runRequest)
			if runErr != nil {
				return FullCognitionScaleResult{}, fmt.Errorf(
					"execute full cognition scale world %d repetition %d: %w",
					request.Cases[caseIndex].WorldSize, runRequest.Repetition, runErr,
				)
			}
			runs[runIndex] = run
		}
		measurement, measureErr := measureFullCognitionScaleCase(authority, fixture, runs)
		if measureErr != nil {
			return FullCognitionScaleResult{}, measureErr
		}
		results[caseIndex] = FullCognitionScaleCaseResult{
			WorldSize: request.Cases[caseIndex].WorldSize, Runs: runs,
		}
		measurements[caseIndex] = measurement
	}
	report, err := EvaluateScaleRail(authority, measurements)
	if err != nil {
		return FullCognitionScaleResult{}, err
	}
	result := FullCognitionScaleResult{Authority: authority, Cases: results, Report: report}
	return result, result.Validate()
}
