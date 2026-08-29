package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelSettingsRequestRejectsLooseOrUnboundedJSON(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body []byte
	}{
		{name: "duplicate", body: []byte(`{"values":{},"values":{}}`)},
		{name: "nested duplicate", body: []byte(`{"values":{"coding_surface_model":"one","coding_surface_model":"two"}}`)},
		{name: "unknown", body: []byte(`{"values":{},"agent":"codex"}`)},
		{name: "inexact case", body: []byte(`{"Values":{}}`)},
		{name: "trailing", body: []byte(`{"values":{}} {}`)},
		{name: "missing", body: []byte(`{}`)},
		{name: "null map", body: []byte(`{"values":null}`)},
		{name: "null value", body: []byte(`{"values":{"coding_surface_model":null}}`)},
		{name: "oversized", body: bytes.Repeat([]byte(" "), int(maxModelSettingsBodyBytes+1))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/v1/settings/models", bytes.NewReader(testCase.body))
			response := httptest.NewRecorder()
			if _, err := decodeModelSettingsRequest(response, request); err == nil {
				t.Fatalf("invalid model settings body accepted: %q", testCase.body)
			}
		})
	}
}

func TestModelSettingsUsesExactStationCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"OMNI_CODING_SURFACE_MODEL=qwen3:4b-surface",
		"OMNI_CODING_REQUIREMENTS_MODEL=qwen2.5-coder:7b-requirements",
		"OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b-deployment",
		"OMNI_CODING_WORKLOAD_MODEL=qwen3.5:27b-workload",
		"OMNI_CODING_ARTIFACT_HANDLING_MODEL=qwen2.5:3b-artifact",
		"OMNI_CODING_CAPABILITY_RELATION_MODEL=qwen3:4b-relation",
		"OMNI_CODING_SKILL_SELECTION_MODEL=qwen3:4b-skill-select",
		"OMNI_CODING_FRAGMENT_MODEL=qwen3-coder:30b-fragment",
		"OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL=deepseek-r1:8b-guidance",
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
	if values["coding_surface_model"] != "qwen3:4b-surface" {
		t.Fatalf("coding_surface_model=%q", values["coding_surface_model"])
	}
	if values["coding_requirements_model"] != "qwen2.5-coder:7b-requirements" {
		t.Fatalf("coding_requirements_model=%q", values["coding_requirements_model"])
	}
	if values["coding_service_deployment_intent_model"] != "phi4:14b-deployment" {
		t.Fatalf(
			"coding_service_deployment_intent_model=%q",
			values["coding_service_deployment_intent_model"],
		)
	}
	if values["coding_workload_model"] != "qwen3.5:27b-workload" {
		t.Fatalf("coding_workload_model=%q", values["coding_workload_model"])
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
	if values["coding_fragment_model"] != "qwen3-coder:30b-fragment" {
		t.Fatalf("coding_fragment_model=%q", values["coding_fragment_model"])
	}
	if values["coding_fragment_repair_guidance_model"] != "deepseek-r1:8b-guidance" {
		t.Fatalf("coding_fragment_repair_guidance_model=%q", values["coding_fragment_repair_guidance_model"])
	}
	if values["coding_fragment_correction_model"] != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding_fragment_correction_model=%q", values["coding_fragment_correction_model"])
	}
	for _, removed := range []string{"default_model", "fast_model", "glue_model", "reasoning_model", "planner_model", "analyzer_model", "responder_model", "tagger_model", "search_model", "memory_model", "executor_model", "thinking_model", "evaluator_model", "file_worker_model", "coding_plan_model", "coding_repair_model", "coding_file_model", "coding_source_review_model", "coding_source_model", "coding_product_identity_model", "coding_requirement_partition_model", "coding_requirement_quote_model", "coding_semantic_check_model", "coding_semantics_model", "coding_requirement_adviser_model", "coding_requirement_split_model", "coding_skill_procedure_model"} {
		if _, exists := values[removed]; exists {
			t.Fatalf("removed model role %q remains in Admin settings", removed)
		}
	}
}

func TestModelSettingsRejectsRemovedRequirementRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OMNI_MODEL=stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", path)

	for _, key := range []string{"coding_requirement_adviser_model", "coding_requirement_split_model"} {
		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/settings/models",
			strings.NewReader(`{"values":{"`+key+`":"removed"}}`),
		)
		recorder := httptest.NewRecorder()
		(&Server{}).handleModelSettings(recorder, req)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), key) {
			t.Fatalf("key=%s status=%d body=%s", key, recorder.Code, recorder.Body.String())
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "OMNI_MODEL=stable\n" {
		t.Fatalf("rejected settings mutated env file: %q", raw)
	}
}

func TestModelSettingsReadFailsOnRemovedEnvironmentRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER=removed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", path)
	if _, err := buildModelSettingsResponse(); err == nil || !strings.Contains(err.Error(), "OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER") {
		t.Fatalf("error=%v", err)
	}
}
