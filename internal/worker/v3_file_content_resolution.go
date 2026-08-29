package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// buildDirectCodingApplicationFileCoveragePlan consumes only code-retained
// provenance. Inferred single-pair trees cover every frozen task; mechanical
// stacks retain their exact task-to-pair allocation. No model maps files to
// tasks or restates code-owned task identity.
func buildDirectCodingApplicationFileCoveragePlan(
	stack directCodingProjectStack,
	workload assemblyline.FrozenApplicationWorkload,
	target assemblyline.TargetTree,
	taskPaths map[string][]string,
) (assemblyline.ApplicationFileCoveragePlan, error) {
	provenance := make(map[string][]string, len(target.Paths))
	kinds := make(map[string]assemblyline.TargetArtifactKind, len(target.Paths))
	knownPaths := make(map[string]struct{}, len(target.Paths))
	for _, artifactPath := range target.Paths {
		_, kind, err := directCodingArtifactAdapterForTreePath(stack, artifactPath)
		if err != nil {
			return assemblyline.ApplicationFileCoveragePlan{}, err
		}
		knownPaths[artifactPath] = struct{}{}
		kinds[artifactPath] = kind
	}
	for _, task := range workload.Tasks {
		paths, exists := taskPaths[task.ID]
		if !exists || len(paths) == 0 {
			return assemblyline.ApplicationFileCoveragePlan{}, fmt.Errorf(
				"target-tree provenance omits task %s", task.ID,
			)
		}
		seen := make(map[string]struct{}, len(paths))
		for _, artifactPath := range paths {
			if _, exists := knownPaths[artifactPath]; !exists {
				return assemblyline.ApplicationFileCoveragePlan{}, fmt.Errorf(
					"task %s provenance names non-target path %s", task.ID, artifactPath,
				)
			}
			if _, duplicate := seen[artifactPath]; duplicate {
				return assemblyline.ApplicationFileCoveragePlan{}, fmt.Errorf(
					"task %s repeats target path %s", task.ID, artifactPath,
				)
			}
			seen[artifactPath] = struct{}{}
			provenance[artifactPath] = append(provenance[artifactPath], task.ID)
		}
	}
	return assemblyline.NewApplicationFileCoveragePlan(workload, target, provenance, kinds)
}

type directCodingTaskArtifactPair struct {
	ImplementationPath string
	VerificationPath   string
}

// directCodingTaskSinglePair is a stack-specific constraint helper. The
// generic coverage plan deliberately permits plural or implementation-only
// files; stacks that truly require a pair must say so at their own boundary.
func directCodingTaskSinglePair(
	coverage assemblyline.ApplicationFileCoveragePlan,
	taskID string,
) (directCodingTaskArtifactPair, error) {
	files, err := coverage.FilesForTask(taskID)
	if err != nil {
		return directCodingTaskArtifactPair{}, err
	}
	var pair directCodingTaskArtifactPair
	for _, file := range files {
		switch file.Kind {
		case assemblyline.TargetArtifactImplementation:
			if pair.ImplementationPath != "" {
				return directCodingTaskArtifactPair{}, fmt.Errorf(
					"task %s has multiple implementation files in a single-pair stack", taskID,
				)
			}
			pair.ImplementationPath = file.Path
		case assemblyline.TargetArtifactVerification:
			if pair.VerificationPath != "" {
				return directCodingTaskArtifactPair{}, fmt.Errorf(
					"task %s has multiple verification files in a single-pair stack", taskID,
				)
			}
			pair.VerificationPath = file.Path
		}
	}
	if pair.ImplementationPath == "" || pair.VerificationPath == "" {
		return directCodingTaskArtifactPair{}, fmt.Errorf(
			"task %s lacks the implementation/verification pair required by the selected stack", taskID,
		)
	}
	return pair, nil
}
