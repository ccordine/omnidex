package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

const OfflineScenarioSpecSchemaV1 = "omnidex.offline-scenario-spec.v1"

type OfflineScenarioKind string

const (
	OfflineScenarioInitial  OfflineScenarioKind = "initial"
	OfflineScenarioExtended OfflineScenarioKind = "extended"
)

// OfflineScenarioSpec is the sole generator authority for ordinary offline
// scenario runs. Dedicated Resume, Scale, Transfer, and Rogue rails cannot be
// represented by this type.
type OfflineScenarioSpec struct {
	Schema   string                `json:"schema"`
	Kind     OfflineScenarioKind   `json:"kind"`
	Initial  *MicrogauntletSpec    `json:"initial,omitempty"`
	Extended *ExtendedScenarioSpec `json:"extended,omitempty"`
}

func ResolveOfflineScenarioSpecV1(
	suite Suite,
	seed uint64,
	budget RunBudget,
) (OfflineScenarioSpec, error) {
	if seed == 0 {
		return OfflineScenarioSpec{}, fmt.Errorf("offline scenario seed must be positive")
	}
	switch suite {
	case SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined:
		initial, err := configuredInitialScenarioSpec(suite, seed, budget)
		if err != nil {
			return OfflineScenarioSpec{}, err
		}
		result := OfflineScenarioSpec{
			Schema: OfflineScenarioSpecSchemaV1, Kind: OfflineScenarioInitial,
			Initial: &initial,
		}
		return result, result.Validate()
	case SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder:
		// Routed below through the only registered extended scenario authority.
	default:
		definition, err := ExtendedSuiteV1(suite)
		if err != nil {
			return OfflineScenarioSpec{}, err
		}
		return OfflineScenarioSpec{}, fmt.Errorf(
			"offline suite %q requires its dedicated %s execution authority",
			suite, definition.Execution,
		)
	}
	definition, err := RequireExecutableExtendedSuiteV1(suite)
	if err != nil {
		return OfflineScenarioSpec{}, err
	}
	if definition.Execution != ExecutionScenario {
		return OfflineScenarioSpec{}, fmt.Errorf(
			"offline suite %q requires its dedicated %s execution authority",
			suite, definition.Execution,
		)
	}
	extended, err := ResolveExtendedScenarioSpecV1(suite, seed, budget)
	if err != nil {
		return OfflineScenarioSpec{}, err
	}
	result := OfflineScenarioSpec{
		Schema: OfflineScenarioSpecSchemaV1, Kind: OfflineScenarioExtended,
		Extended: &extended,
	}
	return result, result.Validate()
}

func (spec OfflineScenarioSpec) Validate() error {
	if spec.Schema != OfflineScenarioSpecSchemaV1 {
		return fmt.Errorf("offline scenario spec schema is invalid")
	}
	switch spec.Kind {
	case OfflineScenarioInitial:
		if spec.Initial == nil || spec.Extended != nil {
			return fmt.Errorf("initial offline scenario requires exactly one initial authority")
		}
		return spec.Initial.Validate()
	case OfflineScenarioExtended:
		if spec.Initial != nil || spec.Extended == nil {
			return fmt.Errorf("extended offline scenario requires exactly one extended authority")
		}
		if err := spec.Extended.Validate(); err != nil {
			return err
		}
		definition, err := RequireExecutableExtendedSuiteV1(Suite(spec.Extended.Generator.Suite))
		if err != nil {
			return err
		}
		if definition.Execution != ExecutionScenario {
			return fmt.Errorf("extended offline scenario uses a dedicated execution authority")
		}
		return nil
	default:
		return fmt.Errorf("offline scenario kind %q is not registered", spec.Kind)
	}
}

func (spec OfflineScenarioSpec) Suite() Suite {
	if spec.Initial != nil {
		return Suite(spec.Initial.Generator.Suite)
	}
	if spec.Extended != nil {
		return Suite(spec.Extended.Generator.Suite)
	}
	return ""
}

func (spec OfflineScenarioSpec) Seed() uint64 {
	if spec.Initial != nil {
		return spec.Initial.Generator.Seed
	}
	if spec.Extended != nil {
		return spec.Extended.Generator.Seed
	}
	return 0
}

func (spec OfflineScenarioSpec) Budget() RunBudget {
	if spec.Initial != nil {
		return spec.Initial.Budget
	}
	if spec.Extended != nil {
		return spec.Extended.Budget
	}
	return RunBudget{}
}

func (spec OfflineScenarioSpec) CaseID() string {
	if spec.Initial != nil {
		return spec.Initial.CaseID
	}
	if spec.Extended != nil {
		return spec.Extended.CaseID
	}
	return ""
}

func configuredInitialScenarioSpec(
	suite Suite,
	seed uint64,
	budget RunBudget,
) (MicrogauntletSpec, error) {
	spec, err := initialMicrogauntletSpec(suite)
	if err != nil {
		return MicrogauntletSpec{}, err
	}
	spec.CaseID = "configured-" + string(suite) + "-v1"
	spec.Generator.Seed = seed
	spec.Budget = budget
	return spec, spec.Validate()
}

type generatedOfflineScenario struct {
	spec          OfflineScenarioSpec
	scenario      labyrinth.Scenario
	public        labyrinth.GeneratedScenario
	suite         Suite
	oracleSHA256  string
	taskArchetype string
	initial       *MicrogauntletCase
	extended      *labyrinth.ExtendedCase
}

func generateOfflineScenario(spec OfflineScenarioSpec) (generatedOfflineScenario, error) {
	if err := spec.Validate(); err != nil {
		return generatedOfflineScenario{}, err
	}
	result := generatedOfflineScenario{spec: spec, suite: spec.Suite()}
	switch spec.Kind {
	case OfflineScenarioInitial:
		fixture, err := GenerateMicrogauntlet(*spec.Initial)
		if err != nil {
			return generatedOfflineScenario{}, err
		}
		oracle := fixture.generated.PrivateOracle()
		result.initial = &fixture
		result.scenario = fixture.SealedEnvironmentScenario()
		result.public = fixture.PublicArtifact()
		result.oracleSHA256 = oracle.OracleSHA256
		result.taskArchetype = string(oracle.TaskArchetype)
	case OfflineScenarioExtended:
		fixture, err := GenerateExtendedScenario(*spec.Extended)
		if err != nil {
			return generatedOfflineScenario{}, err
		}
		oracle := fixture.PrivateOracle()
		result.extended = &fixture
		result.scenario = fixture.ExecutionScenario()
		result.public = fixture.PublicArtifact()
		result.oracleSHA256 = oracle.OracleSHA256
		result.taskArchetype = oracle.TaskArchetype
	}
	if result.scenario.Ref() != result.public.Scenario || result.oracleSHA256 == "" {
		return generatedOfflineScenario{}, fmt.Errorf("offline generated scenario authority is inconsistent")
	}
	return result, nil
}
