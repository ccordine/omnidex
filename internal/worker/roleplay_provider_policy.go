package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

func (s *Service) exactStationContextTokens(
	ctx context.Context,
) (int, error) {
	if ctx == nil || s == nil {
		return 0, fmt.Errorf("exact station context resolution requires context and worker")
	}
	if err := llm.ValidateInferenceContextTokens(s.inferenceContextTokens); err != nil {
		return 0, err
	}
	return s.inferenceContextTokens, nil
}
