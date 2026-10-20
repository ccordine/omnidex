package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
)

const planMutationLedger planMutationKind = "coding_ledger_control"

func (session *chatSession) ledgerWorkspace() *chatWorkspace {
	if session.renderer.console == nil {
		return nil
	}
	return session.renderer.console.workspace
}

func (session *chatSession) ledgerFeedback(text string) error {
	view := session.ledgerWorkspace()
	if view == nil {
		return nil
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	view.ledger.loading = false
	if text != "Server confirmed" {
		view.ledger.offset = 0
	}
	view.ledger.feedback = safeConsoleText(text)
	return view.drawLocked()
}

func (session *chatSession) refreshLedgerPanel() error {
	view := session.ledgerWorkspace()
	if view == nil {
		return nil
	}
	view.mu.Lock()
	opened := view.ledger.opened
	view.mu.Unlock()
	if !opened {
		return nil
	}
	if session.active == nil {
		view.mu.Lock()
		view.ledger.snapshot = ledgerapi.CodingLedgerRevision{}
		view.ledger.selected, view.ledger.offset = "", 0
		view.mu.Unlock()
		return session.ledgerFeedback("No active job")
	}
	jobID, generation := session.active.Job.ID, session.active.Job.CurrentGeneration
	view.mu.Lock()
	if view.ledger.snapshot.JobID != jobID || view.ledger.snapshot.Generation != generation {
		view.ledger.snapshot = ledgerapi.CodingLedgerRevision{}
		view.ledger.selected, view.ledger.offset = "", 0
	}
	view.ledger.loading = true
	view.ledger.feedback = "Loading ledger…"
	drawErr := view.drawLocked()
	view.mu.Unlock()
	if drawErr != nil {
		return drawErr
	}
	snapshot, err := awaitChatRequest(session.ctx, session.signals, func(ctx context.Context) (ledgerapi.CodingLedgerRevision, error) {
		return session.client.CodingLedger(ctx, session.channel, session.workspaceIdentity, jobID)
	})
	if err == nil && snapshot.Generation != generation {
		err = fmt.Errorf("ledger generation differs from current job")
	}
	// The panel keeps the full task payload; rendering must not substitute
	// the goal-only list label for the approved packet.
	if err == nil {
		err = view.acceptLedger(snapshot)
	}
	if err != nil {
		return session.ledgerFeedback(fmt.Sprintf("Ledger refresh failed: %v · r retry", err))
	}
	if session.pendingPlan != nil && session.pendingPlan.kind == planMutationLedger {
		return session.ledgerFeedback("Ledger refreshed; control outcome unresolved · t retries the same operation")
	}
	return nil
}

func (session *chatSession) acceptLedgerPanelRequest(request workspaceLedgerRequest) error {
	if request.key == 'r' {
		return session.refreshLedgerPanel()
	}
	if request.key == 't' {
		if session.pendingPlan == nil || session.pendingPlan.kind != planMutationLedger {
			return session.ledgerFeedback("No unresolved ledger control")
		}
		return session.runLedgerPanelMutation(session.pendingPlan)
	}
	if session.active == nil || session.active.Job.ID != request.jobID || session.active.Job.CurrentGeneration != request.generation {
		return session.ledgerFeedback("Ledger job changed; r refreshes the current job")
	}
	if session.pendingIntent != nil {
		return session.ledgerFeedback("A previous message remains unresolved")
	}
	desired := pendingPlanMutation{kind: planMutationLedger, jobID: request.jobID, generation: request.generation, revision: request.revision, ledgerAction: &request.action}
	pending, err := session.planMutationOperation(desired)
	if err != nil {
		return session.ledgerFeedback(err.Error())
	}
	return session.runLedgerPanelMutation(pending)
}

func (session *chatSession) runLedgerPanelMutation(pending *pendingPlanMutation) error {
	err := session.replayPendingPlanMutation(pending, false)
	if err == nil {
		session.pendingPlan = nil
		// The review editor keeps its selection and input authority. Only its
		// server snapshot changes, using the same existing projection contract.
		if session.planReview != nil && session.planReview.snapshot.JobID == pending.jobID {
			plan, readErr := session.client.CodingPlan(session.ctx, session.channel, session.workspaceIdentity, pending.jobID)
			if readErr != nil {
				return session.ledgerFeedback(fmt.Sprintf("Control saved; review refresh failed: %v", readErr))
			}
			state, _, readErr := reconcilePlanReviewState(*session.planReview, plan)
			if readErr != nil {
				return session.ledgerFeedback(fmt.Sprintf("Control saved; review refresh failed: %v", readErr))
			}
			session.planReview = &state
		}
		return session.ledgerFeedback("Server confirmed")
	}
	if definitiveChatRequestFailure(err) {
		session.pendingPlan = nil
		if refreshErr := session.refreshLedgerPanel(); refreshErr != nil {
			return refreshErr
		}
		label := "Ledger control rejected"
		if client.IsHTTPStatus(err, http.StatusConflict) {
			label = "Ledger conflict; refreshed current state"
		}
		return session.ledgerFeedback(fmt.Sprintf("%s: %v", label, err))
	}
	// Never create a replacement operation after an ambiguous response. The
	// existing pending-plan resolver and explicit retry share this exact identity.
	return session.ledgerFeedback(fmt.Sprintf("Ledger control outcome unresolved: %v · t retries the same operation", err))
}

func (session *chatSession) replayLedgerPanelMutation(pending *pendingPlanMutation, authoritative bool) error {
	if pending.ledgerAction == nil {
		return fmt.Errorf("ledger control lacks its exact action")
	}
	turn := client.PlanningTurn{OperationID: string(pending.operationID), OwnerTurnID: "terminal-" + string(pending.operationID),
		Text: fmt.Sprintf("%s displayed ledger item %s", pending.ledgerAction.Kind, pending.ledgerAction.TargetID), ObservedRevision: pending.revision}
	request := func(ctx context.Context) (ledgerapi.CodingLedgerRevision, error) {
		return session.client.MutateCodingLedgerDirect(ctx, session.channel, session.workspaceIdentity, pending.jobID, turn, *pending.ledgerAction)
	}
	var result ledgerapi.CodingLedgerRevision
	var err error
	if authoritative {
		result, err = awaitAuthoritativeChatRequest(session.ctx, session.signals, request)
	} else {
		result, err = awaitChatRequest(session.ctx, session.signals, request)
	}
	if err != nil {
		return err
	}
	if result.JobID != pending.jobID || result.Generation != pending.generation || result.BaseRevision != pending.revision || result.Revision != pending.revision+1 || result.OperationID != string(pending.operationID) {
		return fmt.Errorf("ledger control response differs from exact operation authority")
	}
	if result.Ledger.Revision != result.Revision {
		return fmt.Errorf("ledger control response has inconsistent revision")
	}
	if err := result.Ledger.ValidateDraft(); err != nil {
		return err
	}
	view := session.ledgerWorkspace()
	if view == nil {
		return fmt.Errorf("ledger workspace unavailable")
	}
	// A replay can resolve after a newer authoritative read. Never regress that read.
	view.mu.Lock()
	current := view.ledger.snapshot
	view.mu.Unlock()
	if current.JobID == result.JobID && current.Generation == result.Generation && current.Revision > result.Revision {
		return session.refreshLedgerPanel()
	}
	return view.acceptLedger(result)
}
