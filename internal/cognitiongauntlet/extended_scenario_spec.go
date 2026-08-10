package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

const ExtendedScenarioSpecSchemaV1 = "omnidex.extended-scenario-spec.v1"

// ExtendedScenarioSpec is public generator authority. It contains no witness,
// oracle, evaluator label, or private world state.
type ExtendedScenarioSpec struct {
	Schema         string                            `json:"schema"`
	CaseID         string                            `json:"case_id"`
	FixtureVersion string                            `json:"fixture_version"`
	Generator      labyrinth.ExtendedGeneratorConfig `json:"generator"`
	Budget         RunBudget                         `json:"budget"`
}

func ResolveExtendedScenarioSpecV1(
	suite Suite,
	seed uint64,
	budget RunBudget,
) (ExtendedScenarioSpec, error) {
	definition, err := RequireExecutableExtendedSuiteV1(suite)
	if err != nil {
		return ExtendedScenarioSpec{}, err
	}
	if definition.Execution != ExecutionScenario &&
		definition.Execution != ExecutionRogueComposition {
		return ExtendedScenarioSpec{}, fmt.Errorf(
			"extended suite %q does not use a generated scenario execution", suite,
		)
	}
	spec := ExtendedScenarioSpec{
		Schema: ExtendedScenarioSpecSchemaV1,
		CaseID: "configured-" + string(suite) + "-v1",
		FixtureVersion: definition.FixtureVersion,
		Generator: labyrinth.ExtendedGeneratorConfig{
			Suite: labyrinth.Suite(suite), Seed: seed,
			GeneratorVersion: labyrinth.ExtendedGeneratorVersionV1,
			GrammarVersion:   labyrinth.ExtendedGrammarVersionV1,
		},
		Budget: budget,
	}
	return spec, spec.Validate()
}

func (spec ExtendedScenarioSpec) Validate() error {
	if spec.Schema != ExtendedScenarioSpecSchemaV1 ||
		spec.FixtureVersion != ExtendedSuiteFixtureVersionV1 {
		return fmt.Errorf("extended scenario spec schema or fixture is invalid")
	}
	if err := requireExact(spec.CaseID, "extended scenario case ID", 256); err != nil {
		return err
	}
	if err := spec.Generator.Validate(); err != nil {
		return err
	}
	definition, err := RequireExecutableExtendedSuiteV1(Suite(spec.Generator.Suite))
	if err != nil {
		return err
	}
	if definition.Execution != ExecutionScenario &&
		definition.Execution != ExecutionRogueComposition {
		return fmt.Errorf("extended scenario spec names a non-scenario suite")
	}
	if err := spec.Budget.Validate(); err != nil {
		return err
	}
	return nil
}

func GenerateExtendedScenario(spec ExtendedScenarioSpec) (labyrinth.ExtendedCase, error) {
	if err := spec.Validate(); err != nil {
		return labyrinth.ExtendedCase{}, err
	}
	generated, err := labyrinth.GenerateExtended(spec.Generator)
	if err != nil {
		return labyrinth.ExtendedCase{}, fmt.Errorf(
			"generate extended scenario %q: %w", spec.CaseID, err,
		)
	}
	witness := generated.PrivateOracle().Witness
	if len(witness) > spec.Budget.ModelCalls || len(witness) > spec.Budget.EnvironmentActions ||
		len(witness)+1 > spec.Budget.RuntimeCycles {
		return labyrinth.ExtendedCase{}, fmt.Errorf(
			"extended scenario %q exceeds its exact runtime budget", spec.CaseID,
		)
	}
	return generated, nil
}
