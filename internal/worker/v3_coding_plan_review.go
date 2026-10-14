package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

var errCodingPlanReviewPending = errors.New("coding plan is waiting for user review")

func (r *nativeRuntimeV3) runDirectCodingPlanAction() error {
	configuration, err := codingagent.ConfigurationFromMetadata(r.claim.Job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode == codingagent.Agents {
		return r.runAgentPlanningAction(configuration)
	}
	request, err := r.directCodingRequest()
	if err != nil {
		return err
	}
	// Requirement intake uses only the request and lexical artifact identities.
	// Actual filesystem access belongs to the first source consumer.
	scope, err := workspaceAuthorityForV3Job(r.claim.Job)
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
		var id model.CodingPlanLeafID
		decision := model.CodingPlanDecisionPending
		originGeneration := r.claim.Job.CurrentGeneration
		if retained, exists := prior[proposal.Statement]; exists {
			id = retained.LeafID
			decision = retained.Decision
			originGeneration = retained.OriginGeneration
		} else {
			id, err = model.NewCodingPlanLeafID()
			if err != nil {
				return fmt.Errorf("construct coding plan leaf %d identity: %w", index, err)
			}
		}
		leaf := model.CodingPlanLeaf{
			ID: id, Statement: proposal.Statement,
			Decision: decision,
		}
		write := queue.CodingPlanLeafWrite{
			Leaf: leaf, DecisionOriginGeneration: originGeneration,
		}
		write.ResultRelation = &assemblyline.ApplicationRequirementCandidateResultRelationResult{
			Schema:   proposal.ResultRelation.Schema,
			Relation: proposal.ResultRelation.Relation,
		}
		writes[index] = write
	}
	command := queue.StoreCodingPlanReviewCommand{
		Authority: r.claim.Authority,
		Leaves:    writes,
	}
	var plan model.CodingPlan
	if err := r.transitionAfterSessionMessages(r.ctx, func() error {
		var storeErr error
		plan, storeErr = r.svc.repo.StoreCodingPlanReview(r.ctx, command)
		return storeErr
	}); err != nil {
		return err
	}
	r.svc.emitStepEvent(r.claim.Authority, "coding_plan_review_ready", fmt.Sprintf(
		"generation=%d revision=%d leaves=%d", plan.Generation, plan.Revision, len(plan.Leaves),
	))
	return errCodingPlanReviewPending
}
