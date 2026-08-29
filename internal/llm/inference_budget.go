package llm

import "fmt"

const (
	MinInferenceContextTokens     = 8192
	DefaultInferenceContextTokens = 8192
	MaxInferenceContextTokens     = 1048576
)

func ValidateInferenceContextTokens(value int) error {
	if value < MinInferenceContextTokens || value > MaxInferenceContextTokens {
		return fmt.Errorf(
			"inference context tokens must be between %d and %d, received %d",
			MinInferenceContextTokens,
			MaxInferenceContextTokens,
			value,
		)
	}
	return nil
}

// ValidateRoleplayCompletionContextTokens preserves roleplay-specific context
// negotiation without weakening the exact transport's authoritative range.
func ValidateRoleplayCompletionContextTokens(value int) error {
	if value < MinInferenceContextTokens || value > MaxInferenceContextTokens {
		return fmt.Errorf(
			"roleplay completion context tokens must be between %d and %d, received %d",
			MinInferenceContextTokens,
			MaxInferenceContextTokens,
			value,
		)
	}
	return nil
}

// ValidateExactPreparedContextTokens validates the sole transport-wide range.
func ValidateExactPreparedContextTokens(value int) error {
	return ValidateInferenceContextTokens(value)
}
