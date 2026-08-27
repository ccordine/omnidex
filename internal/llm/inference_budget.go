package llm

import "fmt"

const (
	MinRoleplayCompletionContextTokens = 4096
	MinInferenceContextTokens          = 8192
	DefaultInferenceContextTokens      = 8192
	MaxInferenceContextTokens          = 1048576
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

// ValidateRoleplayCompletionContextTokens is the narrower transport floor
// available only to code-classified roleplay completion profiles. Ordinary
// semantic and coding stations retain ValidateInferenceContextTokens's floor.
func ValidateRoleplayCompletionContextTokens(value int) error {
	if value < MinRoleplayCompletionContextTokens || value > MaxInferenceContextTokens {
		return fmt.Errorf(
			"roleplay completion context tokens must be between %d and %d, received %d",
			MinRoleplayCompletionContextTokens,
			MaxInferenceContextTokens,
			value,
		)
	}
	return nil
}

// ValidateExactPreparedContextTokens validates the transport-wide range. A
// caller that owns station policy must separately enforce the ordinary or
// roleplay-completion floor before preparing a request.
func ValidateExactPreparedContextTokens(value int) error {
	return ValidateRoleplayCompletionContextTokens(value)
}
