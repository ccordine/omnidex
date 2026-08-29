package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingTargetTreeInput(
	request string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	stack directCodingProjectStack,
	existingPaths []string,
	existingDirs []string,
) (assemblyline.TargetTreeInput, error) {
	if !stack.SupportsSurface(specification.Surface) {
		return assemblyline.TargetTreeInput{}, fmt.Errorf(
			"selected project stack %s supports surfaces %s, not %s",
			stack.ID, directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces), specification.Surface,
		)
	}
	if err := assemblyline.ValidateFrozenApplicationWorkloadFor(specification, workload); err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	technicalContext, err := directCodingTreeTechnicalContext(stack)
	if err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	managedPaths := make([]string, 0, len(existingPaths))
	reservedSet := make(map[string]struct{}, len(stack.TargetTreeReservedPaths))
	for _, artifactPath := range stack.TargetTreeReservedPaths {
		reservedSet[artifactPath] = struct{}{}
	}
	for _, artifactPath := range existingPaths {
		if _, err := normalizeDirectCodingPath(artifactPath); err != nil {
			return assemblyline.TargetTreeInput{}, fmt.Errorf("current workspace path: %w", err)
		}
		if _, reserved := reservedSet[artifactPath]; reserved {
			continue
		}
		if _, _, adapterErr := directCodingArtifactAdapterForTreePath(stack, artifactPath); adapterErr == nil {
			managedPaths = append(managedPaths, artifactPath)
		}
	}
	sort.Strings(managedPaths)
	input := assemblyline.TargetTreeInput{
		Objective:        request,
		TechnicalContext: technicalContext,
		Constraints:      stack.TargetTreeConstraints,
		ExistingPaths:    managedPaths,
		ReservedPaths:    append([]string(nil), stack.TargetTreeReservedPaths...),
		ExistingDirs:     append([]string(nil), existingDirs...),
	}
	if input.ExistingPaths == nil {
		input.ExistingPaths = []string{}
	}
	if input.ReservedPaths == nil {
		input.ReservedPaths = []string{}
	}
	if input.ExistingDirs == nil {
		input.ExistingDirs = []string{}
	}
	if err := input.Validate(); err != nil {
		return assemblyline.TargetTreeInput{}, err
	}
	return input, nil
}
