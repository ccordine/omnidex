package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	localSemanticModel         = "qwen3.5:9b-q4_K_M"
	localDeploymentIntentModel = "phi4:14b"
	localFragmentModel         = "qwen2.5-coder:7b"
	localRepairGuidanceModel   = "qwen3.5:9b-q4_K_M"
	localReviewModel           = "deepseek-r1:8b"
)

func TestLocalModelProfileUsesStableSemanticAndFragmentModels(t *testing.T) {
	semanticKeys := []string{
		"OMNI_CONTEXT_SEARCH_TERMS_MODEL",
		"OMNI_CONTEXT_RELEVANCE_MODEL",
		"OMNI_CONTEXT_MINIFICATION_MODEL",
		"OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL",
		"OMNI_CONVERSATION_RESPONSE_MODEL",
		"OMNI_ROLEPLAY_SEMANTIC_MODEL",
		"OMNI_GROUNDED_ANSWER_MODEL",
		"OMNI_DATABASE_SCHEMA_SELECTION_MODEL",
		"OMNI_DATABASE_QUERY_INTENT_MODEL",
		"OMNI_DATABASE_EVIDENCE_GAP_MODEL",
		"OMNI_DATABASE_JOIN_PATH_SELECTION_MODEL",
		"OMNI_REPOSITORY_EVIDENCE_RELEVANCE_MODEL",
		"OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL",
		"OMNI_WEB_SEARCH_TERMS_MODEL",
		"OMNI_WEB_RELEVANCE_MODEL",
		"OMNI_WEB_GROUNDED_SYNTHESIS_MODEL",
		"OMNI_WEB_GROUNDED_SYNTHESIS_CORRECTION_MODEL",
		"OMNI_CODING_SURFACE_MODEL",
		"OMNI_CODING_ARTIFACT_HANDLING_MODEL",
		"OMNI_CODING_CAPABILITY_RELATION_MODEL",
		"OMNI_CODING_SKILL_SELECTION_MODEL",
		"OMNI_CODING_REPOSITORY_SEARCH_TERM_MODEL",
		"OMNI_CODING_REPOSITORY_CHANGE_SURFACE_MODEL",
	}

	for _, name := range []string{"default.env", ".env.example"} {
		values := readEnvTemplate(t, name)
		for _, key := range semanticKeys {
			if got := values[key]; got != localSemanticModel {
				t.Errorf("%s: %s=%q, want %q", name, key, got, localSemanticModel)
			}
		}
		for _, key := range []string{
			"OMNI_CODING_REQUIREMENTS_MODEL",
			"OMNI_CODING_WORKLOAD_MODEL",
		} {
			if got := values[key]; got != localSemanticModel {
				t.Errorf("%s: %s=%q, want %q", name, key, got, localSemanticModel)
			}
		}
		if got := values["OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL"]; got != localDeploymentIntentModel {
			t.Errorf("%s: deployment intent model=%q, want %q", name, got, localDeploymentIntentModel)
		}
		if got := values["OMNI_WEB_CLAIM_EVIDENCE_REVIEW_MODEL"]; got != localReviewModel {
			t.Errorf("%s: independent web review model=%q, want %q", name, got, localReviewModel)
		}
		if got := values["OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL"]; got != localReviewModel {
			t.Errorf("%s: independent repository review model=%q, want %q", name, got, localReviewModel)
		}
		for _, key := range []string{
			"OMNI_CODING_FRAGMENT_MODEL",
			"OMNI_CODING_FRAGMENT_CORRECTION_MODEL",
		} {
			if got := values[key]; got != localFragmentModel {
				t.Errorf("%s: %s=%q, want %q", name, key, got, localFragmentModel)
			}
		}
		if got := values["OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL"]; got != localRepairGuidanceModel {
			t.Errorf("%s: repair guidance model=%q, want %q", name, got, localRepairGuidanceModel)
		}
		for _, removed := range []string{
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
			"OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT",
			"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER",
			"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_SPLIT",
			"OMNI_CODING_PRODUCT_IDENTITY_MODEL",
			"OMNI_CODING_REQUIREMENT_PARTITION_MODEL",
			"OMNI_CODING_WORKLOAD_REVIEW_MODEL",
			"OMNI_CONVERSATION_CONTEXT_SELECTION_MODEL",
			"OMNI_MEMORY_CONTEXT_SELECTION_MODEL",
			"OMNI_ROLEPLAY_NARRATIVE_CONTINUITY_MODEL",
			"OMNI_ROLEPLAY_CANON_EXTRACTION_MODEL",
			"OMNI_ROLEPLAY_ONGOING_ACTION_MODEL",
		} {
			if _, exists := values[removed]; exists {
				t.Errorf("%s: removed production route %s remains configured", name, removed)
			}
		}
		if got := values["INFERENCE_CONTEXT_TOKENS"]; got != "8192" {
			t.Errorf("%s: INFERENCE_CONTEXT_TOKENS=%q, want 8192", name, got)
		}
		if got := values["CODING_FRAGMENT_CONCURRENCY"]; got != "1" {
			t.Errorf("%s: CODING_FRAGMENT_CONCURRENCY=%q, want 1", name, got)
		}
	}
}

func TestEnvironmentProfilesHaveOneAuthorityPerKey(t *testing.T) {
	for _, name := range []string{".env.example", "default.env"} {
		readEnvTemplate(t, name)
	}
	active := filepath.Clean(filepath.Join("..", "..", ".env"))
	if _, err := os.Stat(active); err == nil {
		readEnvTemplate(t, ".env")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect .env: %v", err)
	}
}

func TestReadmeCannotAdvertiseStaleOrFabricatedCognitionConfiguration(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "README.md")))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(raw)
	if strings.Contains(contents, "INFERENCE_CONTEXT_TOKENS=4096") {
		t.Fatal("README advertises a context below the exact minimum")
	}
	if !strings.Contains(contents, "there is no process-wide cognition brain or universal cognition policy") {
		t.Fatal("README does not disclose the station-owned inference boundary")
	}
	if !strings.Contains(contents, "\nINFERENCE_CONTEXT_TOKENS=8192\n") {
		t.Fatal("README omits the model-call context bound")
	}
	for _, key := range []string{
		"OMNI_CODING_REQUIREMENTS_MODEL",
		"OMNI_CODING_WORKLOAD_MODEL",
	} {
		authority := key + "=" + localSemanticModel
		if strings.Count(contents, authority) != 1 {
			t.Fatalf("README exact route %q count=%d, want 1", authority, strings.Count(contents, authority))
		}
	}
	if strings.Contains(contents, "llama3.2:3b") {
		t.Fatal("README advertises the removed Llama requirement/workload route")
	}
	for _, removed := range []string{
		"COGNITION_MODEL_SHA256=",
		"COGNITION_MODEL_QUANTIZATION=",
		"COGNITION_BACKEND_VERSION=",
		"COGNITION_HARDWARE=",
		"COGNITION_CONTEXT_CEILING_BYTES=",
		"COGNITION_MAX_OUTPUT_TOKENS=",
	} {
		if strings.Contains(contents, removed) {
			t.Fatalf("README advertises removed universal cognition setting %q", removed)
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
			key = strings.TrimSpace(key)
			if _, exists := values[key]; exists {
				t.Fatalf("%s contains duplicate environment key %s", name, key)
			}
			values[key] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return values
}
