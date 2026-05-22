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

func (c *RoutedClient) SuggestTags(ctx context.Context, content string, maxTags int) ([]string, error) {
	if c == nil || c.Generation == nil {
		return nil, fmt.Errorf("generation client is not configured")
	}
	return c.Generation.SuggestTags(ctx, content, maxTags)
}

func (c *RoutedClient) SuggestTagsWithModel(ctx context.Context, model, content string, maxTags int) ([]string, error) {
	if c == nil || c.Generation == nil {
		return nil, fmt.Errorf("generation client is not configured")
	}
	return c.Generation.SuggestTagsWithModel(ctx, model, content, maxTags)
}
