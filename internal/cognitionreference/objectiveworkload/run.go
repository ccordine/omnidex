package objectiveworkload

import (
	"context"
	"fmt"
)

func Run(
	ctx context.Context,
	workload Workload,
	operations Operations,
	limits RunLimits,
) (RunResult, error) {
	base := RunResult{
		EvidenceClass: EvidencePrimitiveContaminatedNonAutonomy,
		Objectives:    []Objective{}, Artifacts: []Artifact{},
		Trace: []TraceTransition{},
	}
	if ctx == nil {
		return base, fmt.Errorf("%w: context is required", ErrInvalidRun)
	}
	if err := ctx.Err(); err != nil {
		return base, err
	}
	if err := limits.validate(); err != nil {
		return base, err
	}
	if nilOperations(operations) {
		return base, fmt.Errorf("%w: code operations are required", ErrInvalidRun)
	}
	if err := preflightWorkloadBounds(workload); err != nil {
		return base, err
	}
	owned := cloneWorkload(workload)
	if err := validateWorkload(owned, true); err != nil {
		return base, err
	}
	base.WorkloadID = owned.ID
	state := newRunState(owned, operations, limits)
	for {
		if err := ctx.Err(); err != nil {
			return state.result(false), err
		}
		root := state.objective(state.workload.RootObjectiveID)
		if root.Status == ObjectiveComplete {
			if err := state.validateCompletion(); err != nil {
				return state.result(false), err
			}
			if err := ctx.Err(); err != nil {
				return state.result(false), err
			}
			return state.result(true), nil
		}
		progressed, err := state.advance(ctx, root.ID, 1, make(map[ObjectiveID]bool))
		if err != nil {
			return state.result(false), err
		}
		if !progressed {
			return state.result(false), fmt.Errorf("%w: root %q remains pending", ErrDeadlock, root.ID)
		}
	}
}

type runState struct {
	workload     Workload
	operations   Operations
	limits       RunLimits
	indices      map[ObjectiveID]int
	requirements map[RequirementID]Requirement
	artifacts    map[RequirementID]Artifact
	trace        []TraceTransition
	opCalls      int
}

func newRunState(workload Workload, operations Operations, limits RunLimits) *runState {
	state := &runState{
		workload: workload, operations: operations, limits: limits,
		indices:      make(map[ObjectiveID]int, len(workload.Objectives)),
		requirements: make(map[RequirementID]Requirement, len(workload.Requirements)),
		artifacts:    make(map[RequirementID]Artifact), trace: []TraceTransition{},
	}
	for index, objective := range workload.Objectives {
		state.indices[objective.ID] = index
	}
	for _, requirement := range workload.Requirements {
		state.requirements[requirement.ID] = requirement
	}
	return state
}

func (state *runState) objective(id ObjectiveID) Objective {
	return state.workload.Objectives[state.indices[id]]
}

func (state *runState) advance(
	ctx context.Context,
	id ObjectiveID,
	depth int,
	visiting map[ObjectiveID]bool,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if depth > state.limits.MaxDepth {
		return false, fmt.Errorf("%w: exceeded depth %d", ErrRunBound, state.limits.MaxDepth)
	}
	if visiting[id] {
		return false, fmt.Errorf("%w: dependency cycle at %q", ErrInvalidGraph, id)
	}
	objective := state.objective(id)
	if objective.Status == ObjectiveComplete {
		return false, nil
	}
	visiting[id] = true
	defer delete(visiting, id)
	for _, dependencyID := range objective.DependsOn {
		dependency := state.objective(dependencyID)
		if dependency.Status == ObjectiveComplete {
			continue
		}
		progressed, err := state.advance(ctx, dependencyID, depth+1, visiting)
		if err != nil {
			return false, err
		}
		if progressed {
			return true, nil
		}
		if state.objective(dependencyID).Status != ObjectiveComplete {
			return false, fmt.Errorf("%w: dependency %q remains pending", ErrDeadlock, dependencyID)
		}
	}
	if len(state.trace) >= state.limits.MaxTransitions {
		return false, fmt.Errorf(
			"%w: exceeded %d transitions", ErrRunBound, state.limits.MaxTransitions,
		)
	}
	return state.perform(ctx, objective)
}

