package ollama

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

func (c *Client) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if err := c.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedGeneration{}, err
	}
	result, generationErr := c.generatePreparedRaw(ctx, prepared)
	if generationErr != nil {
		return result, generationErr
	}
	return result, llm.ValidateExactPreparedGenerationForRequest(prepared, result)
}

func (c *Client) RequireExactPreparedContract() error {
	if c == nil || c.contextTokens <= 0 {
		return fmt.Errorf("ollama exact prepared contract requires an initialized client")
	}
	return nil
}

func (c *Client) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if err := c.RequireExactPreparedContract(); err != nil {
		return err
	}
	_, err := llm.ExactPreparedRequestBytes(prepared)
	return err
}
