package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var removedCognitionEnvironmentKeys = []string{
	"COGNITION_MODEL_SHA256", "COGNITION_MODEL_QUANTIZATION",
	"COGNITION_BACKEND_VERSION", "COGNITION_HARDWARE",
	"COGNITION_CONTEXT_CEILING_BYTES", "COGNITION_MAX_OUTPUT_TOKENS",
}

func TestEnvironmentTemplatesDoNotAdvertiseRemovedUniversalCognitionBrain(t *testing.T) {
	root := coreRepositoryRoot(t)
	profiles := []string{".env.example", "default.env"}
	if _, err := os.Stat(filepath.Join(root, ".env")); err == nil {
		profiles = append(profiles, ".env")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, name := range profiles {
		values := readEnvironmentTemplate(t, filepath.Join(root, name))
		if _, exists := values["INFERENCE_CONTEXT_TOKENS"]; !exists {
			t.Fatalf("%s omits the model-call context bound", name)
		}
		for _, key := range removedCognitionEnvironmentKeys {
			if _, exists := values[key]; exists {
				t.Fatalf("%s retains removed universal cognition setting %s", name, key)
			}
		}
	}
	for _, name := range []string{"docker-compose.yml", "README.md"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range removedCognitionEnvironmentKeys {
			if strings.Contains(string(raw), key) {
				t.Fatalf("%s retains removed universal cognition setting %s", name, key)
			}
		}
	}
}

func coreRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve core test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func readEnvironmentTemplate(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
