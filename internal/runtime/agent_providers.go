package runtime

import (
	"strconv"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/ollama"
)

// No provider process or authentication is resolved until an agent job runs.
func agentProviders(cfg config.Config) func() (map[string]codingagent.Provider, error) {
	return func() (map[string]codingagent.Provider, error) {
		client, err := codex.New(cfg.CodexCommand, cfg.CodexDirectory)
		if err != nil {
			return nil, err
		}
		providers := map[string]codingagent.Provider{"codex": client}
		if cfg.OllamaBaseURL != "" {
			contextTokens, err := strconv.Atoi(cfg.CodingParserContextTokens)
			if err != nil {
				return nil, err
			}
			parser, err := ollama.NewParser(cfg.OllamaBaseURL, contextTokens)
			if err != nil {
				return nil, err
			}
			providers["ollama"] = parser
		}
		return providers, nil
	}
}
