package secrets

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func TestMergeStoredKeepsExistingUnlessUpdated(t *testing.T) {
	current := map[string]string{"openai_api_key": "sk-old"}
	merged, err := MergeStored(current, map[string]string{"qwen_api_key": "qwen-new"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged["openai_api_key"] != "sk-old" {
		t.Fatalf("expected old openai key preserved, got %q", merged["openai_api_key"])
	}
	if merged["qwen_api_key"] != "qwen-new" {
		t.Fatalf("expected qwen key added, got %q", merged["qwen_api_key"])
	}
}

func TestMergeStoredClearKey(t *testing.T) {
	current := map[string]string{"openai_api_key": "sk-old", "qwen_api_key": "qwen-old"}
	merged, err := MergeStored(current, nil, []string{"qwen_api_key"})
	if err != nil {
		t.Fatal(err)
	}
	if merged["qwen_api_key"] != "" {
		t.Fatalf("expected qwen key cleared")
	}
	if merged["openai_api_key"] != "sk-old" {
		t.Fatalf("expected openai key preserved")
	}
}

func TestFieldListMasksStoredValues(t *testing.T) {
	fields := FieldList(map[string]string{"openai_api_key": "sk-live-1234"})
	if len(fields) != len(Fields) {
		t.Fatalf("expected %d fields, got %d", len(Fields), len(fields))
	}
	var openai map[string]any
	for _, field := range fields {
		if field["key"] == "openai_api_key" {
			openai = field
			break
		}
	}
	if openai == nil {
		t.Fatal("openai field missing")
	}
	if openai["configured"] != true {
		t.Fatal("expected configured=true")
	}
	if openai["source"] != "database" {
		t.Fatalf("expected database source, got %#v", openai["source"])
	}
	if openai["hint"] != "••••1234" {
		t.Fatalf("expected masked hint, got %#v", openai["hint"])
	}
}

func TestResolverPrefersDatabase(t *testing.T) {
	store := &MemoryStore{Values: map[string]string{"qwen_api_key": "db-qwen"}}
	resolver := NewResolver(store)
	if got := resolver.Get(context.Background(), "qwen_api_key"); got != "db-qwen" {
		t.Fatalf("expected db value, got %q", got)
	}
}

func TestSecretFieldsExcludeRetiredWholeWorkspaceAgentCredentials(t *testing.T) {
	for _, field := range Fields {
		if field.Key == "cursor_api_key" || field.Key == "codex_api_key" {
			t.Fatalf("retired external-agent credential remains exposed: %q", field.Key)
		}
		for _, envKey := range field.EnvKeys {
			if envKey == "CURSOR_API_KEY" || envKey == "CODEX_API_KEY" {
				t.Fatalf("retired external-agent environment authority remains exposed: %q", envKey)
			}
		}
	}
}

func TestStoredSecretAuthorityRejectsRetiredOrNoncanonicalFields(t *testing.T) {
	for _, stored := range []map[string]string{
		{"cursor_api_key": "retired"},
		{"codex_api_key": "retired"},
		{"openai_api_key": " padded "},
		{"openai_api_key": "nul\x00value"},
	} {
		if err := ValidateStored(stored); err == nil {
			t.Fatalf("invalid stored secret authority accepted: %#v", stored)
		}
	}
}

func TestSecretFieldsCoverEveryCatalogProviderCredential(t *testing.T) {
	fieldsByKey := make(map[string]Field, len(Fields))
	for _, field := range Fields {
		fieldsByKey[field.Key] = field
	}
	for _, definition := range catalog.ProductionDefinitions() {
		if len(definition.APIKeyEnvironmentKeys) == 0 {
			continue
		}
		secretKey, ok := ProviderSecretKey(definition.ID)
		if !ok {
			t.Fatalf("ProviderSecretKey(%q) missing", definition.ID)
		}
		field, ok := fieldsByKey[secretKey]
		if !ok {
			t.Fatalf("secret field %q missing for provider %q", secretKey, definition.ID)
		}
		if len(field.EnvKeys) == 0 || field.EnvKeys[0] != definition.APIKeyEnvironmentKeys[0] {
			t.Fatalf("field %q env keys=%v want catalog keys %v", secretKey, field.EnvKeys, definition.APIKeyEnvironmentKeys)
		}
	}
}

func TestOverlayConfigAppliesChineseProviderDatabaseCredential(t *testing.T) {
	store := &MemoryStore{Values: map[string]string{"qwen_api_key": "db-qwen-key"}}
	cfg := config.Config{
		CompatibleProviders: map[string]config.CompatibleProviderConfig{
			"qwen": {BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: "env-key"},
		},
	}
	OverlayConfig(&cfg, NewResolver(store))
	if got := cfg.CompatibleProviders["qwen"].APIKey; got != "db-qwen-key" {
		t.Fatalf("qwen API key=%q want database value", got)
	}
}
