package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

func (c *Client) AddMemory(
	ctx context.Context,
	input model.MemoryInput,
) (model.MemoryChunk, error) {
	chunks, err := c.AddMemories(ctx, []model.MemoryInput{input})
	if err != nil {
		return model.MemoryChunk{}, err
	}
	return chunks[0], nil
}

func (c *Client) AddMemories(
	ctx context.Context,
	inputs []model.MemoryInput,
) ([]model.MemoryChunk, error) {
	if len(inputs) == 0 || len(inputs) > 512 {
		return nil, fmt.Errorf("memory batch must contain 1..512 items")
	}
	for index, input := range inputs {
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("memory batch item %d: %w", index+1, err)
		}
	}
	var resp struct {
		Memories []model.MemoryChunk `json:"memories"`
		Error    string              `json:"error"`
	}
	if err := c.doJSON(ctx, "POST", "/v1/memory/batch", map[string]any{
		"memories": inputs,
	}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	if len(resp.Memories) != len(inputs) {
		return nil, fmt.Errorf(
			"memory batch response count=%d want=%d", len(resp.Memories), len(inputs),
		)
	}
	for index, chunk := range resp.Memories {
		input := inputs[index]
		if chunk.ID < 1 || chunk.Source != input.Source || chunk.Kind != input.Kind ||
			chunk.Content != input.Content || chunk.Scope != input.Scope {
			return nil, fmt.Errorf("memory batch response item %d differs from its exact input", index+1)
		}
	}
	return resp.Memories, nil
}
