package modelconfig

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestCodingRequirementResultRelationModelRoutingIsExplicitAndIndependent(t *testing.T) {
	t.Parallel()
	routing := Config{
		"coding_requirements_model":                "requirements-model",
		"coding_requirement_result_relation_model": "result-model",
	}.Routing()
	if got := routing.Stations[station.CodingRequirementResultRelation]; got != "result-model" {
		t.Fatalf("result-relation station model=%q", got)
	}
	if got := routing.Stations[station.CodingRequirements]; got != "requirements-model" {
		t.Fatalf("requirements station model=%q", got)
	}
	if got := routing.Stations[station.CodingProjectStackConstraint]; got != "requirements-model" {
		t.Fatalf("project-stack station model=%q", got)
	}

	withoutResultRelation := Config{
		"coding_requirements_model": "requirements-model",
	}.Routing()
	if _, exists := withoutResultRelation.Stations[station.CodingRequirementResultRelation]; exists {
		t.Fatal("result-relation station silently fell back to the requirements model")
	}
}

func TestCodingRequirementResultRelationFieldOwnsOneEnvironmentKey(t *testing.T) {
	t.Parallel()
	for _, field := range RegisteredFields() {
		if field.Key != "coding_requirement_result_relation_model" {
			continue
		}
		if len(field.EnvKeys) != 1 || field.EnvKeys[0] != "OMNI_CODING_REQUIREMENT_RESULT_RELATION_MODEL" {
			t.Fatalf("result-relation field env keys=%v", field.EnvKeys)
		}
		return
	}
	t.Fatal("coding requirement result-relation field is not registered")
}
