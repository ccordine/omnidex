package catalog

import (
	"reflect"
	"testing"
)

func TestChineseProviderCatalogCoversNamedCompatibleServices(t *testing.T) {
	want := []string{
		"antling",
		"baichuan",
		"deepseek",
		"doubao",
		"hunyuan",
		"longcat",
		"mimo",
		"minimax",
		"modelarts",
		"modelscope",
		"moonshot",
		"qianfan",
		"qwen",
		"siliconflow",
		"spark",
		"stepfun",
		"tokenhub",
		"yi",
		"zai",
		"zhipu",
	}

	if got := ChineseProviderIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ChineseProviderIDs()=%v want %v", got, want)
	}
	for _, providerID := range want {
		definition, ok := Lookup(providerID)
		if !ok {
			t.Fatalf("Lookup(%q) missing", providerID)
		}
		if definition.SupportsExactPreparedStations {
			t.Errorf("provider %q falsely advertises exact station inference", providerID)
		}
		if definition.Protocol != ProtocolOpenAICompatible {
			t.Errorf("provider %q protocol=%q want %q", providerID, definition.Protocol, ProtocolOpenAICompatible)
		}
	}
}

func TestLookupCanonicalizesProviderAliasesWithoutGuessing(t *testing.T) {
	tests := map[string]string{
		"local":             "ollama",
		"chat-gpt":          "openai",
		"windows-ai":        "azure",
		"grock":             "xai",
		"gemini":            "google",
		"claude":            "anthropic",
		"hf":                "huggingface",
		"deep-seek":         "deepseek",
		"dashscope":         "qwen",
		"kimi":              "moonshot",
		"glm":               "zhipu",
		"z-ai":              "zai",
		"baidu":             "qianfan",
		"ernie":             "qianfan",
		"tencent":           "hunyuan",
		"volcengine":        "doubao",
		"ark":               "doubao",
		"iflytek":           "spark",
		"xiaomi":            "mimo",
		"meituan":           "longcat",
		"inclusion-ai":      "antling",
		"tencent-tokenhub":  "tokenhub",
		"huawei-maas":       "modelarts",
		"custom-openai":     "compatible",
		"openai-compatible": "compatible",
	}

	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			definition, ok := Lookup(alias)
			if !ok {
				t.Fatalf("Lookup(%q) missing", alias)
			}
			if definition.ID != want {
				t.Fatalf("Lookup(%q).ID=%q want %q", alias, definition.ID, want)
			}
		})
	}

	if definition, ok := Lookup("definitely-not-a-provider"); ok {
		t.Fatalf("unknown provider resolved to %#v", definition)
	}
}

func TestProviderCapabilitiesAndEnvironmentKeysAreAuthoritative(t *testing.T) {
	ollama, ok := Lookup("ollama")
	if !ok {
		t.Fatal("ollama missing")
	}
	if ollama.DefaultBaseURL != "" || !reflect.DeepEqual(ollama.BaseURLEnvironmentKeys, []string{"OLLAMA_BASE_URL"}) {
		t.Fatalf("ollama endpoint authority=%#v", ollama)
	}

	qwen, ok := Lookup("qwen")
	if !ok {
		t.Fatal("qwen missing")
	}
	if !qwen.SupportsEmbeddings {
		t.Fatal("qwen must support embeddings")
	}
	if got := qwen.EnvironmentKeys("MODEL_REASONING"); !reflect.DeepEqual(got, []string{"QWEN_MODEL_REASONING", "DASHSCOPE_MODEL_REASONING"}) {
		t.Fatalf("qwen reasoning keys=%v", got)
	}
	if got := qwen.APIKeyEnvironmentKeys; !reflect.DeepEqual(got, []string{"QWEN_API_KEY", "DASHSCOPE_API_KEY"}) {
		t.Fatalf("qwen API key environment keys=%v", got)
	}

	deepseek, ok := Lookup("deepseek")
	if !ok {
		t.Fatal("deepseek missing")
	}
	if deepseek.SupportsEmbeddings {
		t.Fatal("deepseek must not claim a native embeddings endpoint")
	}

	compatible, ok := Lookup("compatible")
	if !ok {
		t.Fatal("compatible missing")
	}
	if compatible.DefaultBaseURL != "" {
		t.Fatalf("custom compatible provider defaulted to %q", compatible.DefaultBaseURL)
	}
	if !compatible.RequiresBaseURL {
		t.Fatal("custom compatible provider must require an explicit base URL")
	}
	if compatible.ChineseService {
		t.Fatal("a generic compatible endpoint must not claim a geographic service identity")
	}
}

func TestSupportedProviderListsAreSortedAndCapabilityFiltered(t *testing.T) {
	embeddings := EmbeddingProviderIDs()
	if contains(embeddings, "deepseek") {
		t.Fatalf("generation-only provider appeared in embedding providers: %v", embeddings)
	}
	for _, providerID := range []string{"baichuan", "qwen", "zhipu", "qianfan", "hunyuan", "siliconflow", "compatible"} {
		if !contains(embeddings, providerID) {
			t.Errorf("embedding providers missing %q: %v", providerID, embeddings)
		}
	}
}

func TestProductionDefinitionsExcludeUnconsumedGenerationOnlyTransports(t *testing.T) {
	production := map[string]bool{}
	for _, definition := range ProductionDefinitions() {
		production[definition.ID] = true
	}
	for _, providerID := range []string{"anthropic", "deepseek", "xai", "moonshot"} {
		if production[providerID] {
			t.Errorf("unconsumed provider %q remains in the production catalog", providerID)
		}
	}
	for _, providerID := range []string{"ollama", "openai", "google", "qwen"} {
		if !production[providerID] {
			t.Errorf("consumed exact/embedding provider %q missing from production catalog", providerID)
		}
	}
}

func TestExactStationProviderListExcludesGenericHostedGeneration(t *testing.T) {
	if got := ExactStationProviderIDs(); !reflect.DeepEqual(got, []string{"ollama"}) {
		t.Fatalf("ExactStationProviderIDs()=%v want [ollama]", got)
	}
	for _, providerID := range []string{"openai", "qwen", "anthropic", "google"} {
		definition, ok := Lookup(providerID)
		if !ok {
			t.Fatalf("Lookup(%q) missing", providerID)
		}
		if definition.SupportsExactPreparedStations {
			t.Errorf("hosted provider %q claims exact prepared station support", providerID)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
