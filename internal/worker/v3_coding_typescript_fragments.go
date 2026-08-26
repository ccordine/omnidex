package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptFragmentJob struct {
	block          assemblyline.SourceBlock
	dialect        string
	tsx            bool
	available      string
	current        string
	repairRegion   *assemblyline.TypeScriptFragmentRepairRegion
	failure        string
	requiredChange string
	repairGuidance string
}

func directCodingTypeScriptAvailableDeclarations(
	block assemblyline.SourceBlock,
	declarations map[string]string,
) (string, error) {
	available := make([]string, 0, len(block.Capabilities))
	for _, capability := range block.Capabilities {
		declaration := strings.TrimSpace(declarations[capability])
		if declaration == "" {
			return "", fmt.Errorf("block %s capability %s has no accepted API", block.ID, capability)
		}
		available = append(available, declaration)
	}
	return strings.Join(available, "\n"), nil
}

func directCodingTypeScriptAcceptedDeclarations(
	blueprint assemblyline.SourceBlueprint,
	generated map[string]string,
) (map[string]string, error) {
	declarations := make(map[string]string)
	waves, err := blueprint.BuildWaves()
	if err != nil {
		return nil, err
	}
	for _, wave := range waves {
		for _, ref := range wave {
			if ref.Block.Generated() && strings.TrimSpace(generated[ref.Block.ID]) == "" {
				return nil, fmt.Errorf("generated block %s has no accepted declaration", ref.Block.ID)
			}
			declarations[ref.Block.ID] = strings.TrimSpace(ref.Block.API)
		}
	}
	return declarations, nil
}

func directCodingSourceBlueprintBlock(
	blueprint assemblyline.SourceBlueprint,
	blockID string,
) (assemblyline.SourceBlock, bool) {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return block, true
			}
		}
	}
	return assemblyline.SourceBlock{}, false
}
