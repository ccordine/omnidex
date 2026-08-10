package labyrinth

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func Generate(config GeneratorConfig) (GeneratedCase, error) {
	generated, err := generateWithoutSolve(config)
	if err != nil {
		return GeneratedCase{}, err
	}
	transition, verifiedCost, err := verifyScenarioWitness(generated.execution, generated.oracle.Witness)
	if err != nil {
		return GeneratedCase{}, fmt.Errorf("%w: generated witness execution failed: %v", ErrGeneration, err)
	}
	if !transition.Terminal {
		return GeneratedCase{}, fmt.Errorf("%w: generated witness ended before the exact goal", ErrGeneration)
	}
	if verifiedCost != generated.oracle.WitnessCost {
		return GeneratedCase{}, fmt.Errorf("%w: generated witness cost is not exact", ErrGeneration)
	}
	result, solveErr := Solve(generated.execution, SolverBounds{
		MaxStates: config.SolverStateLimit, MaxGroundActions: MaxSolverGroundActions,
	})
	if solveErr != nil && !errors.Is(solveErr, ErrSolverLimit) {
		return GeneratedCase{}, fmt.Errorf("%w: solve generated world: %v", ErrGeneration, solveErr)
	}
	if result.ExpandedStates < 1 {
		return GeneratedCase{}, fmt.Errorf("%w: solver produced no bounded proof evidence", ErrGeneration)
	}
	generated.oracle.ExpandedStates = result.ExpandedStates
	generated.oracle.LowerBound = result.LowerBound
	if solveErr == nil && result.Optimal {
		generated.oracle.Quality = OracleOptimal
		optimal := result.Cost
		generated.oracle.OptimalCost = &optimal
		generated.oracle.LowerBound = optimal
	} else {
		generated.oracle.Quality = OracleWitnessOnly
		generated.oracle.OptimalCost = nil
	}
	if err := generated.oracle.seal(); err != nil {
		return GeneratedCase{}, err
	}
	if err := generated.Validate(); err != nil {
		return GeneratedCase{}, err
	}
	return generated, nil
}

func generateWithoutSolve(config GeneratorConfig) (GeneratedCase, error) {
	if err := config.Validate(); err != nil {
		return GeneratedCase{}, err
	}
	plan := newCausalPlan(config)
	random := newDeterministicRandom(config.Seed)
	world, err := buildGeneratedWorld(config, plan, random)
	if err != nil {
		return GeneratedCase{}, err
	}
	catalog, actions, err := buildActionCatalog(config.GrammarVersion, plan, world.contract)
	if err != nil {
		return GeneratedCase{}, err
	}
	goalPredicate, err := generatedPredicate("state.completed", plan.mainStages[len(plan.mainStages)-1])
	if err != nil {
		return GeneratedCase{}, err
	}
	goalPredicates := []cognition.Predicate{goalPredicate}
	if config.Suite == SuiteMutate || config.Suite == SuiteCombined {
		mutated, mutationErr := generatedPredicate(
			"state.mutated", world.contract.mutationTarget, world.contract.mutationValue,
		)
		if mutationErr != nil {
			return GeneratedCase{}, mutationErr
		}
		goalPredicates = append(goalPredicates, mutated)
	}
	goal, err := cognition.NewGoalExpression(goalPredicates, nil, nil)
	if err != nil {
		return GeneratedCase{}, fmt.Errorf("%w: construct goal: %v", ErrGeneration, err)
	}
	definition, err := NewDefinition(
		catalog, world.entities, generatedPredicateSchemas(world.contract), world.facts, actions, goal,
	)
	if err != nil {
		return GeneratedCase{}, fmt.Errorf("%w: seal generated definition: %v", ErrGeneration, err)
	}
	descriptor := generatedDescriptor(config, world.records, len(world.evidence))
	scenarioID, err := generatedScenarioID(config)
	if err != nil {
		return GeneratedCase{}, err
	}
	scenario, err := NewScenario(scenarioID, definition, descriptor)
	if err != nil {
		return GeneratedCase{}, fmt.Errorf("%w: seal generated scenario: %v", ErrGeneration, err)
	}
	witness, err := buildWitness(plan, world.contract, catalog)
	if err != nil {
		return GeneratedCase{}, err
	}
	oracle := Oracle{
		Schema: OracleSchemaV1, ScenarioID: scenarioID, PublicSHA256: scenario.Ref().SHA256,
		GeneratorVersion: config.GeneratorVersion, GrammarVersion: config.GrammarVersion,
		Seed: config.Seed, DefinitionSHA256: definition.SHA256(), Quality: OracleWitnessOnly,
		Witness: witness, WitnessCost: witnessCost(witness), LowerBound: minimumActionCost(actions),
		RequiredEvidence: world.evidence,
		EvidenceUses:     buildEvidenceUses(world.evidence, witness, world.contract),
		CausalDAG:        plan.dag,
		TaskArchetype:    archetypeForSuite(config.Suite), ExpandedStates: 1,
	}
	generated := GeneratedCase{execution: scenario, public: scenario.PublicArtifact(), oracle: oracle}
	if err := verifyGeneratedCausality(generated); err != nil {
		return GeneratedCase{}, err
	}
	return generated, nil
}

func generatedDescriptor(config GeneratorConfig, records []PublicRecord, evidenceCount int) PublicDescriptor {
	difficulty := config.Difficulty.public()
	difficulty.EvidenceArtifacts = evidenceCount
	return PublicDescriptor{
		Suite: config.Suite, FormatVersion: "public.v1", SurfaceVersion: "symbolic.v1",
		GrammarVersion: config.GrammarVersion,
		Goal:           fmt.Sprintf("Complete the registered %s objective through bounded symbolic actions.", config.Suite),
		Difficulty:     difficulty, Records: append([]PublicRecord(nil), records...),
	}
}

func generatedScenarioID(config GeneratorConfig) (cognition.ScenarioID, error) {
	digest, _, err := digestJSON(struct {
		Format string          `json:"format"`
		Config GeneratorConfig `json:"config"`
	}{"labyrinth-generation.v1", config})
	if err != nil {
		return "", fmt.Errorf("%w: hash generator configuration: %v", ErrGeneration, err)
	}
	return cognition.ScenarioID("scenario-" + digest), nil
}

func minimumActionCost(actions []ActionDefinition) int {
	minimum := actions[0].Cost
	for _, action := range actions[1:] {
		if action.Cost < minimum {
			minimum = action.Cost
		}
	}
	return minimum
}
