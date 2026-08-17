package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// deriveDirectCodingTargetTreeBindings derives only bindings forced by the accepted
// path topology. It must not ask a model to restate a binding code already
// knows. A multi-artifact topology needs an explicit future coordination task;
// the removed requirement-to-path mapper was neither content planning nor a
// coherent semantic uncertainty.
func deriveDirectCodingTargetTreeBindings(
	specification assemblyline.ApplicationSpecification,
	tree assemblyline.TargetTree,
) (assemblyline.TargetTree, error) {
	stack, err := directCodingProjectStackByID(tree.StackID)
	if err != nil {
		return assemblyline.TargetTree{}, err
	}
	if bindings, forced, err := directCodingForcedTargetTreeBindings(stack, tree.Paths, specification.Requirements); err != nil {
		return assemblyline.TargetTree{}, err
	} else if forced {
		tree.Bindings = bindings
		return tree, nil
	}
	return assemblyline.TargetTree{}, fmt.Errorf(
		"target-tree topology has no deterministic implementation/verification binding; explicit artifact coordination is required before content or source work",
	)
}

// directCodingForcedTargetTreeBindings resolves only the topology that has one
// possible requirement binding: exactly one implementation leaf and exactly
// one verification leaf. Asking a model to choose in that state is illegal;
// each accepted requirement must bind to the sole leaf of each required kind.
func directCodingForcedTargetTreeBindings(
	stack directCodingProjectStack,
	paths []string,
	requirements []assemblyline.Requirement,
) ([]assemblyline.TargetTreeRequirementBinding, bool, error) {
	if len(paths) != 2 || len(requirements) == 0 {
		return nil, false, nil
	}
	var implementationPath string
	var verificationPath string
	for _, filePath := range paths {
		_, kind, err := directCodingArtifactAdapterForTreePath(stack, filePath)
		if err != nil {
			return nil, false, err
		}
		switch kind {
		case assemblyline.TargetArtifactImplementation:
			if implementationPath != "" {
				return nil, false, nil
			}
			implementationPath = filePath
		case assemblyline.TargetArtifactVerification:
			if verificationPath != "" {
				return nil, false, nil
			}
			verificationPath = filePath
		default:
			return nil, false, fmt.Errorf("target-tree file %q has unsupported artifact kind %q", filePath, kind)
		}
	}
	if implementationPath == "" || verificationPath == "" {
		return nil, false, nil
	}
	ids := make([]string, len(requirements))
	for index, requirement := range requirements {
		ids[index] = requirement.ID
	}
	return []assemblyline.TargetTreeRequirementBinding{
		{Path: implementationPath, Kind: assemblyline.TargetArtifactImplementation, RequirementIDs: append([]string(nil), ids...)},
		{Path: verificationPath, Kind: assemblyline.TargetArtifactVerification, RequirementIDs: append([]string(nil), ids...)},
	}, true, nil
}
