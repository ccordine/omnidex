package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxMechanicalTargetTreeOrdinal = 999

type mechanicalTargetTreePair func(int) (string, string)
type mechanicalTargetTreePath func(int) string

func projectMechanicalCompleteTargetTree(
	stackLabel string,
	occupation directCodingTargetTreeOccupation,
	pair mechanicalTargetTreePair,
) (assemblyline.TargetTree, error) {
	return allocateMechanicalTargetTreePair(stackLabel, 1, occupation, pair)
}

func projectMechanicalFocusedTargetTree(
	stackLabel string,
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
	pair mechanicalTargetTreePair,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 || taskOrdinal > maxMechanicalTargetTreeOrdinal {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s focused target tree requires a frozen task ordinal between 1 and %d",
			stackLabel, maxMechanicalTargetTreeOrdinal,
		)
	}
	return allocateMechanicalTargetTreePair(stackLabel, taskOrdinal, occupation, pair)
}

func allocateMechanicalTargetTreePair(
	stackLabel string,
	startOrdinal int,
	occupation directCodingTargetTreeOccupation,
	pair mechanicalTargetTreePair,
) (assemblyline.TargetTree, error) {
	if pair == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s target tree requires one registered path-pair grammar", stackLabel,
		)
	}
	for ordinal := startOrdinal; ordinal <= maxMechanicalTargetTreeOrdinal; ordinal++ {
		implementation, verification := pair(ordinal)
		paths := []string{implementation, verification}
		if implementation == verification {
			return assemblyline.TargetTree{}, fmt.Errorf(
				"%s target-tree grammar returned one path for two authorities", stackLabel,
			)
		}
		available, err := directCodingTargetTreePathsAvailable(paths, occupation)
		if err != nil {
			return assemblyline.TargetTree{}, fmt.Errorf(
				"%s target-tree grammar returned an invalid pair: %w", stackLabel, err,
			)
		}
		if !available {
			continue
		}
		sort.Strings(paths)
		return assemblyline.TargetTree{Paths: paths}, nil
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"%s cannot reserve a free three-digit workload pair starting at %d",
		stackLabel, startOrdinal,
	)
}

func projectTypeScriptBrowserCompleteTargetTree(
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectMechanicalCompleteTargetTree(
		"TypeScript browser", occupation,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("src/feature%03d.tsx", ordinal),
				fmt.Sprintf("src/feature%03d.test.tsx", ordinal)
		},
	)
}

func projectGoCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectMechanicalFocusedTargetTree(
		"Go command-line", taskOrdinal, occupation,
		func(ordinal int) (string, string) {
			return fmt.Sprintf("feature%03d.go", ordinal),
				fmt.Sprintf("feature%03d_test.go", ordinal)
		},
	)
}

func projectJavaScriptCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"JavaScript command-line", taskOrdinal, occupation,
		func(ordinal int) string { return fmt.Sprintf("feature%03d.mjs", ordinal) },
	)
}

func projectRustCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"Rust command-line", taskOrdinal, occupation,
		func(ordinal int) string { return fmt.Sprintf("src/feature%03d.rs", ordinal) },
	)
}

func projectJavaCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"Java command-line", taskOrdinal, occupation,
		func(ordinal int) string { return fmt.Sprintf("Feature%03d.java", ordinal) },
	)
}

func projectSingleImplementationPath(
	stackLabel string,
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
	implementation mechanicalTargetTreePath,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 || implementation == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s target tree requires a positive task ordinal and path grammar", stackLabel,
		)
	}
	for ordinal, remaining := taskOrdinal, len(occupation.FilePaths)+1; remaining > 0; ordinal, remaining = ordinal+1, remaining-1 {
		artifactPath := implementation(ordinal)
		available, err := directCodingTargetTreePathsAvailable(
			[]string{artifactPath}, occupation,
		)
		if err != nil {
			return assemblyline.TargetTree{}, fmt.Errorf(
				"%s target-tree grammar returned an invalid implementation path: %w",
				stackLabel, err,
			)
		}
		if available {
			return assemblyline.TargetTree{Paths: []string{artifactPath}}, nil
		}
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"%s has no workload path outside the accepted path authority", stackLabel,
	)
}
