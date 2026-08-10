package cognition

import (
	"fmt"
	"reflect"
)

func simulatePlanRevision(
	before ObligationGraphSnapshot,
	activeID ObligationID,
	root ObligationSpec,
	next ObligationSpec,
) (ObligationGraphSnapshot, error) {
	if err := before.Validate(); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: graph: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	active, exists := obligationInSnapshot(before, activeID)
	oldRoot, rootExists := obligationInSnapshot(before, before.RootID)
	if !exists || active.Status != ObligationActive || active.CreatedGeneration != before.Generation {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: target is not the active current obligation", ErrInvalidPlanRevisionMaterialization)
	}
	if !rootExists || terminalOrSuperseded(oldRoot.Status) || oldRoot.CreatedGeneration != before.Generation ||
		!reflect.DeepEqual(oldRoot.Desired, root.Desired) || oldRoot.CompletionCheck != root.CompletionCheck ||
		!reflect.DeepEqual(oldRoot.SupportingRefs, root.SupportingRefs) {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: replacement root changed the root goal authority", ErrInvalidPlanRevisionMaterialization)
	}
	graph, err := RestoreObligationGraph(before)
	if err != nil {
		return ObligationGraphSnapshot{}, err
	}
	if err := graph.Cutover(before.Generation+1, root.ID, []ObligationSpec{root, next}); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: cutover: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if err := graph.RefreshReadiness(before.Generation + 1); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: refresh: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if err := graph.Transition(next.ID, before.Generation+1, ObligationActive); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: activate next obligation: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	return graph.Snapshot(), nil
}

func (value PlanRevisionMaterialization) Apply(
	before ObligationGraphSnapshot,
) (ObligationGraphSnapshot, error) {
	if err := value.Validate(); err != nil {
		return ObligationGraphSnapshot{}, err
	}
	if err := before.Validate(); err != nil || before.Generation != value.PreviousGeneration ||
		before.SHA256 != value.ExpectedGraphSHA256 {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: expected graph authority is stale", ErrInvalidPlanRevisionMaterialization)
	}
	after, err := simulatePlanRevision(before, value.ActiveObligationID, value.Root, value.Next)
	if err != nil {
		return ObligationGraphSnapshot{}, err
	}
	if after.SHA256 != value.ResultGraphSHA256 {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: result hash does not bind the graph cutover", ErrInvalidPlanRevisionMaterialization)
	}
	return after.Clone(), nil
}
