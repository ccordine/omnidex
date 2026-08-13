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
	Aliases                       []string
	Protocol                      Protocol
	EnvironmentPrefixes           []string
	APIKeyEnvironmentKeys         []string
	BaseURLEnvironmentKeys        []string
	EmbeddingModelEnvironmentKeys []string
	DefaultBaseURL                string
	DefaultEmbeddingModel         string
	SupportsExactPreparedStations bool
	SupportsEmbeddings            bool
	RequiresBaseURL               bool
	ChineseService                bool
}

func (d Definition) EnvironmentKeys(suffix string) []string {
	suffix = strings.ToUpper(strings.Trim(strings.TrimSpace(suffix), "_"))
	if suffix == "" {
		return nil
	}
	switch suffix {
	case "EMBEDDING_MODEL":
		if len(d.EmbeddingModelEnvironmentKeys) > 0 {
			return append([]string(nil), d.EmbeddingModelEnvironmentKeys...)
		}
	}
	keys := make([]string, 0, len(d.EnvironmentPrefixes))
	for _, prefix := range d.EnvironmentPrefixes {
		prefix = strings.Trim(strings.ToUpper(strings.TrimSpace(prefix)), "_")
		if prefix != "" {
			keys = append(keys, prefix+"_"+suffix)
		}
	}
	return keys
}

var definitionsByName = mustIndexDefinitions(providerDefinitions)

func Lookup(value string) (Definition, bool) {
	key := strings.ToLower(strings.TrimSpace(value))
	definition, ok := definitionsByName[key]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(definition), true
}

func CanonicalID(value string) (string, error) {
	definition, ok := Lookup(value)
	if !ok {
		return "", fmt.Errorf("unsupported LLM provider %q", strings.TrimSpace(value))
	}
	return definition.ID, nil
}

func Definitions() []Definition {
	out := make([]Definition, 0, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		out = append(out, cloneDefinition(definition))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ProductionDefinitions() []Definition {
	definitions := Definitions()
	out := make([]Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.SupportsExactPreparedStations || definition.SupportsEmbeddings {
			out = append(out, definition)
		}
	}
	return out
}

func ExactStationProviderIDs() []string {
	return providerIDs(func(definition Definition) bool { return definition.SupportsExactPreparedStations })
}

func EmbeddingProviderIDs() []string {
	return providerIDs(func(definition Definition) bool { return definition.SupportsEmbeddings })
}

func ChineseProviderIDs() []string {
	return providerIDs(func(definition Definition) bool { return definition.ChineseService })
}

func providerIDs(include func(Definition) bool) []string {
	ids := make([]string, 0, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		if include(definition) {
			ids = append(ids, definition.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func mustIndexDefinitions(definitions []Definition) map[string]Definition {
	index := make(map[string]Definition, len(definitions)*2)
	for _, definition := range definitions {
		if strings.TrimSpace(definition.ID) == "" {
			panic("LLM provider catalog contains an empty provider ID")
		}
		if strings.TrimSpace(definition.DisplayName) == "" {
			panic(fmt.Sprintf("LLM provider %q has no display name", definition.ID))
		}
		for _, name := range append([]string{definition.ID}, definition.Aliases...) {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" {
				panic(fmt.Sprintf("LLM provider %q contains an empty alias", definition.ID))
			}
			if existing, duplicate := index[key]; duplicate {
				panic(fmt.Sprintf("LLM provider alias %q is shared by %q and %q", key, existing.ID, definition.ID))
			}
			index[key] = definition
		}
	}
	return index
}

func cloneDefinition(definition Definition) Definition {
	definition.Aliases = append([]string(nil), definition.Aliases...)
	definition.EnvironmentPrefixes = append([]string(nil), definition.EnvironmentPrefixes...)
	definition.APIKeyEnvironmentKeys = append([]string(nil), definition.APIKeyEnvironmentKeys...)
	definition.BaseURLEnvironmentKeys = append([]string(nil), definition.BaseURLEnvironmentKeys...)
	definition.EmbeddingModelEnvironmentKeys = append([]string(nil), definition.EmbeddingModelEnvironmentKeys...)
	return definition
}
