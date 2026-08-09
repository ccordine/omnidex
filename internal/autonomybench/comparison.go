package autonomybench

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const ComparisonSchemaV1 = "omnidex.autonomy-benchmark-comparison.v1"

type ComparisonRequest struct {
	ID                string
	UserRequest       string
	BaselineWorkspace string
	AssistedWorkspace string
}

type ComparisonCheck struct {
	ID               string `json:"id"`
	Weight           int    `json:"weight"`
	BaselinePassed   bool   `json:"baseline_passed"`
	AssistedPassed   bool   `json:"assisted_passed"`
	BaselineEvidence string `json:"baseline_evidence"`
	AssistedEvidence string `json:"assisted_evidence"`
}

type ComparisonResult struct {
	Schema            string            `json:"schema"`
	RequestID         string            `json:"request_id"`
	Baseline          Result            `json:"baseline"`
	Assisted          Result            `json:"assisted"`
	EarnedWeightDelta int               `json:"earned_weight_delta"`
	Checks            []ComparisonCheck `json:"checks"`
	Elapsed           time.Duration     `json:"elapsed"`
}

func RunComparison(
	ctx context.Context,
	request ComparisonRequest,
	baselineBuilder Builder,
	assistedBuilder Builder,
	loader EvaluationLoader,
	evaluator Evaluator,
) (ComparisonResult, error) {
	started := time.Now()
	result := ComparisonResult{Schema: ComparisonSchemaV1, RequestID: request.ID}
	if err := validateComparisonRequest(request); err != nil {
		return result, err
	}
	if baselineBuilder == nil || assistedBuilder == nil || loader == nil || evaluator == nil {
		return result, fmt.Errorf("autonomy comparison requires two builders, one evaluation loader, and one evaluator")
	}

	result.Baseline = buildComparisonVariant(ctx, request.ID, request.UserRequest, request.BaselineWorkspace, baselineBuilder)
	result.Assisted = buildComparisonVariant(ctx, request.ID, request.UserRequest, request.AssistedWorkspace, assistedBuilder)

	// Both builders must stop before the withheld rubric enters process state.
	plan, err := loader.Load(ctx, request.ID)
	if err != nil {
		return result, fmt.Errorf("load withheld comparison evaluation: %w", err)
	}
	if err := validateEvaluationPlan(request.ID, plan); err != nil {
		return result, err
	}
	if err := evaluateComparisonVariant(ctx, &result.Baseline, plan, evaluator); err != nil {
		return result, fmt.Errorf("evaluate baseline workspace: %w", err)
	}
	if err := evaluateComparisonVariant(ctx, &result.Assisted, plan, evaluator); err != nil {
		return result, fmt.Errorf("evaluate assisted workspace: %w", err)
	}
	result.Checks = compareChecks(plan, result.Baseline.Checks, result.Assisted.Checks)
	result.EarnedWeightDelta = result.Assisted.EarnedWeight - result.Baseline.EarnedWeight
	result.Elapsed = time.Since(started)
	return result, nil
}

func validateComparisonRequest(request ComparisonRequest) error {
	baseline := RequestCase{ID: request.ID, UserRequest: request.UserRequest, Workspace: request.BaselineWorkspace}
	if err := validateRequest(baseline); err != nil {
		return err
	}
	assisted := RequestCase{ID: request.ID, UserRequest: request.UserRequest, Workspace: request.AssistedWorkspace}
	if err := validateRequest(assisted); err != nil {
		return err
	}
	if request.BaselineWorkspace != strings.TrimSpace(request.BaselineWorkspace) ||
		request.AssistedWorkspace != strings.TrimSpace(request.AssistedWorkspace) {
		return fmt.Errorf("autonomy comparison workspace paths must not contain surrounding whitespace")
	}
	if filepath.Clean(request.BaselineWorkspace) == filepath.Clean(request.AssistedWorkspace) {
		return fmt.Errorf("autonomy comparison requires distinct baseline and assisted workspaces")
	}
	return nil
}

func buildComparisonVariant(ctx context.Context, requestID, userRequest, workspace string, builder Builder) Result {
	started := time.Now()
	result := Result{RequestID: requestID, Workspace: workspace}
	observation, err := builder.Build(ctx, BuildInput{UserRequest: userRequest, Workspace: workspace})
	result.Build = observation
	if err != nil {
		result.BuildError = err.Error()
	}
	result.Elapsed = time.Since(started)
	return result
}

func evaluateComparisonVariant(ctx context.Context, result *Result, plan EvaluationPlan, evaluator Evaluator) error {
	started := time.Now()
	raw, err := evaluator.Evaluate(ctx, EvaluationInput{
		Workspace: result.Workspace,
		Checks:    append([]Check(nil), plan.Checks...),
	})
	if err != nil {
		return err
	}
	result.Checks, err = orderAndValidateResults(plan, raw)
	if err != nil {
		return err
	}
	for index, check := range plan.Checks {
		result.TotalWeight += check.Weight
		if result.Checks[index].Passed {
			result.EarnedWeight += check.Weight
		}
	}
	var buildErr error
	if result.BuildError != "" {
		buildErr = fmt.Errorf("%s", result.BuildError)
	}
	result.Status = benchmarkStatus(result.EarnedWeight, result.TotalWeight, buildErr)
	result.Elapsed += time.Since(started)
	return nil
}

func compareChecks(plan EvaluationPlan, baseline, assisted []CheckResult) []ComparisonCheck {
	checks := make([]ComparisonCheck, len(plan.Checks))
	for index, check := range plan.Checks {
		checks[index] = ComparisonCheck{
			ID: check.ID, Weight: check.Weight,
			BaselinePassed: baseline[index].Passed, AssistedPassed: assisted[index].Passed,
			BaselineEvidence: baseline[index].Evidence, AssistedEvidence: assisted[index].Evidence,
		}
	}
	return checks
}
