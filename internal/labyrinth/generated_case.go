package labyrinth

import (
	"encoding/json"
	"fmt"
)

func (generated GeneratedCase) Validate() error {
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
		generated.oracle.ScenarioID != generated.public.Scenario.ID ||
		generated.oracle.PublicSHA256 != generated.public.Scenario.SHA256 ||
		generated.oracle.DefinitionSHA256 != generated.execution.definitionSHA256 {
		return fmt.Errorf("%w: generated authority components do not match", ErrGeneration)
	}
	return generated.validateCoordinates()
}

func (generated GeneratedCase) ExecutionScenario() Scenario {
	return generated.execution.clone()
}

func (generated GeneratedCase) PublicArtifact() GeneratedScenario {
	return generated.public.clone()
}

func (generated GeneratedCase) PrivateOracle() Oracle {
	return generated.oracle.clone()
}

func (generated GeneratedCase) MarshalPublicJSON() ([]byte, error) {
	return json.Marshal(generated.PublicArtifact())
}

func (generated GeneratedCase) MarshalOracleJSON() ([]byte, error) {
	return json.Marshal(generated.PrivateOracle())
}

func (generated GeneratedCase) MarshalJSON() ([]byte, error) {
	return nil, ErrArtifactSeparation
}

func (scenario Scenario) clone() Scenario {
	return Scenario{
		ref:              scenario.ref,
		definition:       scenario.definition.clone(),
		definitionSHA256: scenario.definitionSHA256,
		descriptor:       scenario.descriptor.clone(),
		artifactCorpus:   scenario.artifactCorpus.clone(),
	}
}
