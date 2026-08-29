package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxDirectCodingFragmentConcurrency = 4

func validateDirectCodingFragmentConcurrency(value int) error {
	if value < 1 || value > maxDirectCodingFragmentConcurrency {
		return fmt.Errorf("direct coding fragment concurrency must be between 1 and %d, received %d", maxDirectCodingFragmentConcurrency, value)
	}
	return nil
}

func directCodingSourceBlueprintHasGeneratedBlocks(blueprint assemblyline.SourceBlueprint) bool {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Generated() {
				return true
			}
		}
	}
	return false
}
