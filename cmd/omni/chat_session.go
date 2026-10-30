package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

type chatSessionConfig struct {
	Context           context.Context
	Cancel            context.CancelFunc
	Client            *client.Client
	Channel           model.Channel
	WorkspaceIdentity string
	Snapshot          client.ChatSessionSnapshot
	Stream            *client.JobEventStream
	Console           *chatConsole
	Signals           <-chan os.Signal
}

type chatSession struct {
	mode                 sessionintent.Mode
	pendingResearch      *sessionintent.ResearchRequest
	researchConversation model.LifecycleOperationID
	researchCancel       context.CancelFunc
	researchResults      chan researchCompletion
	ctx                  context.Context
	cancel               context.CancelFunc
	client               *client.Client
	channel              model.Channel
	workspaceIdentity    string
	renderer             chatRenderer
	active               *model.JobDetails
	messages             map[int64]model.ChannelMessage
	turns                map[model.LifecycleOperationID]model.ChannelSessionTurn
	controls             map[model.LifecycleOperationID]model.ChannelSessionControl
	localEchoes          map[string]string
	lastPendingIntent    model.LifecycleOperationID
	exitRequested        bool
	pendingIntent        *pendingUserIntent
	pendingControl       *pendingControl
	pendingTurn          *pendingSessionTurn
	pendingPlan          *pendingPlanMutation
	planReview           *planReviewState
	planReviewInputs     <-chan planReviewInput
	planNoteEditing      bool
	planNoteSubmitting   bool
	planNoteSubject      string
	planNoteAuthority    terminalInputAuthority
	signals              <-chan os.Signal
	snapshotRevision     uint64
	realtimeCursor       uint64
	serverState          model.ChannelSessionState
	lastPollError        string
}

type pendingSessionTurn struct {
	exactText        string
	operationID      model.LifecycleOperationID
	codingMode       codingagent.Mode
	activeJob        *pendingChatJob
	executionInputID *int64
}

