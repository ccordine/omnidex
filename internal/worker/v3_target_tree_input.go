package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingTargetTreeInput(
	stack directCodingProjectStack,
) assemblyline.TargetTreeInput {
	return assemblyline.TargetTreeInput{
		Constraints:      stack.TargetTreeConstraints,
		ExistingPaths:    []string{},
		ReservedPaths:    append([]string(nil), stack.TargetTreeReservedPaths...),
		ExistingDirs:     []string{},
	}
}
