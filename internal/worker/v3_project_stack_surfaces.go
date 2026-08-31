package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var directCodingApplicationSurfaces = []assemblyline.ApplicationSurface{
	assemblyline.ApplicationSurfaceBrowser,
	assemblyline.ApplicationSurfaceCommandLine,
}

func (stack directCodingProjectStack) SupportsSurface(
	surface assemblyline.ApplicationSurface,
) bool {
	for _, supported := range stack.SupportedSurfaces {
		if supported == surface {
			return true
		}
	}
	return false
}

func (stack directCodingProjectStack) IsDefaultForSurface(
	surface assemblyline.ApplicationSurface,
) bool {
	for _, registered := range stack.DefaultSurfaces {
		if registered == surface {
			return true
		}
	}
	return false
}

func validateDirectCodingProjectStackSurface(
	stackID string,
	surface assemblyline.ApplicationSurface,
) error {
	stack, err := directCodingProjectStackByID(stackID)
	if err != nil {
		return err
	}
	if !stack.SupportsSurface(surface) {
		return fmt.Errorf(
			"project stack %s supports surfaces %s, not %s",
			stack.ID, directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces), surface,
		)
	}
	return nil
}

func validateDirectCodingProjectStackSurfaceSet(stack directCodingProjectStack) error {
	if err := validateDirectCodingSurfaceSet(
		stack.ID, "supported", stack.SupportedSurfaces, true,
	); err != nil {
		return err
	}
	if err := validateDirectCodingSurfaceSet(
		stack.ID, "default", stack.DefaultSurfaces, false,
	); err != nil {
		return err
	}
	for _, surface := range stack.DefaultSurfaces {
		if !stack.SupportsSurface(surface) {
			return fmt.Errorf(
				"project stack %s default surface %s is not supported", stack.ID, surface,
			)
		}
	}
	return nil
}

func validateDirectCodingSurfaceSet(
	stackID string,
	label string,
	surfaces []assemblyline.ApplicationSurface,
	required bool,
) error {
	if required && len(surfaces) == 0 {
		return fmt.Errorf("project stack %s requires at least one supported surface", stackID)
	}
	last := -1
	for _, surface := range surfaces {
		rank := directCodingApplicationSurfaceRank(surface)
		if rank < 0 {
			return fmt.Errorf("project stack %s has unsupported %s surface %q", stackID, label, surface)
		}
		if rank <= last {
			return fmt.Errorf("project stack %s %s surfaces are duplicated or unordered", stackID, label)
		}
		last = rank
	}
	return nil
}

func validateDirectCodingProjectStackDefaults(stacks []directCodingProjectStack) error {
	for _, surface := range directCodingApplicationSurfaces {
		count := 0
		for _, stack := range stacks {
			if stack.IsDefaultForSurface(surface) {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf(
				"application surface %s has %d default project stacks; exactly one is required",
				surface, count,
			)
		}
	}
	return nil
}

func directCodingApplicationSurfaceRank(surface assemblyline.ApplicationSurface) int {
	for index, candidate := range directCodingApplicationSurfaces {
		if candidate == surface {
			return index
		}
	}
	return -1
}

func directCodingProjectStackSurfaceSummary(
	surfaces []assemblyline.ApplicationSurface,
) string {
	values := make([]string, len(surfaces))
	for index, surface := range surfaces {
		values[index] = string(surface)
	}
	return strings.Join(values, ", ")
}
