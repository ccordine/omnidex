package labyrinth

import (
	"errors"
	"reflect"
	"testing"
)

func TestSolverProvesGeneratedOptimalCost(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteUnlock, 27))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Solve(generated.ExecutionScenario(), SolverBounds{MaxStates: 5000, MaxGroundActions: MaxSolverGroundActions})
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.PrivateOracle()
	if !result.Optimal || oracle.Quality != OracleOptimal || oracle.OptimalCost == nil {
		t.Fatalf("solver/oracle did not report exhaustive optimal proof: %#v %#v", result, oracle)
	}
	if result.Cost != *oracle.OptimalCost || result.Cost != oracle.LowerBound {
		t.Fatalf("solver cost=%d oracle optimal=%d lower=%d", result.Cost, *oracle.OptimalCost, oracle.LowerBound)
	}
	if len(oracle.OptimalPlan) != len(result.Actions) || witnessCost(oracle.OptimalPlan) != result.Cost {
		t.Fatalf("sealed optimal plan=%#v solver=%#v", oracle.OptimalPlan, result)
	}
	for index := range result.Actions {
		if !reflect.DeepEqual(oracle.OptimalPlan[index].Request, result.Actions[index]) {
			t.Fatalf("optimal request %d differs from solver result", index)
		}
	}
	transition, replayedCost, err := verifyScenarioWitness(
		generated.ExecutionScenario(), oracle.OptimalPlan,
	)
	if err != nil || !transition.Terminal || replayedCost != result.Cost {
		t.Fatalf("optimal plan replay terminal=%t cost=%d error=%v", transition.Terminal, replayedCost, err)
	}
}

func TestSolverReportsBoundExhaustionLoudly(t *testing.T) {
	t.Parallel()
	generated, err := generateWithoutSolve(testGeneratorConfig(SuiteCombined, 81))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Solve(generated.execution, SolverBounds{MaxStates: 1, MaxGroundActions: MaxSolverGroundActions})
	if !errors.Is(err, ErrSolverLimit) || result.Optimal {
		t.Fatalf("result=%#v error=%v, want explicit non-optimal solver limit", result, err)
	}
}

func TestSolverRejectsGroundActionOverflowLoudly(t *testing.T) {
	t.Parallel()
	generated, err := generateWithoutSolve(testGeneratorConfig(SuiteCombined, 83))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Solve(generated.execution, SolverBounds{MaxStates: 5000, MaxGroundActions: 1})
	if !errors.Is(err, ErrSolverLimit) || result.Optimal {
		t.Fatalf("result=%#v error=%v, want explicit grounded-action limit", result, err)
	}
}

func TestGeneratorMarksBoundedUnprovenCaseWitnessOnly(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteCombined, 82)
	config.SolverStateLimit = 1
	generated, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.PrivateOracle()
	if oracle.Quality != OracleWitnessOnly || oracle.OptimalCost != nil ||
		oracle.OptimalPlan == nil || len(oracle.OptimalPlan) != 0 || oracle.LowerBound < 1 {
		t.Fatalf("bounded oracle = %#v, want explicit witness-only authority", oracle)
	}
	if transition, _, err := VerifyWitness(generated); err != nil || !transition.Terminal {
		t.Fatalf("witness-only case lacks verified witness: transition=%#v error=%v", transition, err)
	}
}

func TestSolverMatchesBruteForceCostOnTinySymbolicWorld(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	result, err := Solve(world.kernel, SolverBounds{MaxStates: 64, MaxGroundActions: 64})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Optimal || result.Cost != 7 || len(result.Actions) != 2 ||
		result.Actions[0].Kind != "enable" || result.Actions[1].Kind != "finish" {
		t.Fatalf("tiny exhaustive result = %#v, want enable+finish at exact cost 7", result)
	}
}
