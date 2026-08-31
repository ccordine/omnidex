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
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
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
	ctx               context.Context
	cancel            context.CancelFunc
	client            *client.Client
	channel           model.Channel
	workspaceIdentity string
	renderer          chatRenderer
	active            *model.JobDetails
	messages          map[int64]model.ChannelMessage
	turns             map[queue.LifecycleOperationID]queue.ChannelSessionTurn
	controls          map[queue.LifecycleOperationID]queue.ChannelSessionControl
	pendingControl    *pendingControl
	pendingTurn       *pendingSessionTurn
	signals           <-chan os.Signal
	snapshotRevision  uint64
	realtimeCursor    uint64
	stateRevision     string
	lastPollError     string
}

type pendingSessionTurn struct {
	exactText   string
	operationID queue.LifecycleOperationID
}

func runChatSession(config chatSessionConfig) (resultErr error) {
	if config.Context == nil || config.Cancel == nil || config.Client == nil || config.Stream == nil ||
		config.WorkspaceIdentity == "" ||
		config.Console == nil || config.Signals == nil {
		return fmt.Errorf("interactive chat requires context, client, console, signals, and realtime stream")
	}
	session := &chatSession{
		ctx: config.Context, cancel: config.Cancel, client: config.Client, channel: config.Channel,
		workspaceIdentity: config.WorkspaceIdentity,
		renderer:          chatRenderer{console: config.Console},
		messages:          make(map[int64]model.ChannelMessage),
		turns:             make(map[queue.LifecycleOperationID]queue.ChannelSessionTurn),
		controls:          make(map[queue.LifecycleOperationID]queue.ChannelSessionControl),
		signals:           config.Signals,
	}
	if err := session.renderer.banner(config.Channel); err != nil {
		return err
	}
	if err := session.reconcileSnapshot(config.Snapshot, true); err != nil {
		return err
	}
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

	for {
		select {
		case <-config.Context.Done():
			return config.Context.Err()
		case signal := <-config.Signals:
			if signal == syscall.SIGTERM {
				config.Cancel()
				return nil
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
			quit, err := session.acceptInput(input.Text, input.Pasted)
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
			if result.state.Revision == session.stateRevision {
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
	if pasted {
		if strings.TrimSpace(line) == "" {
			return false, nil
		}
		return false, session.acceptText(line)
	}
	name, text, command := parseChatCommand(line)
	if !command {
		if strings.TrimSpace(text) == "" {
			return false, nil
		}
		return false, session.acceptText(text)
	}
	switch name {
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
	case "interrupt", "redirect", "cancel":
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

func (session *chatSession) acceptText(text string) error {
	operationID, err := session.sessionTurnOperationID(text)
	if err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingTurn = nil
		}
		return err
	}
	receipt, err := awaitChatRequest(
		session.ctx,
		session.signals,
		func(requestContext context.Context) (client.SessionTurnReceipt, error) {
			return session.client.SubmitSessionTurn(
				requestContext,
				session.channel,
				session.workspaceIdentity,
				operationID,
				text,
			)
		},
	)
	if err != nil {
		if definitiveChatRequestFailure(err) {
			session.pendingTurn = nil
		}
		return err
	}
	if err := session.renderer.system(
		"job %d · %s",
		receipt.JobID,
		receipt.Disposition,
	); err != nil {
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
) (queue.LifecycleOperationID, error) {
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
	operationID, err := newOperationID()
	if err != nil {
		return "", err
	}
	session.pendingTurn = &pendingSessionTurn{exactText: exactText, operationID: operationID}
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
	case "step_start", "step_complete", "step_failed", "step_authority_lost", "step_canceled":
		return true
	default:
		return false
	}
}

func (session *chatSession) reloadSnapshot() error {
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
	return session.reconcileSnapshot(snapshot, false)
}

func terminalJob(status string) bool {
	return status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled
}
