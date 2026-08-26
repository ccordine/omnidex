package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxPHPServiceFeatureNumber = 999

func projectGenericPHPServiceFocusedTargetTree(
	taskOrdinal int,
	currentPaths []string,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"PHP HTTP focused target tree requires a positive frozen task ordinal",
		)
	}
	present := make(map[string]struct{}, len(currentPaths))
	for _, artifactPath := range currentPaths {
		present[artifactPath] = struct{}{}
	}
	for feature := 1; feature <= maxPHPServiceFeatureNumber; feature++ {
		implementation := fmt.Sprintf("src/Feature%03d.php", feature)
		verification := fmt.Sprintf("tests/Feature%03dTest.php", feature)
		if _, exists := present[implementation]; exists {
			continue
		}
		if _, exists := present[verification]; exists {
			continue
		}
		return assemblyline.TargetTree{Paths: []string{implementation, verification}}, nil
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"PHP HTTP frozen task %d cannot reserve a free three-digit feature pair",
		taskOrdinal,
	)
}
