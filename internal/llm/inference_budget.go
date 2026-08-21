package llm

import "fmt"

const (
	MinRoleplayRawContextTokens   = 4096
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

// ValidateRoleplayRawContextTokens is the narrower transport floor available
// only to code-classified fictional prose stations. Ordinary semantic and
// coding stations retain ValidateInferenceContextTokens's stricter floor.
func ValidateRoleplayRawContextTokens(value int) error {
	if value < MinRoleplayRawContextTokens || value > MaxInferenceContextTokens {
		return fmt.Errorf(
			"roleplay raw context tokens must be between %d and %d, received %d",
			MinRoleplayRawContextTokens,
			MaxInferenceContextTokens,
			value,
		)
	}
	return nil
}

// ValidateExactPreparedContextTokens validates the transport-wide range. A
// caller that owns station policy must separately enforce the ordinary or raw
// roleplay floor before preparing a request.
func ValidateExactPreparedContextTokens(value int) error {
	return ValidateRoleplayRawContextTokens(value)
}
