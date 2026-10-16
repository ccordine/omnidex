package codingagent

import (
	"encoding/json"
	"fmt"
)

func ConfigurationFromMetadata(raw json.RawMessage) (Configuration, error) {
	var root struct {
		Coding json.RawMessage `json:"coding_execution"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return Configuration{}, err
	}
	if len(root.Coding) == 0 {
		return Configuration{}, nil
	}
	var configuration Configuration
	if err := decodeContract(root.Coding, &configuration); err != nil {
		return Configuration{}, err
	}
	if err := configuration.Validate(); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

func (config Configuration) ForMode(mode Mode) (Configuration, error) {
	if mode == "" {
		return Configuration{}, nil
	}
	if mode != Agents {
		return Configuration{}, fmt.Errorf("unknown coding mode %q", mode)
	}
	config.Mode = Agents
	return config, config.Validate()
}
