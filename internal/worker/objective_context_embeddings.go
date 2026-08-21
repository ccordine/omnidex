package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/llm"
)

// embedContextQueries executes independent code-issued embedding operations
// concurrently and returns their vectors in canonical query order. Retrieval
// and deterministic result combination remain sequential and code-owned.
func embedContextQueries(
	ctx context.Context,
	client llm.EmbeddingClient,
	queries []string,
) ([][]float64, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context query embedding requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("context query embedding requires embedding authority")
	}
	if len(queries) == 0 {
		return [][]float64{}, nil
	}

	vectors := make([][]float64, len(queries))
	errorsByIndex := make([]error, len(queries))
	var group sync.WaitGroup
	group.Add(len(queries))
	for index, query := range queries {
		go func() {
			defer group.Done()
			vectors[index], errorsByIndex[index] = client.Embedding(ctx, query)
		}()
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for index, err := range errorsByIndex {
		if err != nil {
			return nil, fmt.Errorf("embed code-issued context query %d: %w", index, err)
		}
	}
	return vectors, nil
}
