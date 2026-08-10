package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

type offlineScaleGeneratedFamily struct {
	descriptor labyrinth.ScaleFamilyDescriptor
	byWorld    map[int]labyrinth.GeneratedCase
}

func generateOfflineScaleFamily(
	registration OfflineScalePreregistration,
) (offlineScaleGeneratedFamily, error) {
	if err := registration.Validate(); err != nil {
		return offlineScaleGeneratedFamily{}, err
	}
	if registration.BaseWorkload.Initial == nil {
		return offlineScaleGeneratedFamily{}, fmt.Errorf("offline Scale requires an initial Combined workload")
	}
	generated, descriptor, err := labyrinth.GenerateScaleFamily(
		registration.BaseWorkload.Initial.Generator, registration.WorldSizes,
	)
	if err != nil {
		return offlineScaleGeneratedFamily{}, err
	}
	if len(generated) != len(registration.WorldSizes) || len(descriptor.Cases) != len(generated) {
		return offlineScaleGeneratedFamily{}, fmt.Errorf("offline Scale generator returned an incomplete family")
	}
	byWorld := make(map[int]labyrinth.GeneratedCase, len(generated))
	for index, item := range generated {
		worldSize := descriptor.Cases[index].WorldSize
		if worldSize != registration.WorldSizes[index] {
			return offlineScaleGeneratedFamily{}, fmt.Errorf("offline Scale generator changed world coordinate %d", index+1)
		}
		byWorld[worldSize] = item
	}
	return offlineScaleGeneratedFamily{descriptor: descriptor, byWorld: byWorld}, nil
}

func (family offlineScaleGeneratedFamily) scenario(
	registration OfflineScalePreregistration,
	coordinate OfflineScaleCase,
) (generatedOfflineScenario, error) {
	generated, exists := family.byWorld[coordinate.WorldSize]
	if !exists {
		return generatedOfflineScenario{}, fmt.Errorf("offline Scale world %d was not generated", coordinate.WorldSize)
	}
	spec := registration.BaseWorkload
	initial := *spec.Initial
	initial.CaseID = coordinate.ID
	spec.Initial = &initial
	if err := spec.Validate(); err != nil {
		return generatedOfflineScenario{}, err
	}
	oracle := generated.PrivateOracle()
	result := generatedOfflineScenario{
		spec: spec, scenario: generated.ExecutionScenario(), public: generated.PublicArtifact(),
		suite: SuiteCombined, oracleSHA256: oracle.OracleSHA256,
		taskArchetype: string(oracle.TaskArchetype),
	}
	fixture := MicrogauntletCase{spec: initial, generated: generated}
	result.initial = &fixture
	if result.scenario.Ref() != result.public.Scenario {
		return generatedOfflineScenario{}, fmt.Errorf("offline Scale scenario changed its public authority")
	}
	return result, nil
}
