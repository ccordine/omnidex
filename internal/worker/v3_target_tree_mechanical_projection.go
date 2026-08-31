package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type mechanicalTargetTreePath func(int) string

func projectSingleImplementationPath(
	stackLabel string,
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
	implementation mechanicalTargetTreePath,
) (assemblyline.TargetTree, error) {
	if taskOrdinal < 1 {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s focused target tree requires a positive frozen task ordinal",
			stackLabel,
		)
	}
	if implementation == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"%s target tree requires one registered implementation-path grammar", stackLabel,
		)
	}
	// Accepted path authority can reserve a mechanically preferred name, but it
	// cannot veto an independent workload leaf. Scan one more exact filename
	// than the complete reservation set; a reservation at the fixed parent
	// boundary remains a real contradiction for this stack.
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
		"%s has no workload path outside the accepted path authority",
		stackLabel,
	)
}

func projectTypeScriptBrowserCompleteTargetTree(
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"TypeScript browser", 1, occupation,
		func(ordinal int) string {
			return fmt.Sprintf("src/feature%03d.tsx", ordinal)
		},
	)
}

func projectGoCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"Go command-line", taskOrdinal, occupation,
		func(ordinal int) string {
			return fmt.Sprintf("feature%03d.go", ordinal)
		},
	)
}

func projectJavaScriptCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"JavaScript command-line", taskOrdinal, occupation,
		func(ordinal int) string {
			return fmt.Sprintf("feature%03d.mjs", ordinal)
		},
	)
}

func projectRustCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"Rust command-line", taskOrdinal, occupation,
		func(ordinal int) string {
			return fmt.Sprintf("src/feature%03d.rs", ordinal)
		},
	)
}

func projectJavaCommandLineFocusedTargetTree(
	taskOrdinal int,
	occupation directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	return projectSingleImplementationPath(
		"Java command-line", taskOrdinal, occupation,
		func(ordinal int) string {
			return fmt.Sprintf("Feature%03d.java", ordinal)
		},
	)
}
