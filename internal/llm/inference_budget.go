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
