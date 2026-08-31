package worker

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/llm"
)

func (s *Service) exactStationContextTokens(
	ctx context.Context,
) (int, error) {
	if ctx == nil || s == nil {
		return 0, fmt.Errorf("exact station context resolution requires context and worker")
	}
	value, err := strconv.Atoi(s.inferenceContextTokens)
	if err != nil {
		return 0, fmt.Errorf("INFERENCE_CONTEXT_TOKENS must be an integer: %w", err)
	}
	if err := llm.ValidateInferenceContextTokens(value); err != nil {
		return 0, err
	}
	return value, nil
}
