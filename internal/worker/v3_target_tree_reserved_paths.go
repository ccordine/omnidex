package worker

import "fmt"

func validateDirectCodingTargetTreeReservedPaths(
	stack directCodingProjectStack,
	adapters map[string]directCodingArtifactAdapter,
	targetAdapters map[string]struct{},
) error {
	previous := ""
	for index, artifactPath := range stack.TargetTreeReservedPaths {
		normalized, err := normalizeDirectCodingPath(artifactPath)
		if err != nil || normalized != artifactPath {
			return fmt.Errorf(
				"project stack %s target-tree reserved path %d is not normalized",
				stack.ID, index,
			)
		}
		if previous != "" && artifactPath <= previous {
			return fmt.Errorf(
				"project stack %s target-tree reserved paths are duplicated or unordered",
				stack.ID,
			)
		}
		matches := 0
		for adapterID := range targetAdapters {
			adapter, exists := adapters[adapterID]
			if !exists {
				return fmt.Errorf(
					"project stack %s target-tree adapter %s is not registered",
					stack.ID, adapterID,
				)
			}
			if _, recognized := adapter.Recognize(artifactPath); recognized {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf(
				"project stack %s target-tree reserved path %q matches %d target adapters",
				stack.ID, artifactPath, matches,
			)
		}
		previous = artifactPath
	}
	return nil
}
