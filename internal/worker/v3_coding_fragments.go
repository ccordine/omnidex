package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxDirectCodingFragmentConcurrency = 4

type directCodingFragmentResult struct {
	blockID string
	source  string
	err     error
}

func (s *directCodingSession) generateProgramFragments(program directCodingProgram) (map[string]string, error) {
	if !directCodingTypeScriptBlueprintHasGeneratedBlocks(program.TypeScript) {
		return map[string]string{}, nil
	}
	modelName, err := s.workerModel(station.CodingFragment)
	if err != nil {
		return nil, err
	}
	correctionModel, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return nil, err
	}
	runtime := directCodingWorkerRuntime(s)
	runtime.CorrectionModel = correctionModel
	return generateDirectCodingTypeScriptFragments(runtime, modelName, program.TypeScript)
}

func validateDirectCodingFragmentConcurrency(value int) error {
	if value < 1 || value > maxDirectCodingFragmentConcurrency {
		return fmt.Errorf("direct coding fragment concurrency must be between 1 and %d, received %d", maxDirectCodingFragmentConcurrency, value)
	}
	return nil
}

func directCodingTypeScriptBlueprintHasGeneratedBlocks(blueprint assemblyline.TypeScriptBlueprint) bool {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Generated() {
				return true
			}
		}
	}
	return false
}
