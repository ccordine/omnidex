package llm

import "fmt"

const (
	MinInferenceContextTokens      = 8192
	DefaultInferenceContextTokens  = 8192
	MaxInferenceContextTokens      = 1048576
	inferenceContextOverheadTokens = 512
	defaultOutputReservationTokens = 2048
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

// ValidateInferenceBudget conservatively treats each input byte as at most one
// token. This intentionally reserves more context than most tokenizers need so
// a request fails explicitly instead of allowing the provider to truncate it.
func ValidateInferenceBudget(contextTokens, maxOutputTokens int, inputs ...string) error {
	available, reservedOutput, err := InferenceInputByteBudget(contextTokens, maxOutputTokens)
	if err != nil {
		return err
	}
	inputUpperBound := 0
	for _, input := range inputs {
		inputUpperBound += len([]byte(input))
	}
	required := inputUpperBound + reservedOutput + inferenceContextOverheadTokens
	if inputUpperBound > available {
		return fmt.Errorf(
			"inference context exhausted before request: input_token_upper_bound=%d reserved_output_tokens=%d overhead_tokens=%d required_tokens=%d configured_context_tokens=%d",
			inputUpperBound,
			reservedOutput,
			inferenceContextOverheadTokens,
			required,
			contextTokens,
		)
	}
	return nil
}

// InferenceInputByteBudget returns the conservative number of model-visible
// input bytes available after the exact output and provider-overhead reserves.
func InferenceInputByteBudget(contextTokens, maxOutputTokens int) (int, int, error) {
	if err := ValidateInferenceContextTokens(contextTokens); err != nil {
		return 0, 0, err
	}
	if maxOutputTokens < 0 {
		return 0, 0, fmt.Errorf("maximum output tokens cannot be negative, received %d", maxOutputTokens)
	}
	reservedOutput := maxOutputTokens
	if reservedOutput == 0 {
		reservedOutput = defaultOutputReservationTokens
	}
	available := contextTokens - reservedOutput - inferenceContextOverheadTokens
	if available < 0 {
		return 0, 0, fmt.Errorf(
			"inference context has no input capacity: reserved_output_tokens=%d overhead_tokens=%d configured_context_tokens=%d",
			reservedOutput, inferenceContextOverheadTokens, contextTokens,
		)
	}
	return available, reservedOutput, nil
}
