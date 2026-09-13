package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func directCodingGoTaskAcceptanceName(program directCodingProgram, taskID string) (string, error) {
	blockID, err := directCodingTaskBlockIDByRole(program.Source, taskID, assemblyline.SourceBlockTaskImplementation)
	if err != nil {
		return "", err
	}
	block, exists := directCodingSourceBlueprintBlock(program.Source, blockID)
	if !exists {
		return "", fmt.Errorf("Go task %s implementation block %s is absent", taskID, blockID)
	}
	compiled, err := gofragment.CompileNewFunctionSignature(block.Signature)
	if err != nil {
		return "", fmt.Errorf("compile Go task %s implementation signature: %w", taskID, err)
	}
	return "Test" + compiled.Name, nil
}