func runChatSession(config chatSessionConfig) (resultErr error) {
	if config.Context == nil || config.Cancel == nil || config.Client == nil || config.Stream == nil ||
		config.WorkspaceIdentity == "" ||
		config.Console == nil || config.Signals == nil {
		return fmt.Errorf("interactive chat requires context, client, console, signals, and realtime stream")
	}
	session := &chatSession{
		mode: sessionintent.Infer,
		ctx:  config.Context, cancel: config.Cancel, client: config.Client, channel: config.Channel,
		workspaceIdentity: config.WorkspaceIdentity,
		renderer:          chatRenderer{console: config.Console},
		messages:          make(map[int64]model.ChannelMessage),
		turns:             make(map[model.LifecycleOperationID]model.ChannelSessionTurn),
		controls:          make(map[model.LifecycleOperationID]model.ChannelSessionControl),
		signals:           config.Signals,
	}
	if err := session.renderer.banner(config.Channel); err != nil {
		return err
	}
	if err := session.reconcileSnapshot(config.Snapshot, true); err != nil {
		return err
	}
	if err := session.reconcilePlanReview(); err != nil {
		return err
	}
	activityErrors, stopActivity := startChatActivityDisplay(config.Context, config.Console)
	defer stopActivity()
	inputs, inputDone := readChatInput(config.Context, config.Console)
	defer func() {
		config.Cancel()
		if config.Console.IsTerminal() {
			<-inputDone
		}
	}()
	stream := followJobEvents(
		config.Context,
		config.Client,
		config.Channel.ID,
		config.WorkspaceIdentity,
		config.Stream,
	)
	statePollTicker := time.NewTicker(5 * time.Second)
	defer statePollTicker.Stop()
	statePollResults := make(chan chatStatePollResult, 1)
	statePollActive := false

	var modeRequests <-chan string
	var ledgerRequests <-chan workspaceLedgerRequest
	if view := session.ledgerWorkspace(); view != nil {
		ledgerRequests = view.ledger.requests
		modeRequests = view.modeRequests
	}
	for {
		select {
		case result := <-session.researchResults:
			if err := session.completeResearch(result); err != nil {
				return err
			}
		case mode := <-modeRequests:
			view := session.ledgerWorkspace()
			view.mu.Lock()
			currentMode := view.conversationMode
			view.mu.Unlock()
			if mode != currentMode {
				continue
			}
			if err := session.setConversationMode(mode); err != nil {
				if err := session.renderer.system("%v", err); err != nil {
					return err
				}
			}
		case request := <-ledgerRequests:
			if err := session.acceptLedgerPanelRequest(request); err != nil {
				return err
			}
		case err := <-activityErrors:
			return err
		case <-config.Context.Done():
			return config.Context.Err()
		case signal := <-config.Signals:
			if signal == syscall.SIGTERM {
				config.Cancel()
				return nil
			}
			if view := session.ledgerWorkspace(); view != nil && view.interruptLedgerEditor() {
				continue
			}
			if session.mode == sessionintent.Research && session.researchCancel != nil {
				if err := session.cancelSelectedResearch(); err != nil {
					if err := session.renderer.system("%v", err); err != nil {
						return err
					}
				}
				continue
			}
			// A prior transport failure may have left an idempotent turn or
			// control outcome unknown. Resolve that exact operation first, then
			// bind this interrupt to the resulting authoritative active job.
			quit, err := session.resolveOperationError(errChatRequestInterrupted)
			if err != nil {
				quit, err = session.resolveOperationError(err)
			}
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
		case input, ok := <-inputs:
			if !ok || input.EOF {
				return nil
			}
			if input.Err != nil {
				if errors.Is(input.Err, errChatInputRejected) {
					if err := session.renderer.system("%v", input.Err); err != nil {
						return err
					}
					continue
				}
				return fmt.Errorf("read interactive input: %w", input.Err)
			}
			quit, err := session.acceptAuthorizedInput(input)
			if err != nil {
				stop, handledErr := session.resolveOperationError(err)
				if handledErr != nil {
					return handledErr
				}
				if stop {
					return nil
				}
			}
			if quit {
				return nil
			}
		case input, ok := <-session.planReviewInputs:
			if !ok {
				session.planReviewInputs = nil
				continue
			}
			if input.EOF {
				return nil
			}
			if input.Err != nil {
				if errors.Is(input.Err, errPlanReviewInputInactive) ||
					errors.Is(input.Err, context.Canceled) && config.Context.Err() != nil {
					session.planReviewInputs = nil
					continue
				}
				return fmt.Errorf("read plan review input: %w", input.Err)
			}
			if err := session.acceptPlanReviewKey(input.Key); err != nil {
				stop, handledErr := session.resolveOperationError(err)
				if handledErr != nil {
					return handledErr
				}
				if stop {
					return nil
				}
			}
		case <-statePollTicker.C:
			if !statePollActive {
				statePollActive = true
				startChatStatePoll(
					config.Context,
					config.Client,
					config.Channel,
					config.WorkspaceIdentity,
					session.snapshotRevision,
					statePollResults,
				)
			}
		case result := <-statePollResults:
			statePollActive = false
			if result.snapshotRevision != session.snapshotRevision {
				continue
			}
			if result.err != nil {
				if config.Context.Err() != nil {
					continue
				}
				message := result.err.Error()
				if message != session.lastPollError {
					session.lastPollError = message
					if err := session.renderer.system("persisted session refresh failed: %v", result.err); err != nil {
						return err
					}
				}
				continue
			}
			session.lastPollError = ""
			if result.state.Equal(session.serverState) {
				if err := session.refreshLedgerPanel(); err != nil {
					return err
				}
				continue
			}
			if err := session.reloadSnapshot(); err != nil {
				stop, handledErr := session.resolveOperationError(err)
				if handledErr != nil {
					return handledErr
				}
				if stop {
					return nil
				}
			}
		case update, ok := <-stream:
			if !ok {
				if config.Context.Err() != nil {
					return config.Context.Err()
				}
				return fmt.Errorf("Omnidex realtime stream stopped")
			}
			if update.Err != nil {
				if err := session.renderer.system("%v", update.Err); err != nil {
					return err
				}
				if update.Fatal {
					return update.Err
				}
				continue
			}
			if err := session.acceptRealtime(update.Event); err != nil {
				stop, handledErr := session.resolveOperationError(err)
				if handledErr != nil {
					return handledErr
				}
				if stop {
					return nil
				}
			}
		}
	}
}

