package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxMechanicalTargetTreeOrdinal = 999

type mechanicalTargetTreePair func(int) (string, string)

func projectMechanicalFocusedTargetTree(
	stackLabel string,
	taskOrdinal int,
	currentPaths []string,
	pair mechanicalTargetTreePair,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 || taskOrdinal > maxMechanicalTargetTreeOrdinal {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s focused target tree requires a frozen task ordinal between 1 and %d",
			stackLabel, maxMechanicalTargetTreeOrdinal,
		)
	}
	if pair == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s focused target tree requires one registered path-pair grammar",
			stackLabel,
		)
	}
	present := make(map[string]struct{}, len(currentPaths))
	for _, artifactPath := range currentPaths {
		present[artifactPath] = struct{}{}
	}
	for ordinal := taskOrdinal; ordinal <= maxMechanicalTargetTreeOrdinal; ordinal++ {
		implementation, verification := pair(ordinal)
		if implementation == "" || verification == "" || implementation == verification {
			return assemblyline.TargetTree{}, fmt.Errorf(
				"%s focused target-tree grammar returned an invalid pair",
				stackLabel,
			)
		}
		if _, exists := present[implementation]; exists {
			continue
		}
		if _, exists := present[verification]; exists {
			continue
		}
		paths := []string{implementation, verification}
		sort.Strings(paths)
		return assemblyline.TargetTree{Paths: paths}, nil
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"%s frozen task %d cannot reserve a free three-digit workload pair",
		stackLabel, taskOrdinal,
	)
}

func projectGoCommandLineFocusedTargetTree(
	taskOrdinal int,
	currentPaths []string,
) (assemblyline.TargetTree, error) {
	return projectMechanicalFocusedTargetTree(
		"Go command-line", taskOrdinal, currentPaths,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("feature%03d.go", ordinal),
				fmt.Sprintf("feature%03d_test.go", ordinal)
		},
	)
}

func projectJavaScriptCommandLineFocusedTargetTree(
	taskOrdinal int,
	currentPaths []string,
) (assemblyline.TargetTree, error) {
	return projectMechanicalFocusedTargetTree(
		"JavaScript command-line", taskOrdinal, currentPaths,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("feature%03d.mjs", ordinal),
				fmt.Sprintf("feature%03d.test.mjs", ordinal)
		},
	)
}

func projectRustCommandLineFocusedTargetTree(
	taskOrdinal int,
	currentPaths []string,
) (assemblyline.TargetTree, error) {
	return projectMechanicalFocusedTargetTree(
		"Rust command-line", taskOrdinal, currentPaths,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("src/feature%03d.rs", ordinal),
				fmt.Sprintf("tests/feature%03d_test.rs", ordinal)
		},
	)
}

func projectJavaCommandLineFocusedTargetTree(
	taskOrdinal int,
	currentPaths []string,
) (assemblyline.TargetTree, error) {
	return projectMechanicalFocusedTargetTree(
		"Java command-line", taskOrdinal, currentPaths,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("Feature%03d.java", ordinal),
				fmt.Sprintf("Feature%03dTest.java", ordinal)
		},
	)
}
