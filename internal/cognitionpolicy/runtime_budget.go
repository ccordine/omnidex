package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
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
		budget.MaxInputTokens+budget.MaxOutputTokens > brain.NativeContextLimit {
		return fmt.Errorf("%w: runtime budget exceeds the frozen brain ceilings", ErrInvalidBrain)
	}
	available, _, err := llm.InferenceInputByteBudget(
		brain.NativeContextLimit, budget.MaxOutputTokens,
	)
	if err != nil || budget.MaxInputBytes+len(llm.MinimalGeneratePrompt) > available {
		return fmt.Errorf("%w: runtime byte budget cannot fit the prepared inference boundary", ErrInvalidBrain)
	}
	return nil
}
