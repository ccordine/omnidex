package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

// CodingPlanResultRelationReceipt is retained code-owned validation authority.
// It is never included in the user-facing plan or any model-visible context.
type CodingPlanResultRelationReceipt struct {
	Schema                   string
	CandidateSHA256          string
	KindReceiptSHA256        string
	CardinalityReceiptSHA256 string
	Relation                 string
}

type CodingPlanLeafWrite struct {
	Leaf                     model.CodingPlanLeaf
	DecisionOriginGeneration int64
	ResultRelation           *CodingPlanResultRelationReceipt
}

type StoreCodingPlanReviewCommand struct {
	Authority     model.StepAttemptAuthority
	ScopeMode     model.CodingScopeMode
	RequestSHA256 string
	Leaves        []CodingPlanLeafWrite
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
	ResultRelation CodingPlanResultRelationReceipt
}

type FrozenCodingPlan struct {
	Plan   model.CodingPlan
	Leaves []FrozenCodingPlanLeaf
}

func (receipt CodingPlanResultRelationReceipt) validateFor(leaf model.CodingPlanLeaf) error {
	if receipt.Schema == "" || receipt.CandidateSHA256 == "" ||
		receipt.KindReceiptSHA256 == "" || receipt.CardinalityReceiptSHA256 == "" ||
		receipt.Relation == "" {
		return fmt.Errorf("executable coding plan leaf requires a complete result-relation receipt")
	}
	return nil
}
