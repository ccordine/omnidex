package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelSettingsUsesAuthoritativeModelRoleCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"OMNI_EXECUTOR_MODEL=qwen3-coder:30b",
		"OMNI_GLUE_MODEL=qwen2.5-coder:3b",
		"OMNI_CODING_SURFACE_MODEL=qwen3:4b-surface",
		"OMNI_CODING_REQUIREMENT_PARTITION_MODEL=qwen2.5-coder:7b-partition",
		"OMNI_CODING_ARTIFACT_HANDLING_MODEL=qwen2.5:3b-artifact",
		"OMNI_CODING_CAPABILITY_RELATION_MODEL=qwen3:4b-relation",
		"OMNI_CODING_SKILL_SELECTION_MODEL=qwen3:4b-skill-select",
		"OMNI_CODING_SKILL_PROCEDURE_MODEL=qwen3:4b-skill",
		"OMNI_CODING_FRAGMENT_MODEL=qwen3-coder:30b-fragment",
		"OMNI_CODING_FRAGMENT_CORRECTION_MODEL=qwen2.5-coder:14b-correction",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", path)

	payload, err := buildModelSettingsResponse()
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := payload["fields"].([]map[string]any)
	if !ok {
		t.Fatalf("fields type=%T", payload["fields"])
	}
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		key, _ := field["key"].(string)
		value, _ := field["value"].(string)
		values[key] = value
	}
	if values["executor_model"] != "qwen3-coder:30b" {
		t.Fatalf("executor_model=%q", values["executor_model"])
	}
	if values["glue_model"] != "qwen2.5-coder:3b" {
		t.Fatalf("glue_model=%q", values["glue_model"])
	}
	if values["coding_surface_model"] != "qwen3:4b-surface" {
		t.Fatalf("coding_surface_model=%q", values["coding_surface_model"])
	}
	if values["coding_requirement_partition_model"] != "qwen2.5-coder:7b-partition" {
		t.Fatalf("coding_requirement_partition_model=%q", values["coding_requirement_partition_model"])
	}
	if values["coding_artifact_handling_model"] != "qwen2.5:3b-artifact" {
		t.Fatalf("coding_artifact_handling_model=%q", values["coding_artifact_handling_model"])
	}
	if values["coding_capability_relation_model"] != "qwen3:4b-relation" {
		t.Fatalf("coding_capability_relation_model=%q", values["coding_capability_relation_model"])
	}
	if values["coding_skill_selection_model"] != "qwen3:4b-skill-select" {
		t.Fatalf("coding_skill_selection_model=%q", values["coding_skill_selection_model"])
	}
	if values["coding_skill_procedure_model"] != "qwen3:4b-skill" {
		t.Fatalf("coding_skill_procedure_model=%q", values["coding_skill_procedure_model"])
	}
	if values["coding_fragment_model"] != "qwen3-coder:30b-fragment" {
		t.Fatalf("coding_fragment_model=%q", values["coding_fragment_model"])
	}
	if values["coding_fragment_correction_model"] != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding_fragment_correction_model=%q", values["coding_fragment_correction_model"])
	}
	for _, removed := range []string{"thinking_model", "evaluator_model", "file_worker_model", "coding_plan_model", "coding_repair_model", "coding_file_model", "coding_source_review_model", "coding_source_model", "coding_requirement_quote_model", "coding_semantic_check_model", "coding_semantics_model"} {
		if _, exists := values[removed]; exists {
			t.Fatalf("removed model role %q remains in Admin settings", removed)
		}
	}
}
