package objectiveworkload

import "context"

type ArtifactKind string

const ArtifactRequirementOutput ArtifactKind = "requirement_output"

type ArtifactValue struct {
	Kind    ArtifactKind
	Content []byte
}

type Artifact struct {
	ID                ArtifactID
	Kind              ArtifactKind
	WorkloadID        WorkloadID
	RequirementID     RequirementID
	AuthoritySHA256   string
	RequirementSHA256 string
	RequirementStart  int
	RequirementEnd    int
	ContentSHA256     string
	Content           []byte
}

type WorkItem struct {
	WorkloadID      WorkloadID
	AuthoritySHA256 string
	Requirement     Requirement
}

type Operations interface {
	Materialize(context.Context, WorkItem) (ArtifactValue, error)
	Verify(context.Context, WorkItem, Artifact) error
}

type TransitionKind string

const (
	TransitionMaterialized TransitionKind = "materialized"
	TransitionVerified     TransitionKind = "verified"
	TransitionAggregated   TransitionKind = "aggregated"
)

type TraceTransition struct {
	Sequence    int
	ObjectiveID ObjectiveID
	Kind        TransitionKind
}

type RunLimits struct {
	MaxTransitions int
	MaxDepth       int
}

type RunResult struct {
	EvidenceClass               EvidenceClass
	WorkloadID                  WorkloadID
	Objectives                  []Objective
	Artifacts                   []Artifact
	Trace                       []TraceTransition
	DeterministicOperationCalls int
	StationCalls                int
	ModelCalls                  int
	Complete                    bool
}

func (limits RunLimits) validate() error {
	if limits.MaxTransitions < 1 || limits.MaxTransitions > maxTransitions ||
		limits.MaxDepth < 1 || limits.MaxDepth > maxObjectiveDepth {
		return ErrRunBound
	}
	return nil
}

func cloneArtifact(value Artifact) Artifact {
	value.Content = append([]byte{}, value.Content...)
	return value
}

func cloneWorkItem(value WorkItem) WorkItem {
	value.Requirement = cloneRequirement(value.Requirement)
	return value
}
