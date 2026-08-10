package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// ValidateRuntimeBudget binds an episode station budget to one frozen brain
// before any durable episode or inference evidence is created.
func ValidateRuntimeBudget(brain BrainRef, budget cognition.RuntimeBudget) error {
	if err := brain.Validate(); err != nil {
		return err
	}
	if err := budget.Validate(); err != nil {
		return fmt.Errorf("%w: runtime budget: %v", ErrInvalidConfig, err)
	}
	if budget.MaxInputBytes > brain.ContextCeilingBytes ||
		budget.MaxOutputTokens > brain.Sampling.MaxOutputTokens ||
		budget.MaxInputBytes+brain.Sampling.InputSpecialTokenReserve != budget.MaxInputTokens ||
		budget.MaxInputTokens+budget.MaxOutputTokens > brain.NativeContextLimit {
		return fmt.Errorf("%w: runtime budget exceeds the frozen brain ceilings", ErrInvalidBrain)
	}
	return nil
}
