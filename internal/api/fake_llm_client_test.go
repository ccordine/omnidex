package api

import "context"

type fakeLLMClient struct {
	embedding    []float64
	embeddingErr error
}

func (client *fakeLLMClient) Embedding(
	context.Context,
	string,
) ([]float64, error) {
	return append([]float64(nil), client.embedding...), client.embeddingErr
}
