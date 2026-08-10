package cognition

import (
	"fmt"
	"math"
	"sort"
)

func (graph *ObligationGraph) RefreshReadiness(generation uint64) error {
	if err := graph.requireGeneration(generation); err != nil {
		return err
	}
	candidate := graph.clone()
	refreshCandidateReadiness(candidate, generation)
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) Transition(
	id ObligationID,
	generation uint64,
	to ObligationStatus,
) error {
	if err := graph.requireGeneration(generation); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligationTransition, err)
	}
	obligation, exists := graph.items[id]
	if !exists || obligation.CreatedGeneration != generation {
		return fmt.Errorf("%w: current obligation %q not found", ErrInvalidObligationTransition, id)
	}
	if to == ObligationSatisfied || to == ObligationSuperseded {
		return fmt.Errorf("%w: completion and supersession are code-owned operations", ErrAuthorityDenied)
	}
	allowed := to == ObligationFailed && !terminalOrSuperseded(obligation.Status)
	if obligation.Status == ObligationReady && to == ObligationActive {
		allowed = true
	}
	if !allowed {
		return fmt.Errorf("%w: %s to %s is not registered", ErrInvalidObligationTransition, obligation.Status, to)
	}
	candidate := graph.clone()
	obligation.Status = to
	candidate.items[id] = obligation
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligationTransition, err)
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) AddSupportingEvidence(
	id ObligationID,
	generation uint64,
	refs []EvidenceRef,
) error {
	if err := graph.requireGeneration(generation); err != nil {
		return err
	}
	if err := validateEvidenceRefs(refs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligation, err)
	}
	obligation, exists := graph.items[id]
	if !exists || obligation.CreatedGeneration != generation || terminalOrSuperseded(obligation.Status) {
		return fmt.Errorf("%w: supporting evidence target %q is unavailable", ErrInvalidObligation, id)
	}
	combined := append(append([]EvidenceRef(nil), obligation.SupportingRefs...), refs...)
	sort.Slice(combined, func(left, right int) bool {
		return evidenceIdentity(combined[left]) < evidenceIdentity(combined[right])
	})
	if err := validateEvidenceRefs(combined); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligation, err)
	}
	candidate := graph.clone()
	obligation.SupportingRefs = combined
	candidate.items[id] = obligation
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) AddDependency(
	id ObligationID,
	dependencyID ObligationID,
	generation uint64,
) error {
	if err := graph.requireGeneration(generation); err != nil {
		return err
	}
	obligation, exists := graph.items[id]
	dependency, dependencyExists := graph.items[dependencyID]
	if !exists || obligation.CreatedGeneration != generation || terminalOrSuperseded(obligation.Status) {
		return fmt.Errorf("%w: dependency target %q is unavailable", ErrInvalidObligation, id)
	}
	if !dependencyExists || dependency.Status == ObligationFailed || dependency.Status == ObligationSuperseded {
		return fmt.Errorf("%w: dependency %q is unavailable", ErrInvalidObligation, dependencyID)
	}
	if len(obligation.DependsOn) >= MaxObligationDependencies {
		return fmt.Errorf("%w: dependency count exceeds %d", ErrInvalidObligation, MaxObligationDependencies)
	}
	for _, existing := range obligation.DependsOn {
		if existing == dependencyID {
			return fmt.Errorf("%w: dependency %q already exists", ErrInvalidObligation, dependencyID)
		}
	}
	candidate := graph.clone()
	obligation.DependsOn = append(append([]ObligationID(nil), obligation.DependsOn...), dependencyID)
	sort.Slice(obligation.DependsOn, func(left, right int) bool {
		return obligation.DependsOn[left] < obligation.DependsOn[right]
	})
	if dependency.Status != ObligationSatisfied {
		obligation.Status = ObligationBlocked
	}
	candidate.items[id] = obligation
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) Satisfy(
	id ObligationID,
	generation uint64,
	result CompletionResult,
) error {
	if err := graph.requireGeneration(generation); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligationTransition, err)
	}
	obligation, exists := graph.items[id]
	if !exists || obligation.CreatedGeneration != generation || obligation.Status != ObligationActive {
		return fmt.Errorf("%w: only the active current obligation may be satisfied", ErrInvalidObligationTransition)
	}
	if err := result.ValidateFor(obligation, result.Revision, obligation.SupportingRefs); err != nil {
		return err
	}
	if result.Outcome != CompletionSatisfied {
		return fmt.Errorf("%w: unsatisfied completion result cannot satisfy an obligation", ErrInvalidCompletionResult)
	}
	candidate := graph.clone()
	completion := result.Clone()
	obligation.Status = ObligationSatisfied
	obligation.Completion = &completion
	candidate.items[id] = obligation
	refreshCandidateReadiness(candidate, generation)
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func (graph *ObligationGraph) Cutover(
	nextGeneration uint64,
	rootID ObligationID,
	specs []ObligationSpec,
) error {
	if graph == nil || graph.items == nil || graph.generation == math.MaxUint64 ||
		nextGeneration != graph.generation+1 {
		return fmt.Errorf("%w: cutover must advance exactly one generation", ErrInvalidObligationGeneration)
	}
	if len(specs) == 0 || len(graph.items)+len(specs) > MaxObligations {
		return fmt.Errorf("%w: replacement obligations exceed graph bounds", ErrInvalidObligationGraph)
	}
	candidate := graph.clone()
	for id, obligation := range candidate.items {
		if !terminalOrSuperseded(obligation.Status) {
			obligation.Status = ObligationSuperseded
			obligation.SupersededGeneration = nextGeneration
			candidate.items[id] = obligation
		}
	}
	candidate.generation = nextGeneration
	candidate.rootID = rootID
	for _, spec := range specs {
		if _, duplicate := candidate.items[spec.ID]; duplicate {
			return fmt.Errorf("%w: obligation ID %q already exists", ErrInvalidObligationGraph, spec.ID)
		}
		candidate.items[spec.ID] = obligationFromSpec(spec, nextGeneration)
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	graph.commit(candidate)
	return nil
}

func dependenciesSatisfied(obligation Obligation, items map[ObligationID]Obligation) bool {
	for _, dependency := range obligation.DependsOn {
		if items[dependency].Status != ObligationSatisfied {
			return false
		}
	}
	return true
}

func refreshCandidateReadiness(graph *ObligationGraph, generation uint64) {
	for id, obligation := range graph.items {
		if obligation.CreatedGeneration != generation ||
			(obligation.Status != ObligationProposed && obligation.Status != ObligationBlocked) {
			continue
		}
		if dependenciesSatisfied(obligation, graph.items) {
			obligation.Status = ObligationReady
		} else {
			obligation.Status = ObligationBlocked
		}
		graph.items[id] = obligation
	}
}