func (session *chatSession) acceptInput(line string, pasted bool) (bool, error) {
	name, text, command := parseChatCommand(line)
	if session.pendingIntent != nil {
		modeControl := command && !pasted && name == "mode"
		if !modeControl && (command && !pasted && name != "exit" || line != session.pendingIntent.request.Text && name != "exit") {
			return false, fmt.Errorf("a previous user message remains unresolved")
		}
	}
	if session.planNoteEditing && session.mode != sessionintent.Research && session.mode != sessionintent.Ledger {
		if pasted || !command {
			err := session.acceptPlanReviewNote(line)
			return session.exitRequested, err
		}
		switch name {
		case "mode":
			return false, session.setConversationMode(text)
		case "stop", "cancel", "interrupt", "redirect":
			return session.acceptPlanReviewControl(line)
		}
	}
	if pasted {
		if strings.TrimSpace(line) == "" {
			return false, nil
		}
		err := session.acceptUserMessage(line)
		return session.exitRequested, err
	}
	if !command {
		if strings.TrimSpace(text) == "" {
			return false, nil
		}
		err := session.acceptUserMessage(text)
		return session.exitRequested, err
	}
	switch name {
	case "research-fork", "research-use", "research-share":
		return false, session.researchCommand(name, text)
	case "mode":
		return false, session.setConversationMode(text)
	case "exit":
		if strings.TrimSpace(text) != "" {
			return false, fmt.Errorf("/exit does not accept arguments")
		}
		return true, nil
	case "help":
		if strings.TrimSpace(text) != "" {
			return false, fmt.Errorf("/help does not accept arguments")
		}
		return false, printChatHelp(session.renderer)
	case "status":
		if strings.TrimSpace(text) != "" {
			return false, fmt.Errorf("/status does not accept arguments")
		}
		if err := session.reloadSnapshot(); err != nil {
			return false, err
		}
		if session.active == nil {
			return false, session.renderer.system("no active job")
		}
		return false, session.renderer.status(*session.active)
	case "diagnostics":
		return false, session.setChatDiagnostics(text)
	case "verbosity":
		return false, session.setChatVerbosity(text)
	case "cancel", "stop":
		if session.mode == sessionintent.Research {
			return false, session.cancelSelectedResearch()
		}
		return false, session.stopCurrentJob(text)
	case "interrupt", "redirect":
		expectedJobID := int64(0)
		if session.active != nil {
			expectedJobID = session.active.Job.ID
		}
		_, err := session.control(name, text, true, &expectedJobID)
		return false, err
	default:
		return false, fmt.Errorf("unknown command /%s; use /help", name)
	}
}

func (session *chatSession) acceptAuthorizedInput(input chatInput) (bool, error) {
	if input.Mode != "" {
		if err := input.Mode.Validate(); err != nil {
			return false, err
		}
		session.mode = input.Mode
		defer func() {
			if view := session.ledgerWorkspace(); view != nil {
				view.mu.Lock()
				session.mode = sessionintent.Mode(view.conversationMode)
				view.mu.Unlock()
			}
		}()
	}
	current := session.renderer.console.CurrentInputAuthority()
	if session.mode == sessionintent.Research || session.mode == sessionintent.Ledger {
		if input.Authority.Generation != current.Generation || input.Authority.Mode != terminalInputOrdinary {
			return false, fmt.Errorf("%w: stale mode input", errChatInputRejected)
		}
		return session.acceptInput(input.Text, input.Pasted)
	}
	if session.planNoteEditing {
		if input.Authority != session.planNoteAuthority ||
			input.Authority.Mode != terminalInputPlanNote {
			return false, fmt.Errorf("%w: input does not belong to the active plan-note editor", errChatInputRejected)
		}
		return session.acceptInput(input.Text, input.Pasted)
	}
	if input.Authority.Mode != terminalInputOrdinary || input.Authority != current ||
		session.planReview != nil || session.planNoteSubmitting {
		return false, fmt.Errorf("%w: stale or plan-bound input was not submitted as a chat turn", errChatInputRejected)
	}
	return session.acceptInput(input.Text, input.Pasted)
}

