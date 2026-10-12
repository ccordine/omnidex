package runtime

import (
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/config"
)

// No provider process or authentication is resolved until an agent job runs.
func agentProviders(cfg config.Config) func() (map[string]codingagent.Provider, codingagent.Verifier, error) {
	return func() (map[string]codingagent.Provider, codingagent.Verifier, error) {
		timeout, err := time.ParseDuration(cfg.RequestTimeout)
		if err != nil {
			return nil, nil, err
		}
		client, err := codex.New(cfg.CodexCommand, cfg.CodexDirectory, timeout)
		if err != nil {
			return nil, nil, err
		}
		return map[string]codingagent.Provider{"codex": client}, client, nil
	}
}
