package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

var cognitionEnvironmentKeys = []string{
	"COGNITION_MODEL_SHA256", "COGNITION_MODEL_QUANTIZATION",
	"COGNITION_BACKEND_VERSION", "COGNITION_HARDWARE",
	"COGNITION_CONTEXT_CEILING_BYTES", "COGNITION_MAX_OUTPUT_TOKENS",
}

func TestCognitionEnvironmentTemplatesCannotOmitRequiredTransport(t *testing.T) {
	root := coreRepositoryRoot(t)
	for _, name := range []string{".env", ".env.example", "default.env"} {
		values := readEnvironmentTemplate(t, filepath.Join(root, name))
		for _, key := range append([]string{"INFERENCE_CONTEXT_TOKENS"}, cognitionEnvironmentKeys...) {
			if _, exists := values[key]; !exists {
				t.Fatalf("%s omits required %s", name, key)
			}
		}
	}
	active := readEnvironmentTemplate(t, filepath.Join(root, ".env"))
	for _, key := range cognitionEnvironmentKeys {
		if strings.TrimSpace(active[key]) == "" {
			t.Fatalf("active .env has no exact %s", key)
		}
	}
	native, err := strconv.Atoi(active["INFERENCE_CONTEXT_TOKENS"])
	if err != nil || llm.ValidateInferenceContextTokens(native) != nil {
		t.Fatalf("active cognition native context=%q", active["INFERENCE_CONTEXT_TOKENS"])
	}
	ceiling, ceilingErr := strconv.Atoi(active["COGNITION_CONTEXT_CEILING_BYTES"])
	output, outputErr := strconv.Atoi(active["COGNITION_MAX_OUTPUT_TOKENS"])
	available, _, budgetErr := llm.InferenceInputByteBudget(native, output)
	if ceilingErr != nil || outputErr != nil || budgetErr != nil ||
		ceiling+len(llm.MinimalGeneratePrompt) > available {
		t.Fatalf("active cognition boundary native=%d ceiling=%d output=%d", native, ceiling, output)
	}
	compose, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range cognitionEnvironmentKeys {
		if !strings.Contains(string(compose), "${"+key+":?") {
			t.Fatalf("docker-compose.yml does not require %s", key)
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
