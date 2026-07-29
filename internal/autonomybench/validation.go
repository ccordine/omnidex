package autonomybench

import (
	"fmt"
	"strings"
)

func validateRequest(request RequestCase) error {
	if strings.TrimSpace(request.ID) == "" {
		return fmt.Errorf("autonomy benchmark request id is required")
	}
	if strings.TrimSpace(request.UserRequest) == "" {
		return fmt.Errorf("autonomy benchmark user request is required")
	}
	if request.UserRequest != strings.TrimSpace(request.UserRequest) {
		return fmt.Errorf("autonomy benchmark user request must not contain surrounding whitespace")
	}
	if strings.TrimSpace(request.Workspace) == "" {
		return fmt.Errorf("autonomy benchmark workspace is required")
	}
	return nil
}

func validateEvaluationPlan(requestID string, plan EvaluationPlan) error {
	if plan.RequestID != requestID {
		return fmt.Errorf("evaluation request id %q does not match build request %q", plan.RequestID, requestID)
	}
	if len(plan.Checks) == 0 {
		return fmt.Errorf("evaluation plan %q requires at least one check", requestID)
	}
	seen := make(map[string]struct{}, len(plan.Checks))
	for index, check := range plan.Checks {
		if strings.TrimSpace(check.ID) == "" {
			return fmt.Errorf("evaluation check %d requires an id", index)
		}
		if check.Weight < 1 {
			return fmt.Errorf("evaluation check %q requires a positive weight", check.ID)
		}
		if _, duplicate := seen[check.ID]; duplicate {
			return fmt.Errorf("evaluation plan repeats check %q", check.ID)
		}
		seen[check.ID] = struct{}{}
	}
	return nil
}

func orderAndValidateResults(plan EvaluationPlan, results []CheckResult) ([]CheckResult, error) {
	byID := make(map[string]CheckResult, len(results))
	for _, result := range results {
		if _, duplicate := byID[result.ID]; duplicate {
			return nil, fmt.Errorf("evaluator repeated result %q", result.ID)
		}
		if strings.TrimSpace(result.Evidence) == "" {
			return nil, fmt.Errorf("evaluation result %q requires evidence", result.ID)
		}
		byID[result.ID] = result
	}
	ordered := make([]CheckResult, 0, len(plan.Checks))
	for _, check := range plan.Checks {
		result, exists := byID[check.ID]
		if !exists {
			return nil, fmt.Errorf("evaluator omitted result %q", check.ID)
		}
		ordered = append(ordered, result)
		delete(byID, check.ID)
	}
	for id := range byID {
		return nil, fmt.Errorf("evaluator returned unknown result %q", id)
	}
	return ordered, nil
}
