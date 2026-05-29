package secrets

import (
	"context"
	"testing"
)

func TestMergeStoredKeepsExistingUnlessUpdated(t *testing.T) {
	current := map[string]string{"openai_api_key": "sk-old"}
	merged := MergeStored(current, map[string]string{"cursor_api_key": "cursor-new"}, nil)
	if merged["openai_api_key"] != "sk-old" {
		t.Fatalf("expected old openai key preserved, got %q", merged["openai_api_key"])
	}
	if merged["cursor_api_key"] != "cursor-new" {
		t.Fatalf("expected cursor key added, got %q", merged["cursor_api_key"])
	}
}

func TestMergeStoredClearKey(t *testing.T) {
	current := map[string]string{"openai_api_key": "sk-old", "cursor_api_key": "cursor-old"}
	merged := MergeStored(current, nil, []string{"cursor_api_key"})
	if merged["cursor_api_key"] != "" {
		t.Fatalf("expected cursor key cleared")
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
	store := &MemoryStore{Values: map[string]string{"cursor_api_key": "db-cursor"}}
	resolver := NewResolver(store)
	if got := resolver.Get(context.Background(), "cursor_api_key"); got != "db-cursor" {
		t.Fatalf("expected db value, got %q", got)
	}
}

func TestCodexAPIKeyFallback(t *testing.T) {
	store := &MemoryStore{Values: map[string]string{"openai_api_key": "sk-openai"}}
	SetGlobal(NewResolver(store))
	defer SetGlobal(nil)
	if got := CodexAPIKey(); got != "sk-openai" {
		t.Fatalf("expected openai fallback, got %q", got)
	}
}
