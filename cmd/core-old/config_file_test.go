package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCoreEnvironmentFileUsesExactStandaloneAuthority(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "default.env"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateCoreEnvironmentFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCoreEnvironmentFileRejectsRemovedOrMalformedConfiguration(t *testing.T) {
	tests := map[string]func([]byte) []byte{
		"removed": func(raw []byte) []byte {
			return append(raw, []byte("APP_ENV=production\n")...)
		},
		"malformed": func(raw []byte) []byte {
			return []byte(strings.Replace(string(raw), "WORKER_COUNT=3", "WORKER_COUNT=many", 1))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := filepath.Clean(filepath.Join("..", ".."))
			raw, err := os.ReadFile(filepath.Join(root, "default.env"))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, mutate(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			err = validateCoreEnvironmentFile(path)
			if err == nil || (!strings.Contains(err.Error(), "removed") && !strings.Contains(err.Error(), "integer")) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateCoreEnvironmentFileRejectsMissingStationRouteBeforePublish(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "default.env"))
	if err != nil {
		t.Fatal(err)
	}
	line := "OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b\n"
	mutated := []byte(strings.Replace(string(raw), line, "", 1))
	if string(mutated) == string(raw) {
		t.Fatalf("default.env lacks exact deployment-semantics route fixture")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	err = validateCoreEnvironmentFile(path)
	if err == nil || !strings.Contains(err.Error(), "coding_service_continued_availability") {
		t.Fatalf("missing station route error=%v", err)
	}
}
