package llm

import "fmt"

const (
	MinInferenceContextTokens     = 8192
	DefaultInferenceContextTokens = 8192
	MaxInferenceContextTokens     = 1048576
)

func ValidateInferenceContextTokens(value int) error {
	if value <= 0 {
		return fmt.Errorf("inference context tokens must be positive, received %d", value)
	}
	return nil
}

func ValidateRoleplayCompletionContextTokens(value int) error {
	return ValidateInferenceContextTokens(value)
}

func ValidateExactPreparedContextTokens(value int) error {
	return ValidateInferenceContextTokens(value)
}
