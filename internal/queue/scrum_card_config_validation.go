package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func canonicalAgentConfig(raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := agentconfig.FromJSON(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(cfg.ToMap())
	if err != nil {
		return nil, fmt.Errorf("encode agent configuration: %w", err)
	}
	return encoded, nil
}

func canonicalModelConfig(raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := modelconfig.FromJSON(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(cfg.ToMap())
	if err != nil {
		return nil, fmt.Errorf("encode model configuration: %w", err)
	}
	return encoded, nil
}
