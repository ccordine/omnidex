package worker

import (
	"fmt"
	"path"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func validateTypeScriptBrowserTargetTree(target assemblyline.TargetTree) error {
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		return err
	}
	return validateDirectCodingSinglePairTargetTree(stack, target, false)
}

func validateGoCommandLineTargetTree(target assemblyline.TargetTree) error {
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		return err
	}
	return validateDirectCodingSinglePairTargetTree(stack, target, true)
}

func validateDirectCodingSinglePairTargetTree(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
	rootOnly bool,
) error {
	if len(target.Paths) != 2 {
		return fmt.Errorf(
			"project stack %s requires exactly one implementation leaf and one verification leaf",
			stack.ID,
		)
	}
	implementations, verifications := 0, 0
	for _, artifactPath := range target.Paths {
		if rootOnly && path.Dir(artifactPath) != "." {
			return fmt.Errorf("project stack %s requires root package workload leaves", stack.ID)
		}
		_, kind, err := directCodingArtifactAdapterForTreePath(stack, artifactPath)
		if err != nil {
			return err
		}
		switch kind {
		case assemblyline.TargetArtifactImplementation:
			implementations++
		case assemblyline.TargetArtifactVerification:
			verifications++
		default:
			return fmt.Errorf("target-tree path %q has unsupported artifact kind %q", artifactPath, kind)
		}
	}
	if implementations != 1 || verifications != 1 {
		return fmt.Errorf(
			"project stack %s requires exactly one implementation leaf and one verification leaf",
			stack.ID,
		)
	}
	return nil
}
