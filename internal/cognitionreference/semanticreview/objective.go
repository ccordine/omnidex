package semanticreview

import (
	"fmt"
	"reflect"
)

var exactRootAcceptance = []AcceptancePredicate{
	AcceptanceCurrentArtifactVerified,
	AcceptanceNoOpenSemanticFinding,
}

func validateObjective(objective Objective) error {
	if !validIdentity(string(objective.ID)) {
		return fmt.Errorf("%w: objective ID is invalid", ErrInvalidObjective)
	}
	if objective.Status != ObjectivePending {
		return fmt.Errorf("%w: initial status must be %q", ErrInvalidObjective, ObjectivePending)
	}
	if !reflect.DeepEqual(objective.Acceptance, exactRootAcceptance) {
		return fmt.Errorf("%w: acceptance predicates are not the exact registered set", ErrInvalidObjective)
	}
	return nil
}

func cloneObjective(value Objective) Objective {
	value.Acceptance = append([]AcceptancePredicate{}, value.Acceptance...)
	return value
}

func validObjectiveStatus(value ObjectiveStatus) bool {
	switch value {
	case ObjectivePending, ObjectiveComplete, ObjectiveFailed, ObjectiveBoundBlocked:
		return true
	default:
		return false
	}
}
