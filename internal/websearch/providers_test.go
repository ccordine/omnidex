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
		{provider: ProviderBrave, contains: "search.brave.com/search?q=rust+async&source=web"},
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

func TestBraveProviderParsesDirectResultLinksAndRejectsProviderLinks(t *testing.T) {
	definition, ok := providerDefinitionFor(ProviderBrave)
	if !ok {
		t.Fatal("Brave provider is not registered")
	}
	searchURL := definition.searchURL("current Go release")
	body := `<a href="https://search.brave.com/help" class="svelte-internal l1">Help</a>` +
		`<a href="https://go.dev/doc/devel/release" target="_self" class="svelte-result l1"><span>Go Release History</span></a>` +
		`<a href="https://go.dev/dl/" target="_self" class="svelte-result l1">Downloads</a>`
	results, err := parseProviderCandidates(definition, searchURL, body, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].url != "https://go.dev/doc/devel/release" ||
		results[0].title != "Go Release History" || results[1].url != "https://go.dev/dl/" {
		t.Fatalf("results=%#v", results)
	}
}
