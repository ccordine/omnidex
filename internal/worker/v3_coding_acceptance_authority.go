package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingBrowserAcceptancePlatformAuthorities() []assemblyline.AcceptanceGroundingAuthority {
	const mechanicsOnly = " This authorizes test-harness mechanics only; it does not authorize product-specific roles, labels, quantities, ranges, defaults, or values."
	return []assemblyline.AcceptanceGroundingAuthority{
		{
			ID: "platform_browser_harness_wait", Kind: assemblyline.AcceptanceGroundingPlatformInvariant,
			Statement:  "The registered browser harness may wait for asynchronous observable behavior." + mechanicsOnly,
			Operations: []string{"harness_call:waitFor"},
		},
	}
}

func directCodingAcceptanceTaskAuthority(
	program directCodingProgram,
	acceptanceID string,
) (assemblyline.ApplicationTaskContext, string, bool, error) {
	var zero assemblyline.ApplicationTaskContext
	for _, task := range program.Workload.Tasks {
		featureID, taskAcceptanceID, err := applicationTaskBlockIDs(task.ID)
		if err != nil {
			return zero, "", false, err
		}
		if taskAcceptanceID != acceptanceID {
			continue
		}
		context, err := directCodingApplicationTaskContextFromFrozen(program.Workload, task.ID)
		if err != nil {
			return zero, "", false, err
		}
		return context, featureID, true, nil
	}
	return zero, "", false, nil
}

func directCodingApplicationTaskContextFromFrozen(
	frozen assemblyline.FrozenApplicationWorkload,
	taskID string,
) (assemblyline.ApplicationTaskContext, error) {
	requirements := make([]assemblyline.Requirement, len(frozen.Tasks))
	for index, task := range frozen.Tasks {
		requirements[index] = assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		}
	}
	input := assemblyline.ApplicationWorkloadDraftInput{
		Surface: frozen.Surface, ProductQuote: frozen.ProductQuote, Requirements: requirements,
	}
	context, err := assemblyline.ProjectApplicationTaskContext(input, frozen, taskID)
	if err != nil {
		return assemblyline.ApplicationTaskContext{}, fmt.Errorf(
			"project acceptance grounding authority for %s: %w", taskID, err,
		)
	}
	return context, nil
}
