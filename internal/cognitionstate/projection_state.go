package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// ProjectionState is the exact cognition authority available before a Context
// Projection exists. Keeping this state projection-independent prevents a
// circular authority in which a snapshot would have to bind the projection
// used to construct that same snapshot.
type ProjectionState struct {
	goal         cognition.GoalExpression
	revision     cognition.WorldRevision
	obligation   cognition.Obligation
	catalog      cognition.ActionCatalog
	attempt      cognition.AttemptRef
	budget       cognition.RuntimeBudget
	evidenceRefs []cognition.EvidenceRef
	sha256       string
}

func NewProjectionState(
	goal cognition.GoalExpression,
	revision cognition.WorldRevision,
	obligation cognition.Obligation,
	catalog cognition.ActionCatalog,
	attempt cognition.AttemptRef,
	budget cognition.RuntimeBudget,
	evidenceRefs []cognition.EvidenceRef,
) (ProjectionState, error) {
	state := ProjectionState{
		goal: goal.Clone(), revision: revision, obligation: obligation.Clone(),
		catalog: catalog.Clone(), attempt: attempt, budget: budget,
		evidenceRefs: append([]cognition.EvidenceRef{}, evidenceRefs...),
	}
	digest, err := mappingDigest(state.identity())
	if err != nil {
		return ProjectionState{}, err
	}
	state.sha256 = digest
	if err := state.Validate(); err != nil {
		return ProjectionState{}, err
	}
	return state, nil
}

func ProjectionStateFromSnapshot(snapshot cognition.RuntimeSnapshot) (ProjectionState, error) {
	if err := snapshot.Validate(); err != nil {
		return ProjectionState{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidReconciliation, err)
	}
	return NewProjectionState(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.Budget(), snapshot.EvidenceRefs(),
	)
}

func (state ProjectionState) Validate() error {
	if err := state.goal.Validate(); err != nil {
		return fmt.Errorf("%w: goal: %v", ErrInvalidReconciliation, err)
	}
	if err := state.revision.Validate(); err != nil {
		return fmt.Errorf("%w: revision: %v", ErrInvalidReconciliation, err)
	}
	if err := state.obligation.Validate(); err != nil || state.obligation.Status != cognition.ObligationActive {
		return fmt.Errorf("%w: current obligation must be valid and active", ErrInvalidReconciliation)
	}
	if err := state.catalog.Validate(); err != nil {
		return fmt.Errorf("%w: catalog: %v", ErrInvalidReconciliation, err)
	}
	if err := state.attempt.Validate(); err != nil {
		return fmt.Errorf("%w: attempt: %v", ErrInvalidReconciliation, err)
	}
	if err := state.budget.Validate(); err != nil {
		return fmt.Errorf("%w: budget: %v", ErrInvalidReconciliation, err)
	}
	if state.revision.EpisodeID == "" || len(state.evidenceRefs) > state.budget.MaxEvidenceRefs {
		return fmt.Errorf("%w: projection authority is inconsistent", ErrInvalidReconciliation)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(state.evidenceRefs))
	for index, ref := range state.evidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != state.revision.EpisodeID ||
			ref.Revision.Number > state.revision.Number {
			return fmt.Errorf("%w: evidence %d is invalid or stale", ErrInvalidReconciliation, index)
		}
		if _, duplicate := available[ref]; duplicate {
			return fmt.Errorf("%w: evidence %d is duplicated", ErrInvalidReconciliation, index)
		}
		available[ref] = struct{}{}
	}
	for _, ref := range state.obligation.SupportingRefs {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf("%w: obligation support is outside available evidence", ErrInvalidReconciliation)
		}
	}
	expected, err := mappingDigest(state.identity())
	if err != nil || expected != state.sha256 {
		return fmt.Errorf("%w: projection state hash changed", ErrInvalidReconciliation)
	}
	return nil
}

func (state ProjectionState) Goal() cognition.GoalExpression    { return state.goal.Clone() }
func (state ProjectionState) Revision() cognition.WorldRevision { return state.revision }
func (state ProjectionState) Obligation() cognition.Obligation  { return state.obligation.Clone() }
func (state ProjectionState) Catalog() cognition.ActionCatalog  { return state.catalog.Clone() }
func (state ProjectionState) Attempt() cognition.AttemptRef     { return state.attempt }
func (state ProjectionState) Budget() cognition.RuntimeBudget   { return state.budget }
func (state ProjectionState) EvidenceRefs() []cognition.EvidenceRef {
	return append([]cognition.EvidenceRef{}, state.evidenceRefs...)
}
func (state ProjectionState) SHA256() string { return state.sha256 }

func (state ProjectionState) identity() any {
	return struct {
		Goal       cognition.GoalExpression
		Revision   cognition.WorldRevision
		Obligation cognition.Obligation
		Catalog    cognition.ActionCatalog
		Attempt    cognition.AttemptRef
		Budget     cognition.RuntimeBudget
		Evidence   []cognition.EvidenceRef
	}{state.goal, state.revision, state.obligation, state.catalog, state.attempt,
		state.budget, append([]cognition.EvidenceRef{}, state.evidenceRefs...)}
}
