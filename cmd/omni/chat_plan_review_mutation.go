package main

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type planMutationKind string

const (
	planMutationDecision planMutationKind = "coding_plan_decision"
	planMutationFreeze   planMutationKind = "coding_plan_freeze"
)

type pendingPlanMutation struct {
	kind        planMutationKind
	jobID       int64
	generation  int64
	revision    int64
	operationID queue.LifecycleOperationID
	leafID      model.CodingPlanLeafID
	decision    model.CodingPlanDecision
}

func (session *chatSession) persistPlanReviewDecision(
	leafID model.CodingPlanLeafID,
	decision model.CodingPlanDecision,
) error {
	if session.planReview == nil {
		return fmt.Errorf("cannot persist a decision without active plan-review authority")
	}
	plan := session.planReview.snapshot
	pending, err := session.planMutationOperation(
		pendingPlanMutation{
			kind: planMutationDecision, jobID: plan.JobID,
			generation: plan.Generation, revision: plan.Revision,
			leafID: leafID, decision: decision,
		},
	)
	if err != nil {
		return err
	}
	if err := session.replayPendingPlanMutation(pending, false); err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingPlan = nil
		}
		return err
	}
	session.pendingPlan = nil
	return nil
}

func (session *chatSession) freezePlanReview() error {
	if session.planReview == nil {
		return fmt.Errorf("cannot freeze a plan without active plan-review authority")
	}
	plan := session.planReview.snapshot
	pending, err := session.planMutationOperation(
		pendingPlanMutation{
			kind: planMutationFreeze, jobID: plan.JobID,
			generation: plan.Generation, revision: plan.Revision,
		},
	)
	if err != nil {
		return err
	}
	if err := session.replayPendingPlanMutation(pending, false); err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingPlan = nil
		}
		return err
	}
	session.pendingPlan = nil
	if err := session.renderer.system(
		"job %d · plan frozen; coding started", pending.jobID,
	); err != nil {
		return err
	}
	return session.reloadSnapshot()
}

func (session *chatSession) planMutationOperation(
	desired pendingPlanMutation,
) (*pendingPlanMutation, error) {
	if session.pendingTurn != nil {
		return nil, fmt.Errorf("session turn %q remains unresolved", session.pendingTurn.operationID)
	}
	if session.pendingControl != nil {
		return nil, fmt.Errorf(
			"/%s operation %q remains unresolved",
			session.pendingControl.action,
			session.pendingControl.operationID,
		)
	}
	if pending := session.pendingPlan; pending != nil {
		if pending.kind == desired.kind && pending.jobID == desired.jobID &&
			pending.generation == desired.generation && pending.revision == desired.revision &&
			pending.leafID == desired.leafID && pending.decision == desired.decision {
			return pending, nil
		}
		return nil, fmt.Errorf(
			"%s operation %q remains unresolved",
			pending.kind,
			pending.operationID,
		)
	}
	operationID, err := newOperationID()
	if err != nil {
		return nil, err
	}
	desired.operationID = operationID
	session.pendingPlan = &desired
	return session.pendingPlan, nil
}

func (session *chatSession) replayPendingPlanMutation(
	pending *pendingPlanMutation,
	authoritative bool,
) error {
	if pending == nil {
		return fmt.Errorf("pending plan mutation is required")
	}
	switch pending.kind {
	case planMutationDecision:
		request := func(requestContext context.Context) (model.CodingPlan, error) {
			return session.client.DecideCodingPlan(
				requestContext,
				session.channel,
				session.workspaceIdentity,
				pending.jobID,
				pending.operationID,
				pending.generation,
				pending.revision,
				[]client.CodingPlanDecisionChange{{
					LeafID: pending.leafID, Decision: pending.decision,
				}},
			)
		}
		var plan model.CodingPlan
		var err error
		if authoritative {
			plan, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
		} else {
			plan, err = awaitChatRequest(session.ctx, session.signals, request)
		}
		if err != nil {
			return err
		}
		return session.acceptPersistedPlanDecision(pending, plan)
	case planMutationFreeze:
		request := func(requestContext context.Context) (client.CodingPlanFreezeReceipt, error) {
			return session.client.FreezeCodingPlan(
				requestContext,
				session.channel,
				session.workspaceIdentity,
				pending.jobID,
				pending.operationID,
				pending.generation,
				pending.revision,
			)
		}
		var receipt client.CodingPlanFreezeReceipt
		var err error
		if authoritative {
			receipt, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
		} else {
			receipt, err = awaitChatRequest(session.ctx, session.signals, request)
		}
		if err != nil {
			return err
		}
		if receipt.Plan.Generation != pending.generation ||
			receipt.Plan.Revision <= pending.revision {
			return fmt.Errorf("coding plan freeze response changed or failed to advance exact authority")
		}
		return session.clearPlanReview()
	default:
		return fmt.Errorf("pending plan mutation kind %q is unsupported", pending.kind)
	}
}

func (session *chatSession) acceptPersistedPlanDecision(
	pending *pendingPlanMutation,
	plan model.CodingPlan,
) error {
	if plan.JobID != pending.jobID || plan.Generation != pending.generation ||
		plan.Revision <= pending.revision || plan.State != model.CodingPlanStateReview {
		return fmt.Errorf("coding plan decision response changed or failed to advance exact authority")
	}
	matched := false
	for _, leaf := range plan.Leaves {
		if leaf.ID == pending.leafID {
			matched = true
			if leaf.Decision != pending.decision {
				return fmt.Errorf(
					"coding plan decision response retained %q for leaf %q, expected %q",
					leaf.Decision,
					leaf.ID,
					pending.decision,
				)
			}
			break
		}
	}
	if !matched {
		return fmt.Errorf("coding plan decision response removed leaf %q", pending.leafID)
	}
	if session.planReview == nil {
		state, err := newPlanReviewState(plan)
		if err != nil {
			return err
		}
		session.planReview = &state
	} else {
		state, _, err := reconcilePlanReviewState(*session.planReview, plan)
		if err != nil {
			return err
		}
		session.planReview = &state
	}
	return session.showPlanReview()
}
