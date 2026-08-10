package labyrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestHiddenDefinitionChangePreservesPublicAuthorityButChangesOracle(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteRecall, 731))
	if err != nil {
		t.Fatal(err)
	}
	original := generated.ExecutionScenario()
	hidden, err := generatedPredicate("state.completed", "stage-000")
	if err != nil {
		t.Fatal(err)
	}
	changedFacts := append(clonePredicates(original.definition.initialFacts), hidden)
	changedDefinition, err := NewDefinition(
		original.definition.catalog, original.definition.entities,
		original.definition.predicateSchemas, changedFacts,
		original.definition.actions, original.definition.goal,
	)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := NewScenario(original.ref.ID, changedDefinition, original.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if original.Ref() != changed.Ref() {
		t.Fatal("a hidden-only state change altered the public scenario authority")
	}
	originalPublic, _ := json.Marshal(original)
	changedPublic, _ := json.Marshal(changed)
	if !bytes.Equal(originalPublic, changedPublic) {
		t.Fatal("a hidden-only state change altered public serialization")
	}
	originalOracle := generated.PrivateOracle()
	changedOracle := originalOracle.clone()
	changedOracle.DefinitionSHA256 = changedDefinition.SHA256()
	changedOracle.OracleSHA256 = ""
	if err := changedOracle.seal(); err != nil {
		t.Fatal(err)
	}
	if originalOracle.OracleSHA256 == changedOracle.OracleSHA256 ||
		originalOracle.DefinitionSHA256 == changedOracle.DefinitionSHA256 {
		t.Fatal("private oracle did not distinguish the changed hidden definition")
	}

	tampered := changed.clone()
	tampered.definitionSHA256 = original.definitionSHA256
	_, err = NewEnvironment(tampered, cognition.EpisodeRef{ID: "wrong-hidden-host"}, func(
		_ context.Context, _ cognition.AttemptRef,
	) error {
		return nil
	})
	if !errors.Is(err, cognition.ErrInvalidScenario) {
		t.Fatalf("environment error = %v, want invalid hidden definition authority", err)
	}
}

func TestOracleRejectsCyclicCausalAuthority(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteUnlock, 919))
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.PrivateOracle()
	oracle.CausalDAG = []CausalEdge{
		{From: "stage-000", To: "stage-001"},
		{From: "stage-001", To: "stage-000"},
	}
	oracle.OracleSHA256 = ""
	if err := oracle.seal(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("error = %v, want cyclic oracle rejection", err)
	}
}
