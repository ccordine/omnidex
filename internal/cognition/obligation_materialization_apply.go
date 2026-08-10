package cognition

import "fmt"

func simulateObligationMaterialization(
	before ObligationGraphSnapshot,
	activeID ObligationID,
	spec ObligationSpec,
) (ObligationGraphSnapshot, error) {
	if err := before.Validate(); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: graph: %v", ErrInvalidObligationMaterialization, err)
	}
	active, exists := obligationInSnapshot(before, activeID)
	if !exists || active.Status != ObligationActive || active.CreatedGeneration != before.Generation {
		return ObligationGraphSnapshot{}, fmt.Errorf(
			"%w: target is not the active current-generation obligation",
			ErrInvalidObligationMaterialization,
		)
	}
	proposedGoal, err := goalIdentity(spec.Desired)
	if err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: desired goal: %v", ErrInvalidObligationMaterialization, err)
	}
	for _, obligation := range before.Obligations {
		existingGoal, identityErr := goalIdentity(obligation.Desired)
		if identityErr != nil {
			return ObligationGraphSnapshot{}, fmt.Errorf("%w: existing goal: %v", ErrInvalidObligationMaterialization, identityErr)
		}
		if existingGoal == proposedGoal {
			return ObligationGraphSnapshot{}, fmt.Errorf(
				"%w: proposed goal duplicates obligation %q", ErrInvalidObligationMaterialization, obligation.ID,
			)
		}
	}
	graph, err := RestoreObligationGraph(before)
	if err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: restore graph: %v", ErrInvalidObligationMaterialization, err)
	}
	if err := graph.Add(before.Generation, spec); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: add child: %v", ErrInvalidObligationMaterialization, err)
	}
	if err := graph.RefreshReadiness(before.Generation); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: prepare child: %v", ErrInvalidObligationMaterialization, err)
	}
	if err := graph.AddDependency(activeID, spec.ID, before.Generation); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: block parent: %v", ErrInvalidObligationMaterialization, err)
	}
	if err := graph.Transition(spec.ID, before.Generation, ObligationActive); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: activate child: %v", ErrInvalidObligationMaterialization, err)
	}
	return graph.Snapshot(), nil
}

func obligationInSnapshot(
	snapshot ObligationGraphSnapshot,
	id ObligationID,
) (Obligation, bool) {
	for _, obligation := range snapshot.Obligations {
		if obligation.ID == id {
			return obligation.Clone(), true
		}
	}
	return Obligation{}, false
}
