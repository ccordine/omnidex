package queue

import (
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type CodingPlanLeafWrite struct {
	Leaf                     model.CodingPlanLeaf
	DecisionOriginGeneration int64
	ResultRelation           *assemblyline.ApplicationRequirementCandidateResultRelationResult
}

type StoreCodingPlanReviewCommand struct {
	Authority model.StepAttemptAuthority
	Leaves    []CodingPlanLeafWrite
}

type CodingPlanDecisionChange struct {
	LeafID   model.CodingPlanLeafID   `json:"leaf_id"`
	Decision model.CodingPlanDecision `json:"decision"`
}

type ApplyCodingPlanDecisionsCommand struct {
	OperationID       LifecycleOperationID       `json:"operation_id"`
	JobID             int64                      `json:"job_id"`
	Generation        int64                      `json:"generation"`
	Revision          int64                      `json:"revision"`
	Decisions         []CodingPlanDecisionChange `json:"decisions"`
	WorkspaceRoot     string                     `json:"workspace_root,omitempty"`
	WorkspaceIdentity string                     `json:"workspace_identity,omitempty"`
}

type FreezeCodingPlanCommand struct {
	OperationID       LifecycleOperationID `json:"operation_id"`
	JobID             int64                `json:"job_id"`
	Generation        int64                `json:"generation"`
	Revision          int64                `json:"revision"`
	WorkspaceRoot     string               `json:"workspace_root,omitempty"`
	WorkspaceIdentity string               `json:"workspace_identity,omitempty"`
}

type CodingPlanMutationResult struct {
	Plan    model.CodingPlan
	Job     model.Job
	Applied bool
}

type FrozenCodingPlanLeaf struct {
	Leaf           model.CodingPlanLeaf
	ResultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult
}

type FrozenCodingPlan struct {
	Plan   model.CodingPlan
	Leaves []FrozenCodingPlanLeaf
}
