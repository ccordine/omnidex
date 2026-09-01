package main

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type pendingOperationResolution struct {
	jobID       int64
	operationID queue.LifecycleOperationID
	exactText   string
	action      string
	disposition queue.ChannelSessionTurnDisposition
}

func (session *chatSession) resolvePendingOperation() (*pendingOperationResolution, error) {
	if pendingOperationCount(session) > 1 {
		return nil, fmt.Errorf("interactive client has multiple unresolved operation identities")
	}
	if pending := session.pendingTurn; pending != nil {
		receipt, err := awaitAuthoritativeChatRequest(
			session.ctx,
			session.signals,
			func(requestContext context.Context) (client.SessionTurnReceipt, error) {
				return session.client.SubmitSessionTurn(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					pending.operationID,
					pending.exactText,
				)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("resolve session turn %q: %w", pending.operationID, err)
		}
		return &pendingOperationResolution{
			jobID:       receipt.JobID,
			operationID: pending.operationID,
			exactText:   pending.exactText,
			disposition: receipt.Disposition,
		}, nil
	}
	if pending := session.pendingControl; pending != nil {
		job, err := session.replayPendingControl(pending)
		if err != nil {
			return nil, err
		}
		return &pendingOperationResolution{
			jobID:       job.ID,
			operationID: pending.operationID,
			exactText:   pending.exactText,
			action:      pending.action,
		}, nil
	}
	if pending := session.pendingPlan; pending != nil {
		if err := session.replayPendingPlanMutation(pending, true); err != nil {
			return nil, err
		}
		session.pendingPlan = nil
		return &pendingOperationResolution{
			jobID: pending.jobID, operationID: pending.operationID,
			action: string(pending.kind),
		}, nil
	}
	return nil, nil
}

func (session *chatSession) replayPendingControl(pending *pendingControl) (model.Job, error) {
	job, err := awaitAuthoritativeChatRequest(
		session.ctx,
		session.signals,
		func(requestContext context.Context) (model.Job, error) {
			switch pending.action {
			case "interrupt":
				return session.client.Interrupt(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					pending.jobID,
					pending.operationID,
					pending.exactText,
				)
			case "redirect":
				return session.client.Replan(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					pending.jobID,
					pending.operationID,
					pending.exactText,
				)
			case "cancel":
				return session.client.Cancel(
					requestContext,
					session.channel,
					session.workspaceIdentity,
					pending.jobID,
					pending.operationID,
					pending.exactText,
				)
			default:
				return model.Job{}, fmt.Errorf("unregistered pending control %q", pending.action)
			}
		},
	)
	if err != nil {
		return model.Job{}, fmt.Errorf(
			"resolve /%s operation %q: %w",
			pending.action,
			pending.operationID,
			err,
		)
	}
	if job.ID != pending.jobID {
		return model.Job{}, fmt.Errorf(
			"resolved /%s operation changed job identity from %d to %d",
			pending.action,
			pending.jobID,
			job.ID,
		)
	}
	return job, nil
}

func (session *chatSession) reloadSnapshotAuthoritatively() error {
	snapshot, err := awaitAuthoritativeChatRequest(
		session.ctx,
		session.signals,
		func(requestContext context.Context) (client.ChatSessionSnapshot, error) {
			return session.client.ChatSession(
				requestContext,
				session.channel,
				session.workspaceIdentity,
				client.MaxChatSessionMessages,
			)
		},
	)
	if err != nil {
		return fmt.Errorf("reconcile authoritative CLI chat session: %w", err)
	}
	if err := session.reconcileSnapshot(snapshot, false); err != nil {
		return err
	}
	return session.reconcilePlanReview()
}

func (session *chatSession) acknowledgePendingResolution(
	resolution *pendingOperationResolution,
) error {
	if resolution == nil {
		return nil
	}
	if resolution.action == string(planMutationDecision) ||
		resolution.action == string(planMutationFreeze) {
		return session.renderer.system(
			"job %d · %s resolved", resolution.jobID, resolution.action,
		)
	}
	if resolution.action == "" {
		turn, persisted := session.turns[resolution.operationID]
		if !persisted || turn.JobID != resolution.jobID ||
			turn.Disposition != resolution.disposition || turn.Text != resolution.exactText {
			return fmt.Errorf(
				"resolved session turn %q is absent from persisted session authority",
				resolution.operationID,
			)
		}
		session.pendingTurn = nil
		return session.renderer.system("job %d · %s resolved", resolution.jobID, resolution.disposition)
	}
	control, persisted := session.controls[resolution.operationID]
	if !persisted || control.JobID != resolution.jobID ||
		control.Kind != sessionControlKind(resolution.action) || control.Text != resolution.exactText {
		return fmt.Errorf(
			"resolved /%s operation %q is absent from persisted session authority",
			resolution.action,
			resolution.operationID,
		)
	}
	session.pendingControl = nil
	return session.renderer.system("job %d · %s resolved", resolution.jobID, resolution.action)
}

func pendingOperationCount(session *chatSession) int {
	count := 0
	if session.pendingTurn != nil {
		count++
	}
	if session.pendingControl != nil {
		count++
	}
	if session.pendingPlan != nil {
		count++
	}
	return count
}

func sessionControlKind(action string) queue.ChannelSessionControlKind {
	switch action {
	case "interrupt":
		return queue.ChannelSessionControlInterrupt
	case "redirect":
		return queue.ChannelSessionControlReplan
	case "cancel":
		return queue.ChannelSessionControlCancel
	default:
		return ""
	}
}

func definitiveChatRequestFailure(err error) bool {
	return client.IsDefinitiveMutationRejection(err)
}
