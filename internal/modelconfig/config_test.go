package modelconfig

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/specialist"
)

func TestMergePriority(t *testing.T) {
	env := Config{"default_model": "env-model", "planner_model": "env-planner"}
	project := Config{"default_model": "project-model"}
	card := Config{"planner_model": "card-planner"}

	merged := Merge(env, project, card)
	if merged.Get("default_model") != "project-model" {
		t.Fatalf("expected project default_model, got %q", merged.Get("default_model"))
	}
	if merged.Get("planner_model") != "card-planner" {
		t.Fatalf("expected card planner_model, got %q", merged.Get("planner_model"))
	}
}

func TestFromSettingsJSON(t *testing.T) {
	raw := json.RawMessage(`{"model_config":{"default_model":"project-only"}}`)
	cfg, err := FromSettingsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("default_model") != "project-only" {
		t.Fatalf("expected project-only, got %q", cfg.Get("default_model"))
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
		json.RawMessage(`{"default_model":"x"} {}`),
	} {
		if _, err := FromJSON(raw); err == nil {
			t.Fatalf("model config %s must fail", raw)
		}
	}
}

func TestModelRolesRejectMissingConfiguration(t *testing.T) {
	if _, err := AnalyzerModel(Config{}, ""); err == nil {
		t.Fatal("missing analyzer model must fail")
	}
	if _, err := PlannerTicketModel(Config{}, ""); err == nil {
		t.Fatal("missing planner ticket model must fail")
	}
	if model, err := AnalyzerModel(Config{"reasoning_model": "reasoner"}, ""); err != nil || model != "reasoner" {
		t.Fatalf("model=%q err=%v", model, err)
	}
}

func TestModelNamesReturnsEverySelectedProviderModel(t *testing.T) {
	cfg := Config{
		"default_model":  "llama3.2:latest",
		"planner_model":  "gpt-4o",
		"executor_model": "qwen2.5:7b",
	}
	names := cfg.ModelNames()
	if len(names) != 3 {
		t.Fatalf("expected all 3 provider model IDs, got %v", names)
	}
}

func TestApplyExpandedRoutingFields(t *testing.T) {
	applied := Apply(Routing{
		Default:   "base-default",
		Fast:      "base-fast",
		Reasoning: "base-reasoning",
		Tagging:   "base-tagging",
		Plan:      "base-plan",
		Analyze:   "base-analyze",
		Response:  "base-response",
		Search:    "base-search",
		Memory:    "base-memory",
	}, Config{
		"fast_model":                         "fast",
		"reasoning_model":                    "reasoning",
		"analyzer_model":                     "analyze",
		"responder_model":                    "respond",
		"tagger_model":                       "tag",
		"search_model":                       "search",
		"memory_model":                       "memory",
		"executor_model":                     "qwen3-coder:30b",
		"glue_model":                         "qwen2.5-coder:3b",
		"coding_surface_model":               "qwen3:4b-surface",
		"coding_product_identity_model":      "qwen2.5-coder:14b-identity",
		"coding_requirement_partition_model": "qwen2.5-coder:7b-partition",
		"coding_requirement_adviser_model":   "deepseek-r1:8b",
		"coding_requirement_split_model":     "qwen2.5-coder:7b-split",
		"coding_artifact_handling_model":     "qwen2.5:3b-artifact",
		"coding_capability_relation_model":   "qwen3:4b-relation",
		"coding_skill_selection_model":       "qwen3:4b-skill-select",
		"coding_skill_procedure_model":       "qwen3:4b-skill",
		"coding_fragment_model":              "qwen3-coder:30b-fragment",
		"coding_fragment_correction_model":   "qwen2.5-coder:14b-correction",
	})
	if applied.Fast != "fast" || applied.Reasoning != "reasoning" || applied.Analyze != "analyze" || applied.Response != "respond" || applied.Tagging != "tag" || applied.Search != "search" || applied.Memory != "memory" {
		t.Fatalf("expanded routing not applied: %#v", applied)
	}
	if got := applied.Specialist[specialist.RoleSubtaskExecutorSpecialist]; got != "qwen3-coder:30b" {
		t.Fatalf("executor model=%q, want dedicated model", got)
	}
	if applied.Glue != "qwen2.5-coder:3b" {
		t.Fatalf("glue model=%q", applied.Glue)
	}
	if got := applied.Specialist[specialist.RoleCodingSurfaceStation]; got != "qwen3:4b-surface" {
		t.Fatalf("coding surface model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingProductIdentityStation]; got != "qwen2.5-coder:14b-identity" {
		t.Fatalf("coding product identity model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingRequirementPartitionStation]; got != "qwen2.5-coder:7b-partition" {
		t.Fatalf("coding requirement partition model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingRequirementAdviserStation]; got != "deepseek-r1:8b" {
		t.Fatalf("coding requirement adviser model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingRequirementSplitStation]; got != "qwen2.5-coder:7b-split" {
		t.Fatalf("coding requirement split model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingArtifactHandlingStation]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact handling model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingCapabilityRelationStation]; got != "qwen3:4b-relation" {
		t.Fatalf("coding capability relation model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingSkillSelectionStation]; got != "qwen3:4b-skill-select" {
		t.Fatalf("coding skill selection model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingSkillProcedureStation]; got != "qwen3:4b-skill" {
		t.Fatalf("coding skill procedure model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingFragmentStation]; got != "qwen3-coder:30b-fragment" {
		t.Fatalf("coding fragment model=%q", got)
	}
	if got := applied.Specialist[specialist.RoleCodingFragmentCorrectionStation]; got != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding fragment correction model=%q", got)
	}
}

func TestApplyKeepsGeneralRoutesOutOfDedicatedRoleAuthority(t *testing.T) {
	base := Routing{
		Reasoning: "reasoning-14b",
		Specialist: map[string]string{
			specialist.RolePlannerSpecialist:            "planner-specialist-14b",
			specialist.RoleReviewVerificationSpecialist: "review-specialist-7b",
		},
	}
	applied := Apply(base, Config{
		"planner_model":  "general-planner-4b",
		"analyzer_model": "general-analyzer-4b",
	})
	if applied.Reasoning != "reasoning-14b" {
		t.Fatalf("general route hijacked reasoning route: %#v", applied)
	}
	if applied.Specialist[specialist.RolePlannerSpecialist] != "planner-specialist-14b" {
		t.Fatalf("general planner hijacked dedicated planner specialist: %#v", applied.Specialist)
	}
	if applied.Specialist[specialist.RoleReviewVerificationSpecialist] != "review-specialist-7b" {
		t.Fatalf("general analyzer hijacked dedicated review specialist: %#v", applied.Specialist)
	}
	if applied.Plan != "general-planner-4b" || applied.Analyze != "general-analyzer-4b" {
		t.Fatalf("general routes were not applied to their own fields: %#v", applied)
	}
}
