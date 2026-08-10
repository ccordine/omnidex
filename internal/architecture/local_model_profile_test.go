package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	localSemanticModel = "qwen3.5:9b-q4_K_M"
	localFragmentModel = "qwen3-coder:30b"
)

func TestLocalModelProfileUsesStableSemanticAndFragmentModels(t *testing.T) {
	semanticKeys := []string{
		"OLLAMA_MODEL",
		"OLLAMA_MODEL_FAST",
		"OLLAMA_MODEL_GLUE",
		"OLLAMA_MODEL_REASONING",
		"OLLAMA_MODEL_TAGGER",
		"OLLAMA_MODEL_PLANNER",
		"OLLAMA_MODEL_ANALYZER",
		"OLLAMA_MODEL_RESPONDER",
		"OLLAMA_MODEL_SEARCH",
		"OLLAMA_MODEL_MEMORY",
		"OLLAMA_MODEL_SPECIALIST_PLANNER",
		"OLLAMA_MODEL_SPECIALIST_TOOLING",
		"OLLAMA_MODEL_SPECIALIST_FILESYSTEM_RESEARCH",
		"OLLAMA_MODEL_SPECIALIST_INTENT_TAGGING",
		"OLLAMA_MODEL_SPECIALIST_MEMORY_RETRIEVAL",
		"OLLAMA_MODEL_SPECIALIST_WEB_RESEARCH",
		"OLLAMA_MODEL_SPECIALIST_ANALYSIS",
		"OLLAMA_MODEL_SPECIALIST_RESPONSE",
		"OLLAMA_MODEL_SPECIALIST_REVIEW_VERIFICATION",
		"OLLAMA_MODEL_SPECIALIST_MEDIA_CONTROL",
		"OLLAMA_MODEL_SPECIALIST_BROWSER_INSPECTION",
		"OLLAMA_MODEL_SPECIALIST_SCREEN_VISION",
		"OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION",
		"OLLAMA_MODEL_SPECIALIST_AUDIO_NOTES",
		"OLLAMA_MODEL_SPECIALIST_CODING_SURFACE",
		"OLLAMA_MODEL_SPECIALIST_CODING_PRODUCT_IDENTITY",
		"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_PARTITION",
		"OLLAMA_MODEL_SPECIALIST_CODING_ARTIFACT_HANDLING",
		"OLLAMA_MODEL_SPECIALIST_CODING_CAPABILITY_RELATION",
		"OLLAMA_MODEL_SPECIALIST_CODING_SKILL_SELECTION",
		"OLLAMA_MODEL_SPECIALIST_CODING_SKILL_PROCEDURE",
	}

	for _, name := range []string{"default.env", ".env.example"} {
		values := readEnvTemplate(t, name)
		for _, key := range semanticKeys {
			if got := values[key]; got != localSemanticModel {
				t.Errorf("%s: %s=%q, want %q", name, key, got, localSemanticModel)
			}
		}
		for _, key := range []string{
			"OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT",
			"OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT_CORRECTION",
		} {
			if got := values[key]; got != localFragmentModel {
				t.Errorf("%s: %s=%q, want %q", name, key, got, localFragmentModel)
			}
		}
		for _, removed := range []string{
			"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER",
			"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_SPLIT",
		} {
			if _, exists := values[removed]; exists {
				t.Errorf("%s: removed production route %s remains configured", name, removed)
			}
		}
		if got := values["INFERENCE_CONTEXT_TOKENS"]; got != "32768" {
			t.Errorf("%s: INFERENCE_CONTEXT_TOKENS=%q, want 32768", name, got)
		}
		if got := values["CODING_FRAGMENT_CONCURRENCY"]; got != "1" {
			t.Errorf("%s: CODING_FRAGMENT_CONCURRENCY=%q, want 1", name, got)
		}
	}
}

func TestReadmeCannotAdvertiseStaleOrFabricatedCognitionConfiguration(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "README.md")))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(raw)
	if strings.Contains(contents, "INFERENCE_CONTEXT_TOKENS=16384") {
		t.Fatal("README advertises a native context too small for the required cognition station")
	}
	if !strings.Contains(contents, "Production cognition is currently Ollama-only") {
		t.Fatal("README does not disclose the exact prepared-provider cognition boundary")
	}
	required := []string{
		"INFERENCE_CONTEXT_TOKENS=32768",
		"COGNITION_MODEL_SHA256=",
		"COGNITION_MODEL_QUANTIZATION=",
		"COGNITION_BACKEND_VERSION=",
		"COGNITION_HARDWARE=",
		"COGNITION_CONTEXT_CEILING_BYTES=",
		"COGNITION_MAX_OUTPUT_TOKENS=",
	}
	for _, line := range required {
		if !strings.Contains(contents, "\n"+line+"\n") {
			t.Fatalf("README omits truthful cognition configuration line %q", line)
		}
	}
}

func readEnvTemplate(t *testing.T, name string) map[string]string {
	t.Helper()
	path := filepath.Clean(filepath.Join("..", "..", name))
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return values
}
