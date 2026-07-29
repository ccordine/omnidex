package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/artifacts"
)

// validateV3IntentActionShape enforces only the structural relationship needed
// by the deterministic execution route. Semantic interpretation belongs to the
// prompt interpreter and is never reconstructed from user wording in code.
func validateV3IntentActionShape(input v3IntentInput, candidate artifacts.IntentArtifact) error {
	if (input.OperationKind != v3OperationUserRequest && input.OperationKind != v3OperationScrumChannel) || !candidate.RequiresAction {
		return nil
	}
	actionCount := 0
	for _, objective := range candidate.Objectives {
		if objective.RequiresAction {
			actionCount++
		}
	}
	if actionCount != 1 {
		return fmt.Errorf(
			"server intent authority requires exactly one action objective for an actionable conversation; received %d",
			actionCount,
		)
	}
	if len(candidate.Objectives) != 1 {
		return fmt.Errorf(
			"server intent authority requires one cohesive objective for an actionable conversation; received %d objectives",
			len(candidate.Objectives),
		)
	}
	return nil
}
