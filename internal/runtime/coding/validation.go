package coding

import "context"

type passValidator struct{}

func (passValidator) ValidatePlan(context.Context, CodingPlan) (ValidationResult, error) {
	return ValidationResult{Status: StatusPass, Evidence: []string{"plan has top-level coding tasks"}}, nil
}

func (passValidator) ValidateArchitect(context.Context, CodingPlannerTask, ArchitectQueue) (ValidationResult, error) {
	return ValidationResult{Status: StatusPass, Evidence: []string{"architect emitted concrete queues"}}, nil
}

func (passValidator) ValidateChange(context.Context, ChangeStep) (ValidationResult, error) {
	return ValidationResult{Status: StatusPass, Evidence: []string{"change validated"}}, nil
}

func (passValidator) ValidateArchitectTask(context.Context, CodingPlannerTask, ArchitectQueue) (ValidationResult, error) {
	return ValidationResult{Status: StatusPass, Evidence: []string{"architect task validated"}}, nil
}
