package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

var errCodingPlanReviewPending = errors.New("coding plan is waiting for user review")

func (r *nativeRuntimeV3) runDirectCodingPlanAction() error {
	request, err := r.directCodingRequest()
	if err != nil {
		return err
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return err
	}
	session := &directCodingSession{
		runtime: r, request: request, root: scope.Root,
		protectedPaths: map[string]struct{}{},
	}
	inputs, err := session.prepareApplicationInputs()
	if err != nil {
		return err
	}
	scopeMode, err := codingScopeModeFromJobMetadata(r.claim.Job.Metadata)
	if err != nil {
		return err
	}
	proposals, err := resolveDirectCodingApplicationPlan(
		inputs.Runtime,
		directCodingApplicationIntentModels{
			Requirements:   inputs.RequirementModel,
			ResultRelation: inputs.ResultRelationModel,
		},
		assemblyline.ApplicationIntentInput{
			UserRequest: inputs.RequestAuthority.modelRequest,
			Context:     inputs.ApplicationContext,
		},
		scopeMode,
		inputs.Identities,
	)
	if err != nil {
		return err
	}
	prior, err := r.svc.repo.PriorCodingPlanDecisions(
		r.ctx, r.claim.Job.ID, r.claim.Job.CurrentGeneration,
	)
	if err != nil {
		return fmt.Errorf("load prior coding plan decisions: %w", err)
	}
	writes := make([]queue.CodingPlanLeafWrite, len(proposals))
	for index, proposal := range proposals {
		id, err := model.NewCodingPlanLeafID(proposal.Statement)
		if err != nil {
			return fmt.Errorf("construct coding plan leaf %d identity: %w", index, err)
		}
		decision := model.CodingPlanDecisionPending
		originGeneration := r.claim.Job.CurrentGeneration
		if retained, exists := prior[id]; exists {
			decision = retained.Decision
			originGeneration = retained.OriginGeneration
		}
		leaf := model.CodingPlanLeaf{
			ID: id, Statement: proposal.Statement, Annotation: proposal.Annotation,
			Decision: decision,
		}
		write := queue.CodingPlanLeafWrite{
			Leaf: leaf, DecisionOriginGeneration: originGeneration,
		}
		write.ResultRelation = &queue.CodingPlanResultRelationReceipt{
			Schema:                   proposal.ResultRelation.Schema,
			CandidateSHA256:          proposal.ResultRelation.CandidateSHA256,
			KindReceiptSHA256:        proposal.ResultRelation.KindReceiptSHA256,
			CardinalityReceiptSHA256: proposal.ResultRelation.CardinalityReceiptSHA256,
			Relation:                 proposal.ResultRelation.Relation,
		}
		writes[index] = write
	}
	plan, err := r.svc.repo.StoreCodingPlanReview(r.ctx, queue.StoreCodingPlanReviewCommand{
		Authority:     r.claim.Authority,
		ScopeMode:     scopeMode,
		RequestSHA256: inputs.RequestAuthority.requestSHA256,
		Leaves:        writes,
	})
	if err != nil {
		return err
	}
	r.svc.emitStepEvent(r.claim.Authority, "coding_plan_review_ready", fmt.Sprintf(
		"generation=%d revision=%d leaves=%d", plan.Generation, plan.Revision, len(plan.Leaves),
	))
	return errCodingPlanReviewPending
}
