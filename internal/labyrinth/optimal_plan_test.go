package labyrinth

import (
	"errors"
	"reflect"
	"testing"
)

func TestOptimalOraclePlanIsHashBoundDeepClonedAndReplayRequired(t *testing.T) {
	generated, err := Generate(testGeneratorConfig(SuiteCombined, 94_501))
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.PrivateOracle()
	if oracle.Quality != OracleOptimal || len(oracle.OptimalPlan) < 2 {
		t.Fatalf("oracle lacks a nontrivial optimal plan: %#v", oracle)
	}

	changedIdentity := oracle.clone()
	changedIdentity.OptimalPlan[0].ID = "changed-optimal-action"
	changedIdentity.OracleSHA256 = ""
	if err := changedIdentity.seal(); err != nil {
		t.Fatal(err)
	}
	if changedIdentity.OracleSHA256 == oracle.OracleSHA256 {
		t.Fatal("oracle hash does not bind the exact optimal plan")
	}

	returned := generated.PrivateOracle()
	returned.OptimalPlan[0].ID = "mutated-returned-plan"
	if reflect.DeepEqual(returned.OptimalPlan, generated.PrivateOracle().OptimalPlan) {
		t.Fatal("private oracle getter did not deep-clone its optimal plan")
	}

	reordered := oracle.clone()
	for left, right := 0, len(reordered.OptimalPlan)-1; left < right; left, right = left+1, right-1 {
		reordered.OptimalPlan[left], reordered.OptimalPlan[right] =
			reordered.OptimalPlan[right], reordered.OptimalPlan[left]
	}
	reordered.OracleSHA256 = ""
	if err := reordered.seal(); err != nil {
		t.Fatalf("structurally valid reordered plan did not seal: %v", err)
	}
	tampered := generated
	tampered.oracle = reordered
	if err := tampered.Validate(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("non-replayable optimal plan error=%v", err)
	}

	missing := oracle.clone()
	missing.OptimalPlan = nil
	missing.OracleSHA256 = ""
	if err := missing.seal(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("missing optimal plan error=%v", err)
	}
	wrongCost := oracle.clone()
	wrongCost.OptimalPlan[0].Cost++
	wrongCost.OracleSHA256 = ""
	if err := wrongCost.seal(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("wrong optimal plan cost error=%v", err)
	}
}
