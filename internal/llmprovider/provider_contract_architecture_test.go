package llmprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBroadProductionProviderSurfacesAreAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{
		"internal/llm/routed.go",
		"internal/llm/advisory.go",
		"internal/ollama/advisory.go",
		"internal/ollama/chat.go",
		"internal/ollama/chat_stream.go",
		"internal/ollama/prepared_stream.go",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("broad production provider surface remains: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", name, err)
		}
	}
}

func TestProviderPackagesExposeOnlyNarrowProductionContracts(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/llm/llm.go": {
			"type Client interface", "type PreparedStreamingClient interface",
			"func DerivePreparedModelPromptHint", "func ExtractPromptBlock", "func TruncatePromptHint",
		},
		"internal/ollama/prepared.go": {
			"func (c *Client) Generate(", "func (c *Client) PrepareContextModel(",
			"func (c *Client) GeneratePrepared(", "func (c *Client) CleanupPreparedModel(",
		},
		"internal/openai/client.go": {
			"func (c *Client) Generate(", "func (c *Client) PrepareContextModel(",
			"func (c *Client) GeneratePrepared(", "func (c *Client) CleanupPreparedModel(",
		},
		"internal/googleai/client.go": {
			"func (c *Client) Generate(", "func (c *Client) PrepareContextModel(",
			"func (c *Client) GeneratePrepared(", "func (c *Client) CleanupPreparedModel(",
		},
		"internal/huggingface/client.go": {
			"func (c *Client) Generate(", "func (c *Client) PrepareContextModel(",
			"func (c *Client) GeneratePrepared(", "func (c *Client) CleanupPreparedModel(",
		},
		"internal/worker/engine.go": {
			"type embeddingClient interface", "type exactStationClient interface", ".(exactStationClient)",
		},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("broad production provider token %q remains in %s", token, name)
			}
		}
	}
}