func (state *runState) perform(ctx context.Context, objective Objective) (bool, error) {
	switch objective.Kind {
	case ObjectiveMaterialize:
		requirement := state.requirements[objective.RequirementID]
		item := state.workItem(requirement)
		if err := ctx.Err(); err != nil {
			return false, err
		}
		state.opCalls++
		value, err := state.operations.Materialize(ctx, cloneWorkItem(item))
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		if err != nil {
			return false, fmt.Errorf("%w: materialize %q: %w", ErrOperation, requirement.ID, err)
		}
		artifact, err := newArtifact(state.workload, item, value)
		if err != nil {
			return false, err
		}
		if _, duplicate := state.artifacts[requirement.ID]; duplicate {
			return false, fmt.Errorf("%w: requirement %q already has an artifact", ErrArtifact, requirement.ID)
		}
		state.artifacts[requirement.ID] = artifact
		state.complete(objective.ID, TransitionMaterialized)
		return true, nil
	case ObjectiveVerify:
		requirement := state.requirements[objective.RequirementID]
		artifact, exists := state.artifacts[requirement.ID]
		if !exists {
			return false, fmt.Errorf("%w: requirement %q has no artifact", ErrArtifact, requirement.ID)
		}
		if err := validateArtifact(state.workload, requirement, artifact); err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		state.opCalls++
		err := state.operations.Verify(ctx, cloneWorkItem(state.workItem(requirement)), cloneArtifact(artifact))
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		if err != nil {
			return false, fmt.Errorf("%w: verify %q: %w", ErrOperation, requirement.ID, err)
		}
		if err := validateArtifact(state.workload, requirement, state.artifacts[requirement.ID]); err != nil {
			return false, err
		}
		state.complete(objective.ID, TransitionVerified)
		return true, nil
	case ObjectiveRequirement, ObjectiveRoot:
		state.complete(objective.ID, TransitionAggregated)
		return true, nil
	default:
		return false, fmt.Errorf("%w: objective %q has unsupported kind %q", ErrInvalidGraph, objective.ID, objective.Kind)
	}
}

func (state *runState) complete(id ObjectiveID, kind TransitionKind) {
	index := state.indices[id]
	state.workload.Objectives[index].Status = ObjectiveComplete
	state.trace = append(state.trace, TraceTransition{
		Sequence: len(state.trace) + 1, ObjectiveID: id, Kind: kind,
	})
}

func (state *runState) workItem(requirement Requirement) WorkItem {
	return WorkItem{
		WorkloadID: state.workload.ID, AuthoritySHA256: state.workload.Authority.SHA256,
		Requirement: cloneRequirement(requirement),
	}
}

func (state *runState) validateCompletion() error {
	if err := validateWorkload(state.workload, false); err != nil {
		return err
	}
	for _, objective := range state.workload.Objectives {
		if objective.Status != ObjectiveComplete {
			return fmt.Errorf("%w: objective %q is incomplete", ErrInvalidRun, objective.ID)
		}
	}
	if len(state.artifacts) != len(state.workload.Requirements) {
		return fmt.Errorf("%w: artifact set is incomplete", ErrInvalidRun)
	}
	for _, requirement := range state.workload.Requirements {
		if err := validateArtifact(state.workload, requirement, state.artifacts[requirement.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (state *runState) result(complete bool) RunResult {
	artifacts := make([]Artifact, 0, len(state.workload.Requirements))
	for _, requirement := range state.workload.Requirements {
		if artifact, exists := state.artifacts[requirement.ID]; exists {
			artifacts = append(artifacts, cloneArtifact(artifact))
		}
	}
	objectives := make([]Objective, len(state.workload.Objectives))
	for index, objective := range state.workload.Objectives {
		objectives[index] = cloneObjective(objective)
	}
	return RunResult{
		EvidenceClass: EvidencePrimitiveContaminatedNonAutonomy,
		WorkloadID:    state.workload.ID, Objectives: objectives, Artifacts: artifacts,
		Trace:                       append([]TraceTransition{}, state.trace...),
		DeterministicOperationCalls: state.opCalls,
		StationCalls:                0, ModelCalls: 0, Complete: complete,
	}
}

func nilOperations(operations Operations) bool {
	return nilInterface(operations)
}
