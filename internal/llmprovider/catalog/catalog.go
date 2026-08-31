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

var definitionsByName = mustIndexDefinitions(providerDefinitions)

func Lookup(value string) (Definition, bool) {
	definition, ok := definitionsByName[value]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(definition), true
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
	index := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" {
			panic("LLM provider catalog contains an empty provider ID")
		}
		if definition.ID != strings.ToLower(strings.TrimSpace(definition.ID)) {
			panic(fmt.Sprintf("LLM provider ID %q is not canonical", definition.ID))
		}
		if strings.TrimSpace(definition.DisplayName) == "" {
			panic(fmt.Sprintf("LLM provider %q has no display name", definition.ID))
		}
		if definition.EnvironmentPrefix == "" {
			panic(fmt.Sprintf("LLM provider %q has no environment prefix", definition.ID))
		}
		if definition.EnvironmentPrefix != strings.Trim(strings.ToUpper(strings.TrimSpace(definition.EnvironmentPrefix)), "_") {
			panic(fmt.Sprintf("LLM provider %q has non-canonical environment prefix %q", definition.ID, definition.EnvironmentPrefix))
		}
		if existing, duplicate := index[definition.ID]; duplicate {
			panic(fmt.Sprintf("LLM provider ID %q is shared by %q and %q", definition.ID, existing.ID, definition.ID))
		}
		index[definition.ID] = definition
	}
	return index
}

func cloneDefinition(definition Definition) Definition {
	return definition
}
