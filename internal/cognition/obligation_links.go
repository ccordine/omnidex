package cognition

import "fmt"

func validateObligationLinks(snapshot ObligationGraphSnapshot, items map[ObligationID]Obligation) error {
	root, exists := items[snapshot.RootID]
	if !exists || root.ParentID != "" || root.CreatedGeneration != snapshot.Generation ||
		root.Status == ObligationSuperseded {
		return fmt.Errorf("%w: current root is missing, historical, parented, or superseded", ErrInvalidObligationGraph)
	}
	adjacency := make(map[ObligationID][]ObligationID, len(items))
	for _, obligation := range items {
		if obligation.ParentID != "" {
			parent, exists := items[obligation.ParentID]
			if !exists {
				return fmt.Errorf("%w: obligation %q has an unknown parent", ErrInvalidObligationGraph, obligation.ID)
			}
			if parent.CreatedGeneration != obligation.CreatedGeneration {
				return fmt.Errorf("%w: obligation %q crosses parent generations", ErrInvalidObligationGraph, obligation.ID)
			}
			adjacency[parent.ID] = append(adjacency[parent.ID], obligation.ID)
		}
		for _, dependencyID := range obligation.DependsOn {
			dependency, exists := items[dependencyID]
			if !exists {
				return fmt.Errorf("%w: obligation %q has an unknown dependency", ErrInvalidObligationGraph, obligation.ID)
			}
			if dependency.CreatedGeneration > obligation.CreatedGeneration ||
				(dependency.CreatedGeneration < obligation.CreatedGeneration && dependency.Status != ObligationSatisfied) {
				return fmt.Errorf("%w: obligation %q depends on unavailable history", ErrInvalidObligationGraph, obligation.ID)
			}
			adjacency[obligation.ID] = append(adjacency[obligation.ID], dependencyID)
		}
		if err := validateDependencyStatus(obligation, items); err != nil {
			return err
		}
	}
	if err := validateAcyclic(adjacency, items); err != nil {
		return err
	}
	for _, obligation := range items {
		if err := validateParentDepth(obligation, items, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateDependencyStatus(obligation Obligation, items map[ObligationID]Obligation) error {
	allSatisfied := true
	for _, dependencyID := range obligation.DependsOn {
		if items[dependencyID].Status != ObligationSatisfied {
			allSatisfied = false
			break
		}
	}
	switch obligation.Status {
	case ObligationReady, ObligationActive:
		if !allSatisfied {
			return fmt.Errorf("%w: obligation %q is %s with unresolved dependencies", ErrInvalidObligationGraph, obligation.ID, obligation.Status)
		}
	case ObligationBlocked:
		if allSatisfied {
			return fmt.Errorf("%w: obligation %q is blocked without an unresolved dependency", ErrInvalidObligationGraph, obligation.ID)
		}
	}
	return nil
}

func validateAcyclic(adjacency map[ObligationID][]ObligationID, items map[ObligationID]Obligation) error {
	const (
		visiting = 1
		visited  = 2
	)
	states := make(map[ObligationID]int, len(items))
	var visit func(ObligationID) error
	visit = func(id ObligationID) error {
		if states[id] == visiting {
			return fmt.Errorf("%w: obligation graph contains a cycle at %q", ErrInvalidObligationGraph, id)
		}
		if states[id] == visited {
			return nil
		}
		states[id] = visiting
		for _, next := range adjacency[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		states[id] = visited
		return nil
	}
	for id := range items {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateParentDepth(
	obligation Obligation,
	items map[ObligationID]Obligation,
	snapshot ObligationGraphSnapshot,
) error {
	depth := 1
	cursor := obligation
	for cursor.ParentID != "" {
		depth++
		if depth > MaxObligationDepth {
			return fmt.Errorf("%w: obligation %q exceeds maximum depth %d", ErrInvalidObligationGraph, obligation.ID, MaxObligationDepth)
		}
		cursor = items[cursor.ParentID]
	}
	if obligation.CreatedGeneration == snapshot.Generation && cursor.ID != snapshot.RootID {
		return fmt.Errorf("%w: current obligation %q is outside the current root", ErrInvalidObligationGraph, obligation.ID)
	}
	return nil
}
