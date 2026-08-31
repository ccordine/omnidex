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
	return validateDirectCodingSingleImplementationTargetTree(stack, target, false)
}

func validateGoCommandLineTargetTree(target assemblyline.TargetTree) error {
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		return err
	}
	return validateDirectCodingSingleImplementationTargetTree(stack, target, true)
}

func validateDirectCodingSingleImplementationTargetTree(
	stack directCodingProjectStack,
	target assemblyline.TargetTree,
	rootOnly bool,
) error {
	if len(target.Paths) != 1 {
		return fmt.Errorf("project stack %s requires exactly one implementation leaf", stack.ID)
	}
	artifactPath := target.Paths[0]
	if rootOnly && path.Dir(artifactPath) != "." {
		return fmt.Errorf("project stack %s requires a root package workload leaf", stack.ID)
	}
	_, kind, err := directCodingArtifactAdapterForTreePath(stack, artifactPath)
	if err != nil {
		return err
	}
	if kind != assemblyline.TargetArtifactImplementation {
		return fmt.Errorf(
			"project stack %s requires one implementation leaf, received %q",
			stack.ID, kind,
		)
	}
	return nil
}
