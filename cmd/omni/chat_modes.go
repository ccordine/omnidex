package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func (session *chatSession) setConversationMode(text string) error {
	mode := sessionintent.Mode(strings.ToLower(strings.TrimSpace(text)))
	if mode == "" {
		return session.renderer.system("Modes: Infer, Interact, Ledger, Research. Select with /mode <name>.")
	}
	if err := mode.Validate(); err != nil {
		return err
	}
	if mode == sessionintent.Ledger && session.ledgerWorkspace() == nil {
		return fmt.Errorf("Ledger mode requires the interactive terminal item controls")
	}
	if view := session.ledgerWorkspace(); view != nil {
		view.mu.Lock()
		requestSwitch := view.editorSwitching && view.conversationMode != string(mode)
		view.mu.Unlock()
		if requestSwitch {
			session.renderer.console.reviewInput.requestEditorSwitch(string(mode))
			return nil
		}
	}
	var history []sessionintent.ResearchTurn
	if mode == sessionintent.Research {
		var err error
		history, err = session.client.ResearchConversationHistory(session.ctx, session.channel, session.workspaceIdentity, session.researchConversation)
		if err != nil {
			return err
		}
	}
	session.mode = mode
	if view := session.ledgerWorkspace(); view != nil {
		view.mu.Lock()
		if !view.editorSwitching {
			view.modeOffsets[view.conversationMode] = view.conversationOffset
			view.conversationMode = string(mode)
			view.conversationOffset = view.modeOffsets[string(mode)]
			view.focus = workspaceCompose
			if mode == sessionintent.Ledger {
				view.focus, view.ledger.opened = workspaceLedgerPane, true
			}
		}
		err := view.drawLocked()
		view.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if view := session.ledgerWorkspace(); view == nil || !view.editorSwitching {
		if err := session.renderer.console.SetPrompt(string(mode) + "> "); err != nil {
			return err
		}
	}
	if mode == sessionintent.Research {
		return session.renderResearch(history)
	}
	if mode == sessionintent.Ledger {
		return session.refreshLedgerPanel()
	}
	return nil
}

type researchCompletion struct {
	request sessionintent.ResearchRequest
	turn    sessionintent.ResearchTurn
	history []sessionintent.ResearchTurn
	err     error
}

func (session *chatSession) acceptResearchQuestion(text string) error {
	if err := client.ValidateSessionTurnText(text); err != nil {
		return err
	}
	if session.researchCancel != nil {
		return fmt.Errorf("research is running; /cancel in its conversation stops it")
	}
	if session.pendingResearch != nil && session.pendingResearch.Text != text {
		return fmt.Errorf("retry the unresolved research question before sending another")
	}
	if session.pendingResearch == nil {
		id, err := newOperationID()
		if err != nil {
			return err
		}
		session.pendingResearch = &sessionintent.ResearchRequest{OperationID: id, WorkspaceIdentity: session.workspaceIdentity, ConversationID: session.researchConversation, Text: text}
	}
	request := *session.pendingResearch
	if session.researchResults == nil {
		session.researchResults = make(chan researchCompletion, 1)
	}
	results := session.researchResults
	ctx, cancel := context.WithCancel(session.ctx)
	session.researchCancel = cancel
	// Only this goroutine owns transport; the session loop owns all UI and state.
	go func() {
		result := researchCompletion{request: request}
		result.turn, result.err = session.client.Research(ctx, session.channel, request)
		// Resolve cancellation from durable history without submitting new work.
		if result.err != nil && ctx.Err() != nil && session.ctx.Err() == nil {
			result.turn, result.history, result.err = session.client.ResearchOutcome(session.ctx, session.channel, request, 15*time.Second)
		}
		if result.err == nil && result.history == nil {
			result.history, result.err = session.client.ResearchConversationHistory(ctx, session.channel, session.workspaceIdentity, request.ConversationID)
		}
		cancel()
		select {
		case results <- result:
		case <-session.ctx.Done():
		}
	}()
	return session.renderer.system("Research running: %s. You can switch panes; /cancel stops this Research operation.", request.OperationID)
}

func (session *chatSession) completeResearch(result researchCompletion) error {
	session.researchCancel = nil
	if result.err != nil {
		if definitiveChatRequestFailure(result.err) {
			session.pendingResearch = nil
		}
		return session.renderer.system("Research %s: %v. Retry the same question to resolve its retained operation.", result.request.OperationID, result.err)
	}
	session.pendingResearch = nil
	if result.request.ConversationID == session.researchConversation {
		if err := session.renderResearch(result.history); err != nil {
			return err
		}
	}
	if result.turn.Error != "" {
		return session.renderer.system("Research failed: %s", result.turn.Error)
	}
	return nil
}

func (session *chatSession) cancelSelectedResearch() error {
	if session.pendingResearch == nil || session.pendingResearch.ConversationID != session.researchConversation || session.researchCancel == nil {
		return fmt.Errorf("no running operation in the selected Research conversation")
	}
	session.researchCancel()
	return session.renderer.system("Research cancellation requested: %s", session.pendingResearch.OperationID)
}

func (session *chatSession) renderResearch(history []sessionintent.ResearchTurn) error {
	view := session.ledgerWorkspace()
	lines := []string{"Research " + string(session.researchConversation), "/research-fork messages:<ids>|ledger:<id>:<revision> <question>", "/research-use <conversation-id|main>; /research-share <finding-id> [exact excerpt]", ""}
	for _, turn := range history {
		answer := turn.Answer
		if turn.Error != "" {
			answer = "Failed: " + turn.Error
		}
		lines = append(lines, "Finding "+string(turn.OperationID)+"\nYou: "+safeConsoleText(turn.Question), "", safeConsoleText(answer), "")
	}
	if view == nil {
		return session.renderer.system("%s", strings.Join(lines, "\n"))
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	anchors := view.resizeAnchorsLocked()
	view.researchLines = lines
	view.restoreResizeAnchorsLocked(anchors)
	return view.drawLocked()
}
