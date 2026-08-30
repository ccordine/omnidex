package modelconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestMergePriority(t *testing.T) {
	env := Config{"conversation_response_model": "env-response", "coding_fragment_model": "env-fragment"}
	project := Config{
		"conversation_response_model": "project-response",
		"coding_fragment_model":       "project-fragment",
	}

	merged := Merge(env, project)
	if merged.Get("conversation_response_model") != "project-response" {
		t.Fatalf("expected project response route, got %q", merged.Get("conversation_response_model"))
	}
	if merged.Get("coding_fragment_model") != "project-fragment" {
		t.Fatalf("expected project fragment route, got %q", merged.Get("coding_fragment_model"))
	}
}

func TestFromSettingsJSON(t *testing.T) {
	raw := json.RawMessage(`{"model_config":{"conversation_response_model":"project-only"}}`)
	cfg, err := FromSettingsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("conversation_response_model") != "project-only" {
		t.Fatalf("expected project-only, got %q", cfg.Get("conversation_response_model"))
	}
}

func TestFromJSONRejectsMalformedAndUnknownValues(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"default_model":42}`),
		json.RawMessage(`{"legacy_model":"x"}`),
		json.RawMessage(`{"thinking_model":"x"}`),
		json.RawMessage(`{"evaluator_model":"x"}`),
		json.RawMessage(`{"file_worker_model":"x"}`),
		json.RawMessage(`{"coding_product_identity_model":"x"}`),
		json.RawMessage(`{"coding_requirement_partition_model":"x"}`),
		json.RawMessage(`{"coding_workload_review_model":"x"}`),
		json.RawMessage(`{"coding_requirement_adviser_model":"x"}`),
		json.RawMessage(`{"coding_requirement_split_model":"x"}`),
		json.RawMessage(`{"database_evidence_refinement_model":"x"}`),
		json.RawMessage(`{"web_grounded_synthesis_correction_model":"x"}`),
		json.RawMessage(`{"web_claim_evidence_review_model":"x"}`),
		json.RawMessage(`{"default_model":"x"}`),
		json.RawMessage(`{"fast_model":"x"}`),
		json.RawMessage(`{"glue_model":"x"}`),
		json.RawMessage(`{"reasoning_model":"x"}`),
		json.RawMessage(`{"planner_model":"x"}`),
		json.RawMessage(`{"analyzer_model":"x"}`),
		json.RawMessage(`{"responder_model":"x"}`),
		json.RawMessage(`{"tagger_model":"x"}`),
		json.RawMessage(`{"search_model":"x"}`),
		json.RawMessage(`{"memory_model":"x"}`),
		json.RawMessage(`{"objective_advisory_model":"x"}`),
		json.RawMessage(`{"roleplay_canon_extraction_model":"x"}`),
		json.RawMessage(`{"roleplay_ongoing_action_model":"x"}`),
		json.RawMessage(`{"default_model":"x"} {}`),
	} {
		if _, err := FromJSON(raw); err == nil {
			t.Fatalf("model config %s must fail", raw)
		}
	}
}

func TestFromJSONRejectsRetiredObjectiveAdvisoryRoute(t *testing.T) {
	_, err := FromJSON(json.RawMessage(`{"objective_advisory_model":"qwen"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported field "objective_advisory_model"`) {
		t.Fatalf("retired objective advisory route error=%v", err)
	}
}

func TestValidateEnvironmentValuesRejectsRemovedRequirementRoutes(t *testing.T) {
	for _, key := range RemovedEnvironmentKeys() {
		if err := ValidateEnvironmentValues(map[string]string{key: ""}); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("key=%s error=%v", key, err)
		}
	}
	if err := ValidateEnvironmentValues(map[string]string{"OMNI_CODING_REQUIREMENTS_MODEL": "stable"}); err != nil {
		t.Fatal(err)
	}
}

func TestModelNamesReturnsEverySelectedProviderModel(t *testing.T) {
	cfg := Config{
		"conversation_objective_kind_model": "llama3.2:latest",
		"web_relevance_model":               "gpt-4o",
		"conversation_response_model":       "qwen2.5:7b",
	}
	names := cfg.ModelNames()
	if len(names) != 3 {
		t.Fatalf("expected all 3 provider model IDs, got %v", names)
	}
}

func TestApplyExactStationRoutingFields(t *testing.T) {
	applied := Apply(Routing{}, Config{
		"conversation_objective_kind_model":      "qwen3:4b-kind",
		"conversation_response_model":            "qwen3:8b-response",
		"roleplay_semantic_model":                "qwen3:8b-roleplay-fidelity",
		"grounded_answer_model":                  "qwen3:8b-grounded",
		"web_relevance_model":                    "qwen3:4b-relevance",
		"web_grounded_synthesis_model":           "qwen3:8b-synthesis",
		"coding_surface_model":                   "qwen3:4b-surface",
		"coding_requirements_model":              "qwen2.5-coder:7b-requirements",
		"coding_service_deployment_intent_model": "phi4:14b-deployment",
		"coding_workload_model":                  "qwen3.5:27b-workload",
		"coding_artifact_handling_model":         "qwen2.5:3b-artifact",
		"coding_capability_relation_model":       "qwen3:4b-relation",
		"coding_skill_selection_model":           "qwen3:4b-skill-select",
		"coding_fragment_model":                  "qwen3-coder:30b-fragment",
		"coding_fragment_repair_guidance_model":  "deepseek-r1:8b-guidance",
		"coding_fragment_correction_model":       "qwen2.5-coder:14b-correction",
	})
	if got := applied.Stations[station.ConversationObjectiveKind]; got != "qwen3:4b-kind" {
		t.Fatalf("conversation kind model=%q", got)
	}
	if got := applied.Stations[station.ConversationResponse]; got != "qwen3:8b-response" {
		t.Fatalf("conversation response model=%q", got)
	}
	if got := applied.RoleplaySemanticModel; got != "qwen3:8b-roleplay-fidelity" {
		t.Fatalf("roleplay semantic model=%q", got)
	}
	if got := applied.Stations[station.GroundedAnswer]; got != "qwen3:8b-grounded" {
		t.Fatalf("grounded answer model=%q", got)
	}
	if got := applied.Stations[station.WebRelevance]; got != "qwen3:4b-relevance" {
		t.Fatalf("web relevance model=%q", got)
	}
	if got := applied.Stations[station.WebGroundedSynthesis]; got != "qwen3:8b-synthesis" {
		t.Fatalf("web synthesis model=%q", got)
	}
	if got := applied.Stations[station.CodingSurface]; got != "qwen3:4b-surface" {
		t.Fatalf("coding surface model=%q", got)
	}
	if got := applied.Stations[station.CodingRequirements]; got != "qwen2.5-coder:7b-requirements" {
		t.Fatalf("coding requirements model=%q", got)
	}
	if got := applied.Stations[station.CodingProjectStackConstraint]; got != "qwen2.5-coder:7b-requirements" {
		t.Fatalf("coding project stack constraint model=%q", got)
	}
	if got := applied.Stations[station.CodingServiceContinuedAvailability]; got != "phi4:14b-deployment" {
		t.Fatalf("coding service continued availability model=%q", got)
	}
	if got := applied.Stations[station.CodingServicePersistenceDestination]; got != "phi4:14b-deployment" {
		t.Fatalf("coding service persistence destination model=%q", got)
	}
	if got := applied.Stations[station.CodingServiceStateLifetime]; got != "qwen3.5:27b-workload" {
		t.Fatalf("coding service state lifetime model=%q", got)
	}
	if got := applied.Stations[station.CodingServiceEndpointRequirement]; got != "qwen3.5:27b-workload" {
		t.Fatalf("coding service endpoint requirement model=%q", got)
	}
	if got := applied.Stations[station.CodingArtifactHandling]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact handling model=%q", got)
	}
	if got := applied.Stations[station.CodingRepositoryArtifactAbsence]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding repository artifact absence model=%q", got)
	}
	if got := applied.Stations[station.CodingPlainTextArtifactCreation]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding plain-text artifact creation model=%q", got)
	}
	if got := applied.Stations[station.CodingDeclarationArtifactBoundary]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding declaration artifact boundary model=%q", got)
	}
	if got := applied.Stations[station.CodingArtifactCandidateSelection]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact candidate selection model=%q", got)
	}
	if got := applied.Stations[station.CodingCapabilityRelation]; got != "qwen3:4b-relation" {
		t.Fatalf("coding capability relation model=%q", got)
	}
	if got := applied.Stations[station.CodingSkillSelection]; got != "qwen3:4b-skill-select" {
		t.Fatalf("coding skill selection model=%q", got)
	}
	if got := applied.Stations[station.CodingFragment]; got != "qwen3-coder:30b-fragment" {
		t.Fatalf("coding fragment model=%q", got)
	}
	if got := applied.Stations[station.CodingFragmentRepairGuidance]; got != "deepseek-r1:8b-guidance" {
		t.Fatalf("coding fragment repair guidance model=%q", got)
	}
	if got := applied.Stations[station.CodingFragmentCorrection]; got != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding fragment correction model=%q", got)
	}
}
