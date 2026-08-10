package labyrinth

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	ExtendedGeneratorVersionV1 = "extended-generator.v1"
	ExtendedGrammarVersionV1   = "extended-grammar.v1"
	ExtendedOracleSchemaV1     = "labyrinth.extended-private-oracle.v1"
)

type ExtendedGeneratorConfig struct {
	Suite            Suite  `json:"suite"`
	Seed             uint64 `json:"seed"`
	GeneratorVersion string `json:"generator_version"`
	GrammarVersion   string `json:"grammar_version"`
}

type ExtendedRailOutcome string

const (
	ExtendedRailRejected            ExtendedRailOutcome = "rejected_action"
	ExtendedRailIrreversibleDeadEnd ExtendedRailOutcome = "irreversible_dead_end"
)

type ExtendedInvalidRail struct {
	ID            string                      `json:"id"`
	PrefixActions int                         `json:"prefix_actions"`
	Request       cognition.ActionRequest     `json:"request"`
	Outcome       ExtendedRailOutcome         `json:"outcome"`
	FailureCode   cognition.ActionFailureCode `json:"failure_code,omitempty"`
}

type ExtendedOmissionRail struct {
	ID            string                      `json:"id"`
	OmittedAction int                         `json:"omitted_action"`
	OmittedKind   cognition.ActionKind        `json:"omitted_kind"`
	FailureAction int                         `json:"failure_action"`
	FailureCode   cognition.ActionFailureCode `json:"failure_code"`
}

type ExtendedOracle struct {
	Schema           string                 `json:"schema"`
	ScenarioID       cognition.ScenarioID   `json:"scenario_id"`
	PublicSHA256     string                 `json:"public_sha256"`
	OracleSHA256     string                 `json:"oracle_sha256"`
	DefinitionSHA256 string                 `json:"definition_sha256"`
	GeneratorVersion string                 `json:"generator_version"`
	GrammarVersion   string                 `json:"grammar_version"`
	Suite            Suite                  `json:"suite"`
	Seed             uint64                 `json:"seed"`
	TaskArchetype    string                 `json:"task_archetype"`
	Witness          []WitnessAction        `json:"witness"`
	EvidenceUses     []EvidenceUse          `json:"evidence_uses"`
	InvalidRails     []ExtendedInvalidRail  `json:"invalid_rails"`
	OmissionRails    []ExtendedOmissionRail `json:"omission_rails"`
}

type ExtendedCase struct {
	execution Scenario
	public    GeneratedScenario
	oracle    ExtendedOracle
}

type ExtendedOracleRun struct {
	Transitions []cognition.Transition `json:"transitions"`
	Terminal    bool                   `json:"terminal"`
}

func (config ExtendedGeneratorConfig) Validate() error {
	if !extendedLabyrinthSuite(config.Suite) || config.Seed == 0 ||
		config.GeneratorVersion != ExtendedGeneratorVersionV1 ||
		config.GrammarVersion != ExtendedGrammarVersionV1 {
		return fmt.Errorf("%w: extended suite, seed, generator, or grammar is invalid", ErrInvalidGeneratorConfig)
	}
	return nil
}

func (generated ExtendedCase) Validate() error {
	if err := generated.execution.Validate(); err != nil {
		return err
	}
	if err := generated.public.Validate(); err != nil {
		return err
	}
	if err := generated.oracle.Validate(); err != nil {
		return err
	}
	if generated.execution.Ref() != generated.public.Scenario ||
		generated.public.World.Descriptor.Suite != generated.oracle.Suite ||
		generated.oracle.ScenarioID != generated.public.Scenario.ID ||
		generated.oracle.PublicSHA256 != generated.public.Scenario.SHA256 ||
		generated.oracle.DefinitionSHA256 != generated.execution.definitionSHA256 ||
		generated.public.World.Descriptor.Difficulty.WorldSize != len(generated.public.World.Descriptor.Records) ||
		generated.public.World.Descriptor.Difficulty.DecisionDepth != len(generated.oracle.Witness) {
		return fmt.Errorf("%w: extended public, private, and execution authorities differ", ErrGeneration)
	}
	return validateExtendedMotif(generated)
}

func (generated ExtendedCase) ExecutionScenario() Scenario { return generated.execution.clone() }

func (generated ExtendedCase) PublicArtifact() GeneratedScenario { return generated.public.clone() }

func (generated ExtendedCase) PrivateOracle() ExtendedOracle { return generated.oracle.clone() }

func (generated ExtendedCase) MarshalPublicJSON() ([]byte, error) {
	return json.Marshal(generated.PublicArtifact())
}

func (generated ExtendedCase) MarshalOracleJSON() ([]byte, error) {
	return json.Marshal(generated.PrivateOracle())
}

func (generated ExtendedCase) MarshalJSON() ([]byte, error) { return nil, ErrArtifactSeparation }

func extendedLabyrinthSuite(suite Suite) bool {
	switch suite {
	case SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder, SuiteRogue:
		return true
	default:
		return false
	}
}
