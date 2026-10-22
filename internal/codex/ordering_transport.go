package codex

import (
	"context"

	"github.com/gryph/omnidex/internal/llm"
)

// OrderingClient reuses the instruction-isolated lightweight semantic transport.
// Its lifetime belongs to one ordering operation, independently of native work.
func (client *Client) OrderingClient(ctx context.Context) (llm.ExactStationClient, func(), error) {
	parser, err := NewParser(ctx, client.command, client.directory)
	if err != nil {
		return nil, nil, err
	}
	return parser, func() { _ = parser.Close() }, nil
}
