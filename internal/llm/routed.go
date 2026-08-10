package llm

import (
	"context"
	"fmt"
)

type RoutedClient struct {
	Generation Client
	Embeddings Client
}

func NewRoutedClient(generation Client, embeddings Client) *RoutedClient {
	if embeddings == nil {
		embeddings = generation
	}
	return &RoutedClient{
		Generation: generation,
		Embeddings: embeddings,
	}
}

func (c *RoutedClient) Generate(ctx context.Context, model, prompt string) (string, error) {
	if c == nil || c.Generation == nil {
		return "", fmt.Errorf("generation client is not configured")
	}
	return c.Generation.Generate(ctx, model, prompt)
}

func (c *RoutedClient) PrepareContextModel(ctx context.Context, model, prompt string) (PreparedModel, error) {
	if c == nil || c.Generation == nil {
		return PreparedModel{}, fmt.Errorf("generation client is not configured")
	}
	return c.Generation.PrepareContextModel(ctx, model, prompt)
}

func (c *RoutedClient) GeneratePrepared(ctx context.Context, prepared PreparedModel) (string, error) {
	if c == nil || c.Generation == nil {
		return "", fmt.Errorf("generation client is not configured")
	}
	return c.Generation.GeneratePrepared(ctx, prepared)
}

func (c *RoutedClient) RequireExactPreparedContract() error {
	if c == nil || c.Generation == nil {
		return fmt.Errorf("generation client is not configured")
	}
	exact, ok := c.Generation.(ExactPreparedContractClient)
	if !ok {
		return fmt.Errorf("configured generation provider does not enforce the exact prepared contract")
	}
	return exact.RequireExactPreparedContract()
}

func (c *RoutedClient) ValidateExactPreparedContract(prepared PreparedModel) error {
	if err := c.RequireExactPreparedContract(); err != nil {
		return err
	}
	return c.Generation.(ExactPreparedContractClient).ValidateExactPreparedContract(prepared)
}

func (c *RoutedClient) AttestProviderIdentity(
	ctx context.Context,
	expected ProviderIdentityExpectation,
) (ProviderIdentityAttestation, error) {
	if c == nil || c.Generation == nil {
		return ProviderIdentityAttestation{}, fmt.Errorf("generation client is not configured")
	}
	attestor, ok := c.Generation.(ProviderIdentityAttestor)
	if !ok {
		return ProviderIdentityAttestation{}, fmt.Errorf(
			"configured generation provider cannot attest its live identity",
		)
	}
	return attestor.AttestProviderIdentity(ctx, expected)
}

func (c *RoutedClient) GeneratePreparedAdvisory(ctx context.Context, prepared PreparedModel) (AdvisoryResponse, error) {
	if c == nil || c.Generation == nil {
		return AdvisoryResponse{}, fmt.Errorf("generation client is not configured")
	}
	client, okay := c.Generation.(PreparedAdvisoryClient)
	if !okay {
		return AdvisoryResponse{}, fmt.Errorf("configured generation provider does not support native advisory thinking")
	}
	return client.GeneratePreparedAdvisory(ctx, prepared)
}

func (c *RoutedClient) CleanupPreparedModel(prepared PreparedModel) {
	if c == nil || c.Generation == nil {
		return
	}
	c.Generation.CleanupPreparedModel(prepared)
}

func (c *RoutedClient) Embedding(ctx context.Context, content string) ([]float64, error) {
	if c == nil || c.Embeddings == nil {
		return nil, fmt.Errorf("embedding client is not configured")
	}
	return c.Embeddings.Embedding(ctx, content)
}
