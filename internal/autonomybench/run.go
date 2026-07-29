package autonomybench

import (
	"context"
	"fmt"
	"time"
)

func Run(
	ctx context.Context,
	request RequestCase,
	builder Builder,
	loader EvaluationLoader,
	evaluator Evaluator,
) (Result, error) {
	started := time.Now()
	result := Result{RequestID: request.ID, Workspace: request.Workspace}
	if err := validateRequest(request); err != nil {
		return result, err
	}
	if builder == nil || loader == nil || evaluator == nil {
		return result, fmt.Errorf("autonomy benchmark requires builder, evaluation loader, and evaluator")
	}

	observation, buildErr := builder.Build(ctx, BuildInput{
		UserRequest: request.UserRequest,
		Workspace:   request.Workspace,
	})
	result.Build = observation
	if buildErr != nil {
		result.BuildError = buildErr.Error()
	}

	// The rubric is intentionally loaded only after Build has returned. This is
	// the hard boundary preventing expected answers from becoming model context.
	plan, err := loader.Load(ctx, request.ID)
	if err != nil {
		return result, fmt.Errorf("load withheld evaluation: %w", err)
	}
	if err := validateEvaluationPlan(request.ID, plan); err != nil {
		return result, err
	}
	rawResults, err := evaluator.Evaluate(ctx, EvaluationInput{
		Workspace: request.Workspace,
		Checks:    append([]Check(nil), plan.Checks...),
	})
	if err != nil {
		return result, fmt.Errorf("evaluate built workspace: %w", err)
	}
	result.Checks, err = orderAndValidateResults(plan, rawResults)
	if err != nil {
		return result, err
	}

	for index, check := range plan.Checks {
		result.TotalWeight += check.Weight
		if result.Checks[index].Passed {
			result.EarnedWeight += check.Weight
		}
	}
	result.Status = benchmarkStatus(result.EarnedWeight, result.TotalWeight, buildErr)
	result.Elapsed = time.Since(started)
	return result, nil
}

func benchmarkStatus(earned, total int, buildErr error) Status {
	if buildErr == nil && earned == total {
		return StatusPassed
	}
	if earned > 0 {
		return StatusPartial
	}
	return StatusFailed
}
