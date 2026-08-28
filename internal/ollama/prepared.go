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
	observed, err := c.ObserveProviderIdentity(ctx, llm.ProviderIdentityObservationRequest{
		Expectation:     *prepared.ProviderIdentityExpectation,
		ChallengeSHA256: prepared.ProviderObservationChallenge,
	})
	if err != nil {
		return llm.PreparedGeneration{
			Schema:                     llm.PreparedGenerationSchemaV1,
			Protocol:                   prepared.Protocol,
			ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
			ProviderIdentityEvidence:   observed.Evidence,
		}, fmt.Errorf("observe exact cognition provider identity: %w", err)
	}
	result, generationErr := c.generatePreparedRaw(ctx, prepared, observed)
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

func (c *Client) ValidateExactPreparedProvider(expected llm.ProviderIdentityExpectation) error {
	return llm.ValidateExactPreparedProviderExpectation(expected)
}

func (c *Client) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if err := c.RequireExactPreparedContract(); err != nil {
		return err
	}
	_, err := llm.ExactPreparedRequestBytes(prepared)
	return err
}
