package labyrinth

import (
	"container/heap"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

var solverActor = cognition.AttemptRef{
	JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "symbolic-solver",
}

func Solve(scenario Scenario, bounds SolverBounds) (SolverResult, error) {
	if err := scenario.Validate(); err != nil {
		return SolverResult{}, err
	}
	if bounds.MaxStates < 1 || bounds.MaxStates > MaxSolverStateLimit ||
		bounds.MaxGroundActions < 1 || bounds.MaxGroundActions > MaxSolverGroundActions {
		return SolverResult{}, fmt.Errorf("%w: solver bounds are invalid", ErrSolverLimit)
	}
	definition := scenario.definition.clone()
	entities, _, err := validateEntities(definition.entities)
	if err != nil {
		return SolverResult{}, err
	}
	predicates, err := validatePredicateSchemas(definition.predicateSchemas, entityKinds(entities))
	if err != nil {
		return SolverResult{}, err
	}
	initialFacts := newFactSet(definition.initialFacts)
	initialEvidence := solverEvidence{}
	initialKey := solverStateKey(initialFacts, initialEvidence)
	queue := newSolverQueue(&solverNode{
		facts: initialFacts, evidence: initialEvidence, key: initialKey,
	})
	best := map[string]int{initialKey: 0}
	expanded := 0
	lowerBound := minimumActionCost(definition.actions)
	for queue.Len() > 0 {
		node := heap.Pop(queue).(*solverNode)
		if best[node.key] != node.cost {
			continue
		}
		expanded++
		if goalSatisfied(definition.goal, node.facts) {
			return SolverResult{
				Actions: cloneRequests(node.actions), Cost: node.cost, LowerBound: node.cost,
				ExpandedStates: expanded, Optimal: true,
			}, nil
		}
		requests, groundErr := groundLegalRequests(definition, node.facts, bounds.MaxGroundActions)
		if groundErr != nil {
			return SolverResult{LowerBound: lowerBound, ExpandedStates: expanded}, groundErr
		}
		for _, request := range requests {
			action, exists := actionDefinitionForKind(definition, request.Kind)
			if !exists {
				return SolverResult{}, fmt.Errorf("%w: grounded request has no definition", ErrGeneration)
			}
			if !solverRequestEvidenceGrounded(
				action.Schema, request, node.facts, node.evidence, scenario.descriptor.Records,
			) {
				continue
			}
			registered, registerErr := solverRegisteredAction(action.Schema, request)
			if registerErr != nil {
				return SolverResult{}, registerErr
			}
			candidate, changed, applyErr := applyActionDefinition(action, registered, entities, predicates, node.facts)
			if errors.Is(applyErr, ErrPrecondition) {
				continue
			}
			if applyErr != nil {
				return SolverResult{}, fmt.Errorf("%w: apply grounded request: %v", ErrGeneration, applyErr)
			}
			candidateEvidence := solverEvidenceAfterRequest(scenario, candidate, request, node.evidence)
			if !changed && candidateEvidence.equal(node.evidence) {
				continue
			}
			if len(candidate) > MaxWorldFacts {
				return SolverResult{}, fmt.Errorf("%w: solver candidate exceeds world fact limit", ErrWorldLimit)
			}
			cost := node.cost + action.Cost
			key := solverStateKey(candidate, candidateEvidence)
			if previous, exists := best[key]; exists && previous <= cost {
				continue
			}
			if _, exists := best[key]; !exists && len(best) >= bounds.MaxStates {
				return SolverResult{LowerBound: lowerBound, ExpandedStates: expanded}, fmt.Errorf(
					"%w: state count exceeds %d", ErrSolverLimit, bounds.MaxStates,
				)
			}
			best[key] = cost
			path := append(cloneRequests(node.actions), request.Clone())
			heap.Push(queue, &solverNode{
				facts: candidate, evidence: candidateEvidence, cost: cost, actions: path, key: key,
			})
		}
	}
	return SolverResult{LowerBound: lowerBound, ExpandedStates: expanded}, ErrUnsolvable
}

func solverRegisteredAction(
	schema cognition.ActionSchema,
	request cognition.ActionRequest,
) (cognition.RegisteredAction, error) {
	digest, _, err := digestJSON(request)
	if err != nil {
		return cognition.RegisteredAction{}, err
	}
	registered, err := cognition.NewRegisteredAction(
		cognition.ActionID("solver-action-"+digest[:32]), solverActor, schema, request, generationEvidenceRefs(schema),
	)
	if err != nil {
		return cognition.RegisteredAction{}, fmt.Errorf("%w: register solver action: %v", ErrGeneration, err)
	}
	return registered, nil
}

func actionDefinitionForKind(definition Definition, kind cognition.ActionKind) (ActionDefinition, bool) {
	for _, action := range definition.actions {
		if action.Schema.Kind == kind {
			return action, true
		}
	}
	return ActionDefinition{}, false
}

func cloneRequests(values []cognition.ActionRequest) []cognition.ActionRequest {
	cloned := make([]cognition.ActionRequest, len(values))
	for index, value := range values {
		cloned[index] = value.Clone()
	}
	return cloned
}
