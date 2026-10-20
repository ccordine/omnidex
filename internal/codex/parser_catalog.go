package codex

import (
	"encoding/json"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"io"
	"os"
	"path/filepath"
)

// Native model metadata can force code-mode tools even when every capability
// flag is off. Project only that harness choice into a private catalog; preserve
// model identity, limits, reasoning support, and the host catalog unchanged.
func parserCatalog(source, scratch string) (string, error) {
	file, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("parser model catalog: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		return "", err
	}
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return "", fmt.Errorf("parser model catalog: %w", err)
	}
	if len(catalog.Models) == 0 {
		return "", fmt.Errorf("parser model catalog is empty")
	}
	for _, model := range catalog.Models {
		model["tool_mode"] = "direct"
		model["experimental_supported_tools"] = []string{}
	}
	data, err = json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	path := filepath.Join(scratch, "parser-models.json")
	return path, os.WriteFile(path, data, 0600)
}

func parserCatalogSource(profile string) (string, error) {
	var config struct {
		Catalog string `toml:"model_catalog_json"`
	}
	data, err := os.ReadFile(filepath.Join(profile, "config.toml"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		if err := toml.Unmarshal(data, &config); err != nil {
			return "", err
		}
	}
	if config.Catalog != "" {
		return config.Catalog, nil
	}
	return filepath.Join(profile, "models_cache.json"), nil
}
