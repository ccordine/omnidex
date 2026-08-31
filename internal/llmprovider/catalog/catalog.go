package catalog

import (
	"fmt"
	"sort"
	"strings"
)

type Protocol string

const (
	ProtocolOllama           Protocol = "ollama"
	ProtocolOpenAICompatible Protocol = "openai-compatible"
	ProtocolAzure            Protocol = "azure"
	ProtocolGoogle           Protocol = "google"
	ProtocolAnthropic        Protocol = "anthropic"
	ProtocolHuggingFace      Protocol = "huggingface"
)

type Definition struct {
	ID                            string
	DisplayName                   string
	Protocol                      Protocol
	EnvironmentPrefix             string
	DefaultBaseURL                string
	DefaultEmbeddingModel         string
	SupportsExactPreparedStations bool
	SupportsEmbeddings            bool
	RequiresBaseURL               bool
	ChineseService                bool
}

func (d Definition) EnvironmentKey(suffix string) string {
	suffix = strings.ToUpper(strings.Trim(strings.TrimSpace(suffix), "_"))
	if suffix == "" {
		return ""
	}
	prefix := strings.Trim(strings.ToUpper(strings.TrimSpace(d.EnvironmentPrefix)), "_")
	if prefix == "" {
		return ""
	}
	return prefix + "_" + suffix
}

// Resolve validates only the provider selected by the current operation. An
// unused catalog entry cannot prevent the process from starting or another
// provider from serving work.
func Resolve(value string) (Definition, error) {
	var selected *Definition
	for index := range providerDefinitions {
		definition := providerDefinitions[index]
		if definition.ID != value {
			continue
		}
		if selected != nil {
			return Definition{}, fmt.Errorf("LLM provider ID %q is duplicated", value)
		}
		selected = &definition
	}
	if selected == nil {
		return Definition{}, fmt.Errorf("unsupported LLM provider %q", value)
	}
	if err := validateDefinition(*selected); err != nil {
		return Definition{}, err
	}
	return cloneDefinition(*selected), nil
}

func ProductionDefinitions() []Definition {
	out := make([]Definition, 0, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		if definition.SupportsExactPreparedStations || definition.SupportsEmbeddings {
			out = append(out, cloneDefinition(definition))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func validateDefinition(definition Definition) error {
	if definition.ID == "" {
		return fmt.Errorf("LLM provider catalog contains an empty provider ID")
	}
	if definition.ID != strings.ToLower(strings.TrimSpace(definition.ID)) {
		return fmt.Errorf("LLM provider ID %q is not canonical", definition.ID)
	}
	if strings.TrimSpace(definition.DisplayName) == "" {
		return fmt.Errorf("LLM provider %q has no display name", definition.ID)
	}
	if definition.EnvironmentPrefix == "" {
		return fmt.Errorf("LLM provider %q has no environment prefix", definition.ID)
	}
	if definition.EnvironmentPrefix != strings.Trim(strings.ToUpper(strings.TrimSpace(definition.EnvironmentPrefix)), "_") {
		return fmt.Errorf("LLM provider %q has non-canonical environment prefix %q", definition.ID, definition.EnvironmentPrefix)
	}
	return nil
}

func cloneDefinition(definition Definition) Definition {
	return definition
}
