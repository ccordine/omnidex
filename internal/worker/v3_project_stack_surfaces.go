package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

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

func directCodingProjectStackSurfaceSummary(
	surfaces []assemblyline.ApplicationSurface,
) string {
	values := make([]string, len(surfaces))
	for index, surface := range surfaces {
		values[index] = string(surface)
	}
	return strings.Join(values, ", ")
}
