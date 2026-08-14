package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func applicationWorkloadInput(
	specification assemblyline.ApplicationSpecification,
) assemblyline.ApplicationWorkloadDraftInput {
	return assemblyline.ApplicationWorkloadDraftInput{
		Surface:      specification.Surface,
		ProductQuote: specification.ProductQuote,
		Requirements: append([]assemblyline.Requirement(nil), specification.Requirements...),
	}
}

func directCodingApplicationTaskContexts(
	input assemblyline.ApplicationWorkloadDraftInput,
	frozen assemblyline.FrozenApplicationWorkload,
) (map[string]assemblyline.ApplicationTaskContext, error) {
	contexts := make(map[string]assemblyline.ApplicationTaskContext, len(frozen.Tasks))
	err := executeDirectCodingApplicationWorkload(
		input, frozen,
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
	if len(contexts) != len(input.Requirements) {
		return nil, fmt.Errorf("application workload context does not cover accepted requirements")
	}
	return contexts, nil
}

func executeDirectCodingApplicationWorkload(
	input assemblyline.ApplicationWorkloadDraftInput,
	frozen assemblyline.FrozenApplicationWorkload,
	execute func(assemblyline.ApplicationTaskContext) error,
) error {
	if execute == nil {
		return fmt.Errorf("application workload executor requires one task callback")
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		return err
	}
	waves, err := assemblyline.BuildApplicationWorkloadWaves(input, frozen)
	if err != nil {
		return err
	}
	for _, wave := range waves {
		for _, taskID := range wave {
			context, projectErr := assemblyline.ProjectApplicationTaskContext(input, frozen, taskID)
			if projectErr != nil {
				return projectErr
			}
			if executeErr := execute(context); executeErr != nil {
				return fmt.Errorf("execute application workload task %s: %w", taskID, executeErr)
			}
		}
	}
	return nil
}

func compileDirectCodingApplicationTaskBehavior(
	context assemblyline.ApplicationTaskContext,
	capabilities []directCodingCapabilityBinding,
) (string, error) {
	if len(context.Dependencies) != 0 {
		return "", fmt.Errorf("scheduler dependencies cannot grant coding-model context")
	}
	if context.Surface == assemblyline.ApplicationSurfaceUnsupported ||
		strings.TrimSpace(string(context.Surface)) == "" ||
		strings.TrimSpace(context.ProductQuote) == "" ||
		strings.TrimSpace(context.Task.RequirementQuote) == "" ||
		strings.TrimSpace(context.Task.Objective) == "" ||
		len(context.Task.RequiredBehaviors) < 1 ||
		len(context.Task.AcceptanceCriteria) < 1 {
		return "", fmt.Errorf("application task context lacks one complete executable objective")
	}
	parts := []string{
		"Delivery surface: " + string(context.Surface),
		"Product objective: " + context.ProductQuote,
		"Exact requirement: " + context.Task.RequirementQuote,
		"Concrete objective: " + context.Task.Objective,
	}
	for index, behavior := range context.Task.RequiredBehaviors {
		if strings.TrimSpace(behavior) == "" || behavior != strings.TrimSpace(behavior) {
			return "", fmt.Errorf("application task contains invalid required behavior %d", index+1)
		}
		parts = append(parts, fmt.Sprintf("Required behavior %d: %s", index+1, behavior))
	}
	for index, criterion := range context.Task.AcceptanceCriteria {
		if strings.TrimSpace(criterion) == "" || criterion != strings.TrimSpace(criterion) {
			return "", fmt.Errorf("application task contains invalid acceptance criterion %d", index+1)
		}
		parts = append(parts, fmt.Sprintf("Observable acceptance %d: %s", index+1, criterion))
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
