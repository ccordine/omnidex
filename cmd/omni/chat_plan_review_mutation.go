package main

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

type planMutationKind string

const (
	planMutationDecision planMutationKind = "coding_plan_decision"
	planMutationFreeze   planMutationKind = "coding_plan_freeze"
)

type pendingPlanMutation struct {
	kind         planMutationKind
	jobID        int64
	generation   int64
	revision     int64
	operationID  model.LifecycleOperationID
	leafID       model.CodingPlanLeafID
	decision     model.CodingPlanDecision
	ledgerAction *client.DirectPlanningAction
	targetID     model.LedgerItemID
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
			leafID: leafID, targetID: model.LedgerItemID(leafID), decision: decision,
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
			pending.leafID == desired.leafID && pending.decision == desired.decision && reflect.DeepEqual(pending.ledgerAction, desired.ledgerAction) {
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
	case planMutationLedger:
		return session.replayLedgerPanelMutation(pending, authoritative)
	case planMutationDecision:
		reviewState := model.LedgerReviewNeedsDiscussion
		if pending.decision == model.CodingPlanDecisionApproved {
			reviewState = model.LedgerReviewAgreed
		}
		turn := client.PlanningTurn{
			OperationID: string(pending.operationID), OwnerTurnID: "terminal-" + string(pending.operationID),
			Text: "Set the displayed item review state.", ObservedRevision: pending.revision,
		}
		request := func(requestContext context.Context) (ledgerapi.CodingLedgerRevision, error) {
			return session.client.MutateCodingLedgerDirect(requestContext, session.channel, session.workspaceIdentity,
				pending.jobID, turn, client.DirectPlanningAction{Kind: model.LedgerMutationReview,
					TargetID: pending.targetID, ReviewState: reviewState, Rationale: "explicit terminal item control"})
		}
		var revision ledgerapi.CodingLedgerRevision
		var err error
		if authoritative {
			revision, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
		} else {
			revision, err = awaitChatRequest(session.ctx, session.signals, request)
		}
		if err != nil {
			return err
		}
		if revision.JobID != pending.jobID || revision.BaseRevision != pending.revision || revision.Revision <= pending.revision {
			return fmt.Errorf("coding ledger decision response changed or failed to advance exact authority")
		}
		plan := session.planReview.snapshot
		plan.Revision = revision.Revision
		for index := range plan.Leaves {
			if plan.Leaves[index].ID == pending.leafID {
				plan.Leaves[index].Decision = pending.decision
			}
		}
		return session.acceptPersistedPlanDecision(pending, plan)
	case planMutationFreeze:
		turn := client.PlanningTurn{
			OperationID: string(pending.operationID), OwnerTurnID: "terminal-" + string(pending.operationID),
			Text: "Sign off the exact displayed planning revision.", ObservedRevision: pending.revision,
		}
		request := func(requestContext context.Context) (ledgerapi.CodingLedgerRevision, error) {
			return session.client.FreezeCodingLedger(requestContext, session.channel, session.workspaceIdentity,
				pending.jobID, turn)
		}
		var receipt ledgerapi.CodingLedgerRevision
		var err error
		if authoritative {
			receipt, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
		} else {
			receipt, err = awaitChatRequest(session.ctx, session.signals, request)
		}
		if err != nil {
			return err
		}
		if receipt.JobID != pending.jobID || receipt.BaseRevision != pending.revision ||
			receipt.Revision != pending.revision+1 || receipt.State != "frozen" {
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
