package worker

import "github.com/gryph/omnidex/internal/assemblyline"

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
