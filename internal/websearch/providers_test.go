package websearch

import (
	"strings"
	"testing"
)

func TestRegisteredProviderStrategiesRemainExplicitAndDeterministic(t *testing.T) {
	tests := []struct {
		provider ProviderID
		contains string
	}{
		{provider: ProviderDuckDuckGo, contains: "duckduckgo.com/html/?q=rust+async"},
		{provider: ProviderGoogle, contains: "google.com/search?q=rust+async"},
		{provider: ProviderReddit, contains: "google.com/search?q=site%3Areddit.com+rust+async"},
		{provider: ProviderYahoo, contains: "search.yahoo.com/search?p=rust+async"},
	}
	for _, test := range tests {
		t.Run(string(test.provider), func(t *testing.T) {
			definition, ok := providerDefinitionFor(test.provider)
			if !ok {
				t.Fatalf("provider %q is not registered", test.provider)
			}
			first := definition.searchURL("rust async")
			second := definition.searchURL("rust async")
			if first != second || !strings.Contains(first, test.contains) {
				t.Fatalf("search URL=%q second=%q want substring %q", first, second, test.contains)
			}
		})
	}
}
