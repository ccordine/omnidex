package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
)

func (journal *ablationCallJournal) bindInput(
	attemptID string,
	projection contextbuilder.Projection,
	snapshot cognition.RuntimeSnapshot,
) error {
	if journal == nil {
		return fmt.Errorf("ablation call journal is nil")
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return fmt.Errorf("ablation call journal is sealed")
	}
	attempt, exists := journal.attempts[attemptID]
	if !exists {
		return fmt.Errorf("ablation runtime input lacks its exact call attempt")
	}
	if _, duplicate := journal.inputs[attemptID]; duplicate {
		return fmt.Errorf("ablation runtime input was replaced")
	}
	if err := cognitionpolicy.VerifyCallAttempt(snapshot, projection, attempt); err != nil {
		return fmt.Errorf("ablation runtime input differs from its exact call: %w", err)
	}
	journal.inputs[attemptID] = ablationCallInput{
		Projection: cloneAblationProjection(projection), Snapshot: newSemanticRuntimeSnapshot(snapshot),
	}
	return nil
}

func newSemanticRuntimeSnapshot(snapshot cognition.RuntimeSnapshot) semanticRuntimeSnapshot {
	return semanticRuntimeSnapshot{
		Goal: snapshot.Goal(), CurrentRevision: snapshot.CurrentRevision(),
		CurrentObligation: snapshot.CurrentObligation(), ActionCatalog: snapshot.ActionCatalog(),
		Attempt: snapshot.Attempt(), ContextProjection: snapshot.ContextProjection(),
		Budget: snapshot.Budget(), EvidenceRefs: snapshot.EvidenceRefs(),
	}
}

func (value semanticRuntimeSnapshot) runtimeSnapshot() (cognition.RuntimeSnapshot, error) {
	return cognition.NewRuntimeSnapshot(
		value.Goal, value.CurrentRevision, value.CurrentObligation, value.ActionCatalog,
		value.Attempt, value.ContextProjection, value.Budget, value.EvidenceRefs,
	)
}

func (value semanticRuntimeSnapshot) clone() semanticRuntimeSnapshot {
	value.Goal = value.Goal.Clone()
	value.CurrentObligation = value.CurrentObligation.Clone()
	value.ActionCatalog = value.ActionCatalog.Clone()
	value.EvidenceRefs = append([]cognition.EvidenceRef(nil), value.EvidenceRefs...)
	return value
}

func cloneAblationProjection(value contextbuilder.Projection) contextbuilder.Projection {
	omitted := value.Omitted
	value.Selected = append([]contextbuilder.Selection(nil), value.Selected...)
	for index := range value.Selected {
		sources := value.Selected[index].SourceRefs
		value.Selected[index].SourceRefs = make([]taskstate.Ref, len(sources))
		copy(value.Selected[index].SourceRefs, sources)
	}
	value.Omitted = make([]contextbuilder.Omission, len(omitted))
	copy(value.Omitted, omitted)
	return value
}
