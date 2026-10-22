package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

type userMessageOutcome struct {
	proposalID model.LifecycleOperationID
	text       string
	receipt    sessionintent.Receipt
	executed   bool
	quit       bool
}

type pendingUserIntent struct{ request sessionintent.Request }

// Infer and Interact share the persisted proposal boundary. Interact binds the
// known recipient before transport; only constrained answers need interpretation.
func (session *chatSession) acceptUserMessage(text string) (resultErr error) {
	if session.mode == sessionintent.Research {
		return session.acceptResearchQuestion(text)
	}
	if session.mode == sessionintent.Ledger {
		return fmt.Errorf("Ledger mode uses explicit item controls; /mode interact sends a message")
	}
	if err := client.ValidateSessionTurnText(text); err != nil {
		return err
	}
	if pending := session.pendingIntent; pending != nil && pending.request.Text != text {
		return fmt.Errorf("user message %q remains unresolved", pending.request.OperationID)
	}
	if session.pendingIntent == nil {
		if pendingOperationCount(session) > 0 {
			return fmt.Errorf("a previous operation remains unresolved")
		}
		id, err := newOperationID()
		if err != nil {
			return err
		}
		request := sessionintent.Request{OperationID: id, Text: text, State: session.serverState, Mode: session.mode}
		if session.mode == sessionintent.Interact {
			planning, err := activeJobRequiresPlanReview(session.active)
			if err != nil {
				return err
			}
			current := sessionintent.Context{Planning: planning}
			if session.active != nil {
				current.InputID = session.active.ExecutionInputID
			}
			if session.lastPendingIntent != "" {
				current.Pending = &sessionintent.Proposal{ID: session.lastPendingIntent}
			}
			request.Recipient = sessionintent.InteractionRecipient(current)
		}
		if session.planReview != nil {
			request.LedgerRevision = session.planReview.snapshot.Revision
			if session.planNoteSubject != "" && session.planReview.selected < len(session.planReview.snapshot.Leaves) {
				request.SelectedItem = model.LedgerItemID(session.planReview.snapshot.Leaves[session.planReview.selected].ID)
			}
		}
		session.pendingIntent = &pendingUserIntent{request: request}
	}
	if session.renderer.console != nil {
		label := "Interpreting reply"
		if session.mode == sessionintent.Interact {
			label = "Sending reply"
		}
		activity := chatActivity{Label: label, Working: true, StartedAt: time.Now()}
		if session.active != nil {
			activity.JobID = session.active.Job.ID
			activity.Generation = session.active.Job.CurrentGeneration
		}
		if err := session.renderer.console.SetActivity(activity); err != nil {
			return err
		}
		defer func() {
			if !session.exitRequested {
				resultErr = errors.Join(resultErr, session.renderer.prompt(session.active))
			}
		}()
	}
	session.planNoteSubmitting = true
	defer func() { session.planNoteSubmitting = false }()
	summary, err := awaitChatRequest(session.ctx, session.signals, func(ctx context.Context) (userMessageOutcome, error) {
		return session.submitUserIntent(ctx, session.pendingIntent)
	})
	if err != nil {
		if definitiveChatRequestFailure(err) || client.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			session.pendingIntent = nil
		}
		return err
	}
	session.rememberUserIntentEcho(summary.receipt, text)
	session.pendingIntent = nil
	session.exitRequested = summary.quit
	session.lastPendingIntent = summary.proposalID
	if summary.executed && session.planNoteEditing {
		if err := session.endPlanReviewNoteInput(); err != nil {
			return err
		}
		session.planNoteEditing = false
		session.planNoteSubject = ""
	}
	if err := session.renderer.system("%s", summary.text); err != nil {
		return err
	}
	session.planNoteSubmitting = false
	if !summary.executed || summary.quit {
		return nil
	}
	return session.reloadSnapshot()
}

func (session *chatSession) submitUserIntent(ctx context.Context, pending *pendingUserIntent) (userMessageOutcome, error) {
	if pending == nil {
		return userMessageOutcome{}, fmt.Errorf("user message lacks pending authority")
	}
	result, err := session.client.InterpretUserMessage(ctx, session.channel, pending.request)
	if err != nil {
		return userMessageOutcome{}, err
	}
	if result.Clarification != "" {
		return userMessageOutcome{text: result.Clarification}, nil
	}
	if result.RequiresApproval {
		return userMessageOutcome{text: result.Pitch, proposalID: result.OperationID}, nil
	}
	receipt, err := session.client.ExecuteUserMessage(ctx, session.channel, session.workspaceIdentity, pending.request.OperationID)
	if err != nil {
		return userMessageOutcome{}, err
	}
	return userMessageOutcome{text: userIntentReceiptText(receipt), receipt: receipt, executed: true, quit: receipt.ExitClient}, nil
}

func (session *chatSession) resolvePendingUserIntent() error {
	pending := session.pendingIntent
	receipt, err := awaitAuthoritativeChatRequest(session.ctx, session.signals, func(ctx context.Context) (sessionintent.Receipt, error) {
		return session.client.CancelUserMessage(ctx, session.channel, pending.request)
	})
	if err != nil {
		return err
	}
	session.rememberUserIntentEcho(receipt, pending.request.Text)
	session.pendingIntent = nil
	if session.planNoteEditing {
		if err := session.endPlanReviewNoteInput(); err != nil {
			return err
		}
		session.planNoteEditing = false
		session.planNoteSubject = ""
	}
	if err := session.renderer.system("%s", userIntentReceiptText(receipt)); err != nil {
		return err
	}
	if err := session.reloadSnapshotAuthoritatively(); err != nil {
		return err
	}
	if receipt.ExitClient {
		session.exitRequested = true
		return nil
	}
	if receipt.Interrupted || session.active == nil || session.active.Job.ID != receipt.JobID || session.active.Job.Status != model.JobStatusRunning && session.active.Job.Status != model.JobStatusPending {
		return nil
	}
	_, err = session.control("interrupt", ctrlCInterruptReason, false, &receipt.JobID)
	return err
}

// Display the receipt's target without changing its authoritative summary.
func userIntentReceiptText(receipt sessionintent.Receipt) string {
	if receipt.JobID > 0 {
		return fmt.Sprintf("job %d · %s", receipt.JobID, receipt.Summary)
	}
	return receipt.Summary
}
