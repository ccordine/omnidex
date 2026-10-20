package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParserCatalogPreservesModelAndHost(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "models.json")
	original := []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":500000,"tool_mode":"code_mode_only","experimental_supported_tools":["clock"],"default_reasoning_level":"low"}]}`)
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := parserCatalog(source, root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	m := result.Models[0]
	if m["slug"] != "gpt-6.1-sol" || m["context_window"] != float64(500000) || m["default_reasoning_level"] != "low" || m["tool_mode"] != "direct" || len(m["experimental_supported_tools"].([]any)) != 0 {
		t.Fatalf("unexpected projection: %s", data)
	}
	unchanged, _ := os.ReadFile(source)
	if string(unchanged) != string(original) {
		t.Fatal("host catalog changed")
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model_catalog_json = '"+source+"'"), 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := parserCatalogSource(root)
	if err != nil || selected != source {
		t.Fatalf("configured catalog lost: %s %v", selected, err)
	}
	if err := os.WriteFile(source, []byte(`{"models":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := parserCatalog(source, root); err == nil {
		t.Fatal("empty catalog accepted")
	}
}
