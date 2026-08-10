package cognition

import (
	"fmt"
	"sort"
)

func NewObligationGraph(
	generation uint64,
	rootID ObligationID,
	specs []ObligationSpec,
) (*ObligationGraph, error) {
	if generation == 0 {
		return nil, fmt.Errorf("%w: generation must be positive", ErrInvalidObligationGeneration)
	}
	if len(specs) == 0 || len(specs) > MaxObligations {
		return nil, fmt.Errorf("%w: obligation count must be between 1 and %d", ErrInvalidObligationGraph, MaxObligations)
	}
	graph := &ObligationGraph{
		generation: generation,
		rootID:     rootID,
		items:      make(map[ObligationID]Obligation, len(specs)),
	}
	for _, spec := range specs {
		if _, duplicate := graph.items[spec.ID]; duplicate {
			return nil, fmt.Errorf("%w: obligation ID %q is duplicated", ErrInvalidObligationGraph, spec.ID)
		}
		graph.items[spec.ID] = obligationFromSpec(spec, generation)
	}
	if err := graph.Validate(); err != nil {
		return nil, err
	}
	return graph, nil
}

func RestoreObligationGraph(snapshot ObligationGraphSnapshot) (*ObligationGraph, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	graph := &ObligationGraph{
		generation: snapshot.Generation,
		rootID:     snapshot.RootID,
		items:      make(map[ObligationID]Obligation, len(snapshot.Obligations)),
	}
	for _, obligation := range snapshot.Obligations {
		graph.items[obligation.ID] = obligation.Clone()
	}
	return graph, nil
}

func (graph *ObligationGraph) Validate() error {
	if graph == nil || graph.items == nil {
		return fmt.Errorf("%w: graph is nil or uninitialized", ErrInvalidObligationGraph)
	}
	return graph.Snapshot().Validate()
}

func (graph *ObligationGraph) Generation() uint64 {
	if graph == nil {
		return 0
	}
	return graph.generation
}

func (graph *ObligationGraph) RootID() ObligationID {
	if graph == nil {
		return ""
	}
	return graph.rootID
}

func (graph *ObligationGraph) Obligation(id ObligationID) (Obligation, bool) {
	if graph == nil {
		return Obligation{}, false
	}
	obligation, exists := graph.items[id]
	return obligation.Clone(), exists
}

func (graph *ObligationGraph) Snapshot() ObligationGraphSnapshot {
	if graph == nil {
		return ObligationGraphSnapshot{}
	}
	obligations := make([]Obligation, 0, len(graph.items))
	for _, obligation := range graph.items {
		obligations = append(obligations, obligation.Clone())
	}
	sort.Slice(obligations, func(left, right int) bool {
		return obligations[left].ID < obligations[right].ID
	})
	snapshot := ObligationGraphSnapshot{
		Schema: ObligationGraphSchemaV1, Generation: graph.generation,
		RootID: graph.rootID, Obligations: obligations,
	}
	snapshot.SHA256 = obligationSnapshotSHA256(snapshot)
	return snapshot
}

func (graph *ObligationGraph) Add(generation uint64, spec ObligationSpec) error {
	if err := graph.requireGeneration(generation); err != nil {
		return err
	}
	if len(graph.items) >= MaxObligations {
		return fmt.Errorf("%w: obligation count exceeds %d", ErrInvalidObligationGraph, MaxObligations)
	}
	if _, duplicate := graph.items[spec.ID]; duplicate {
		return fmt.Errorf("%w: obligation ID %q already exists", ErrInvalidObligationGraph, spec.ID)
	}
	candidate := graph.clone()
	candidate.items[spec.ID] = obligationFromSpec(spec, generation)
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) TerminalStatus() (ObligationGraphTerminalStatus, error) {
	if err := graph.Validate(); err != nil {
		return "", err
	}
	switch graph.items[graph.rootID].Status {
	case ObligationSatisfied:
		return ObligationGraphSatisfied, nil
	case ObligationFailed:
		return ObligationGraphFailed, nil
	default:
		return ObligationGraphRunning, nil
	}
}

func (graph *ObligationGraph) requireGeneration(generation uint64) error {
	if graph == nil || graph.items == nil {
		return fmt.Errorf("%w: graph is nil or uninitialized", ErrInvalidObligationGraph)
	}
	if generation == 0 || generation != graph.generation {
		return fmt.Errorf("%w: expected=%d actual=%d", ErrInvalidObligationGeneration, graph.generation, generation)
	}
	return nil
}

func (graph *ObligationGraph) clone() *ObligationGraph {
	cloned := &ObligationGraph{
		generation: graph.generation,
		rootID:     graph.rootID,
		items:      make(map[ObligationID]Obligation, len(graph.items)),
	}
	for id, obligation := range graph.items {
		cloned.items[id] = obligation.Clone()
	}
	return cloned
}

func (graph *ObligationGraph) commit(candidate *ObligationGraph) {
	graph.generation = candidate.generation
	graph.rootID = candidate.rootID
	graph.items = candidate.items
}

func obligationFromSpec(spec ObligationSpec, generation uint64) Obligation {
	dependencies := cloneSlice(spec.DependsOn)
	sort.Slice(dependencies, func(left, right int) bool { return dependencies[left] < dependencies[right] })
	supporting := cloneSlice(spec.SupportingRefs)
	sort.Slice(supporting, func(left, right int) bool {
		return evidenceIdentity(supporting[left]) < evidenceIdentity(supporting[right])
	})
	return Obligation{
		ID: spec.ID, ParentID: spec.ParentID, Desired: spec.Desired.Clone(),
		Status: ObligationProposed, DependsOn: dependencies, SupportingRefs: supporting,
		CompletionCheck: spec.CompletionCheck, CreatedGeneration: generation,
	}
}
