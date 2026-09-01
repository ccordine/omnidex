package ollama

import (
	"context"

	"github.com/gryph/omnidex/internal/llm"
)

func (c *Client) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	return c.generatePreparedRaw(ctx, prepared)
}
