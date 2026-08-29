package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingApplicationTaskContexts(
	workload assemblyline.FrozenApplicationWorkload,
) (map[string]assemblyline.ApplicationTaskContext, error) {
	contexts := make(map[string]assemblyline.ApplicationTaskContext, len(workload.Tasks))
	err := executeDirectCodingApplicationWorkload(
		workload,
		func(context assemblyline.ApplicationTaskContext) error {
			if _, duplicate := contexts[context.Task.RequirementID]; duplicate {
				return fmt.Errorf("application workload repeats requirement %s", context.Task.RequirementID)
			}
			contexts[context.Task.RequirementID] = context
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	if len(contexts) != len(workload.Tasks) {
		return nil, fmt.Errorf("application workload context does not cover accepted requirements")
	}
	return contexts, nil
}

func executeDirectCodingApplicationWorkload(
	workload assemblyline.FrozenApplicationWorkload,
	execute func(assemblyline.ApplicationTaskContext) error,
) error {
	if execute == nil {
		return fmt.Errorf("application workload executor requires one task callback")
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(workload); err != nil {
		return err
	}
	for _, task := range workload.Tasks {
		context, err := assemblyline.ProjectApplicationTaskContext(workload, task.ID)
		if err != nil {
			return err
		}
		if err := execute(context); err != nil {
			return fmt.Errorf("execute application workload task %s: %w", task.ID, err)
		}
	}
	return nil
}

func compileDirectCodingApplicationTaskBehavior(
	context assemblyline.ApplicationTaskContext,
	capabilities []directCodingCapabilityBinding,
) (string, error) {
	if context.Surface == assemblyline.ApplicationSurfaceUnsupported ||
		strings.TrimSpace(string(context.Surface)) == "" ||
		strings.TrimSpace(context.ProductQuote) == "" ||
		strings.TrimSpace(context.Task.RequirementQuote) == "" {
		return "", fmt.Errorf("application task context lacks one exact accepted requirement")
	}
	parts := []string{
		"Authoritative delivery surface: " + string(context.Surface),
		"Exact user requirement: " + context.Task.RequirementQuote,
	}
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		purpose := strings.TrimSpace(capability.Purpose)
		if purpose == "" || purpose != capability.Purpose || capability.RequirementID == context.Task.RequirementID {
			return "", fmt.Errorf("application task has an invalid direct capability projection")
		}
		if _, duplicate := seen[capability.RequirementID]; duplicate {
			return "", fmt.Errorf("application task repeats direct capability requirement %s", capability.RequirementID)
		}
		seen[capability.RequirementID] = struct{}{}
		parts = append(parts, "Direct capability "+capability.CapabilityID+": "+purpose)
	}
	return strings.Join(parts, "\n"), nil
}
