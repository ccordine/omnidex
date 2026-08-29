package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxPHPServiceFeatureNumber = 999

func projectGenericPHPServiceFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"PHP HTTP focused target tree requires a positive frozen task ordinal",
		)
	}
	for feature := 1; feature <= maxPHPServiceFeatureNumber; feature++ {
		implementation := fmt.Sprintf("src/Feature%03d.php", feature)
		verification := fmt.Sprintf("tests/Feature%03dTest.php", feature)
		available, err := directCodingTargetTreePairAvailable(
			[]string{implementation, verification}, occupation,
		)
		if err != nil {
			return assemblyline.TargetTree{}, fmt.Errorf(
				"PHP HTTP target-tree grammar returned an invalid pair: %w", err,
			)
		}
		if !available {
			continue
		}
		return assemblyline.TargetTree{Paths: []string{implementation, verification}}, nil
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"PHP HTTP frozen task %d cannot reserve a free three-digit feature pair",
		taskOrdinal,
	)
}