func (session *chatSession) acceptText(text string) error {
	operationID, err := session.sessionTurnOperationID(text)
	if err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingTurn = nil
		}
		return err
	}
	pending := session.pendingTurn
	receipt, err := awaitChatRequest(
		session.ctx,
		session.signals,
		func(requestContext context.Context) (client.SessionTurnReceipt, error) {
			return session.submitPendingTurn(requestContext, pending)
		},
	)
	if err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingTurn = nil
		}
		return err
	}
	if err := session.renderPendingTurnAccepted(receipt, pending); err != nil {
		return err
	}
	if err := session.reloadSnapshot(); err != nil {
		return err
	}
	if _, persisted := session.turns[operationID]; !persisted {
		return fmt.Errorf("session turn %q was accepted but is absent from persisted session state", operationID)
	}
	session.pendingTurn = nil
	return nil
}

func (session *chatSession) sessionTurnOperationID(
	exactText string,
) (model.LifecycleOperationID, error) {
	if err := client.ValidateSessionTurnText(exactText); err != nil {
		return "", err
	}
	if session.pendingTurn != nil && session.pendingTurn.exactText == exactText {
		return session.pendingTurn.operationID, nil
	}
	if session.pendingTurn != nil {
		return "", fmt.Errorf("session turn %q remains unresolved", session.pendingTurn.operationID)
	}
	if session.pendingControl != nil {
		return "", fmt.Errorf("/%s operation %q remains unresolved", session.pendingControl.action, session.pendingControl.operationID)
	}
	if session.pendingPlan != nil {
		return "", fmt.Errorf(
			"%s operation %q remains unresolved",
			session.pendingPlan.kind,
			session.pendingPlan.operationID,
		)
	}
	operationID, err := newOperationID()
	if err != nil {
		return "", err
	}
	session.pendingTurn = &pendingSessionTurn{
		exactText: exactText, operationID: operationID, codingMode: codingagent.Agents,
	}
	if session.active != nil && session.active.ExecutionInputID != nil {
		id := *session.active.ExecutionInputID
		session.pendingTurn.executionInputID = &id
	}
	if session.active != nil && (session.active.Job.Status == model.JobStatusPending || session.active.Job.Status == model.JobStatusRunning) {
		session.pendingTurn.activeJob = &pendingChatJob{jobID: session.active.Job.ID, generation: session.active.Job.CurrentGeneration}
	}
	return operationID, nil
}

func (session *chatSession) acceptRealtime(event client.RealtimeEvent) error {
	if event.EventName == client.RealtimeConnected {
		if event.SyncRequired {
			if err := session.renderer.system("realtime replay gap; reloading persisted session state"); err != nil {
				return err
			}
			return session.reloadSnapshot()
		}
		return nil
	}
	if event.ChannelID != "" && event.ChannelID != session.channel.ID {
		return nil
	}
	if event.ID <= session.realtimeCursor && realtimeEventReflectedBySnapshot(event) {
		return nil
	}
	if detail, changed := session.renderer.eventActivity(event); changed {
		if err := session.renderer.console.SetActivityDetail(event.JobID, event.StepID, detail); err != nil {
			return err
		}
	}
	if err := session.renderer.realtime(event); err != nil {
		return err
	}
	if event.EventName == client.RealtimeJobProgress ||
		event.EventName == client.RealtimeJobRuntimeEvent && runtimeEventChangesJobState(event.RuntimeEvent) {
		return session.reloadSnapshot()
	}
	return nil
}

func realtimeEventReflectedBySnapshot(event client.RealtimeEvent) bool {
	return event.EventName == client.RealtimeJobProgress ||
		event.EventName == client.RealtimeJobRuntimeEvent &&
			runtimeEventChangesJobState(event.RuntimeEvent)
}

func runtimeEventChangesJobState(kind string) bool {
	switch kind {
	case "step_start", "step_complete", "step_failed", "step_authority_lost", "step_canceled",
		"step_waiting_input", "coding_plan_review_ready", "session_message_resolved":
		return true
	default:
		return false
	}
}

func (session *chatSession) reloadSnapshot() error {
	if err := session.reloadSnapshotAuthority(); err != nil {
		return err
	}
	return session.reconcilePlanReview()
}

func (session *chatSession) reloadSnapshotAuthority() error {
	snapshot, err := awaitChatRequest(
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
		return fmt.Errorf("reload CLI chat session: %w", err)
	}
	if err := session.reconcileSnapshot(snapshot, false); err != nil {
		return err
	}
	return nil
}

func terminalJob(status string) bool {
	return status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled
}
