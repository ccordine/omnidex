package main

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type chatSnapshotEntryKind int

const (
	chatSnapshotMessage chatSnapshotEntryKind = iota
	chatSnapshotTurn
	chatSnapshotControl
	chatSnapshotTerminal
)

type chatSnapshotEntry struct {
	createdAt time.Time
	key       string
	kind      chatSnapshotEntryKind
	message   model.ChannelMessage
	turn      queue.ChannelSessionTurn
	control   queue.ChannelSessionControl
	terminal  model.ChannelMessage
}

func (session *chatSession) reconcileSnapshot(
	snapshot client.ChatSessionSnapshot,
	initial bool,
) error {
	if snapshot.Channel.ID != session.channel.ID ||
		snapshot.Channel.WorkspaceRoot != session.channel.WorkspaceRoot {
		return fmt.Errorf("session snapshot differs from exact CLI channel authority")
	}
	if err := session.requireSnapshotContinuity(snapshot); err != nil {
		return err
	}

	entries := make([]chatSnapshotEntry, 0)
	enqueued := enqueuedTurnsByJob(snapshot.Turns)
	for _, message := range snapshot.Messages {
		previous, seen := session.messages[message.ID]
		if seen {
			changed, err := changedMessageTurn(previous, message)
			if err != nil {
				return err
			}
			if changed && terminalMessageTurn(message) {
				entries = append(entries, terminalSnapshotEntry(message))
			}
		} else {
			if !session.suppressLocalEnqueuedMessage(message, enqueued) {
				entries = append(entries, chatSnapshotEntry{
					createdAt: message.CreatedAt,
					key:       fmt.Sprintf("message:%020d", message.ID),
					kind:      chatSnapshotMessage,
					message:   message,
				})
			}
			if terminalMessageTurn(message) {
				entries = append(entries, terminalSnapshotEntry(message))
			}
		}
		session.messages[message.ID] = message
	}
	for _, turn := range snapshot.Turns {
		previous, seen := session.turns[turn.OperationID]
		if seen && previous != turn {
			return fmt.Errorf("persisted session turn %q changed", turn.OperationID)
		}
		if !seen && !session.suppressSessionTurn(turn, snapshot.Messages) {
			entries = append(entries, chatSnapshotEntry{
				createdAt: turn.CreatedAt,
				key:       "turn:" + string(turn.OperationID),
				kind:      chatSnapshotTurn,
				turn:      turn,
			})
		}
		session.turns[turn.OperationID] = turn
	}
	for _, control := range snapshot.Controls {
		previous, seen := session.controls[control.OperationID]
		if seen && previous != control {
			return fmt.Errorf("persisted session control %q changed", control.OperationID)
		}
		if !seen && !session.suppressSessionControl(control) {
			entries = append(entries, chatSnapshotEntry{
				createdAt: control.CreatedAt,
				key:       "control:" + string(control.OperationID),
				kind:      chatSnapshotControl,
				control:   control,
			})
		}
		session.controls[control.OperationID] = control
	}

	if initial {
		if err := session.renderer.resumed(
			len(entries),
			snapshot.HasMore,
			snapshot.TurnsTruncated,
			snapshot.ControlsTruncated,
		); err != nil {
			return err
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].createdAt.Equal(entries[right].createdAt) {
			if entries[left].kind == entries[right].kind {
				return entries[left].key < entries[right].key
			}
			return entries[left].kind < entries[right].kind
		}
		return entries[left].createdAt.Before(entries[right].createdAt)
	})
	for _, entry := range entries {
		var err error
		switch entry.kind {
		case chatSnapshotMessage:
			err = session.renderer.message(entry.message)
		case chatSnapshotTurn:
			err = session.renderer.sessionTurn(entry.turn)
		case chatSnapshotControl:
			err = session.renderer.sessionControl(entry.control)
		case chatSnapshotTerminal:
			err = session.renderer.messageTurnState(entry.terminal)
		default:
			err = fmt.Errorf("session snapshot contains an unknown presentation entry")
		}
		if err != nil {
			return err
		}
	}
	if err := session.reconcileActiveJob(snapshot.ActiveJob, initial); err != nil {
		return err
	}
	if err := session.reconcilePersistedPendingOperations(); err != nil {
		return err
	}
	session.retainSnapshotAuthority(snapshot)
	if err := session.renderer.prompt(session.active); err != nil {
		return err
	}
	session.realtimeCursor = snapshot.RealtimeCursor
	session.stateRevision = snapshot.Revision
	session.snapshotRevision++
	return nil
}

func terminalSnapshotEntry(message model.ChannelMessage) chatSnapshotEntry {
	return chatSnapshotEntry{
		createdAt: message.Turn.UpdatedAt,
		key:       fmt.Sprintf("terminal:%020d", message.Turn.JobID),
		kind:      chatSnapshotTerminal,
		terminal:  message,
	}
}

func (session *chatSession) requireSnapshotContinuity(snapshot client.ChatSessionSnapshot) error {
	messageIDs := make(map[int64]struct{}, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		messageIDs[message.ID] = struct{}{}
	}
	if err := requireHistoryContinuity(session.messages, messageIDs, snapshot.HasMore, "transcript"); err != nil {
		return err
	}
	turnIDs := make(map[queue.LifecycleOperationID]struct{}, len(snapshot.Turns))
	for _, turn := range snapshot.Turns {
		turnIDs[turn.OperationID] = struct{}{}
	}
	if err := requireHistoryContinuity(session.turns, turnIDs, snapshot.TurnsTruncated, "turn history"); err != nil {
		return err
	}
	controlIDs := make(map[queue.LifecycleOperationID]struct{}, len(snapshot.Controls))
	for _, control := range snapshot.Controls {
		controlIDs[control.OperationID] = struct{}{}
	}
	return requireHistoryContinuity(
		session.controls,
		controlIDs,
		snapshot.ControlsTruncated,
		"control history",
	)
}

func requireHistoryContinuity[K comparable, V any](
	known map[K]V,
	current map[K]struct{},
	truncated bool,
	label string,
) error {
	if len(known) == 0 {
		return nil
	}
	overlap := false
	for key := range known {
		if _, present := current[key]; present {
			overlap = true
			continue
		}
		if !truncated {
			return fmt.Errorf("persisted session %s removed previously observed authority", label)
		}
	}
	if truncated && !overlap {
		return fmt.Errorf("persisted session %s advanced beyond its bounded snapshot", label)
	}
	return nil
}

func changedMessageTurn(previous, current model.ChannelMessage) (bool, error) {
	if previous.ID != current.ID || previous.ChannelID != current.ChannelID ||
		previous.Role != current.Role || previous.SpeakerName != current.SpeakerName ||
		previous.Content != current.Content || !previous.CreatedAt.Equal(current.CreatedAt) ||
		!reflect.DeepEqual(previous.Roleplay, current.Roleplay) {
		return false, fmt.Errorf("persisted session message %d changed immutable content", current.ID)
	}
	if previous.Turn == nil || current.Turn == nil {
		if previous.Turn != nil || current.Turn != nil {
			return false, fmt.Errorf("persisted session message %d changed job binding", current.ID)
		}
		return false, nil
	}
	if previous.Turn.JobID != current.Turn.JobID {
		return false, fmt.Errorf("persisted session message %d changed job identity", current.ID)
	}
	if current.Turn.UpdatedAt.Before(previous.Turn.UpdatedAt) {
		return false, fmt.Errorf("persisted session message %d moved job state backward in time", current.ID)
	}
	if terminalMessageTurn(previous) &&
		(previous.Turn.Status != current.Turn.Status || previous.Turn.Error != current.Turn.Error ||
			!previous.Turn.UpdatedAt.Equal(current.Turn.UpdatedAt)) {
		return false, fmt.Errorf("persisted session message %d changed terminal job state", current.ID)
	}
	return previous.Turn.Status != current.Turn.Status || previous.Turn.Error != current.Turn.Error, nil
}

func terminalMessageTurn(message model.ChannelMessage) bool {
	return message.Turn != nil && terminalJob(message.Turn.Status)
}

func enqueuedTurnsByJob(turns []queue.ChannelSessionTurn) map[int64]queue.ChannelSessionTurn {
	result := make(map[int64]queue.ChannelSessionTurn)
	for _, turn := range turns {
		if turn.Disposition == queue.ChannelSessionTurnEnqueued {
			result[turn.JobID] = turn
		}
	}
	return result
}

func (session *chatSession) suppressLocalEnqueuedMessage(
	message model.ChannelMessage,
	enqueued map[int64]queue.ChannelSessionTurn,
) bool {
	if !session.renderer.console.IsTerminal() || session.pendingTurn == nil || message.Turn == nil {
		return false
	}
	turn, exists := enqueued[message.Turn.JobID]
	return exists && turn.OperationID == session.pendingTurn.operationID &&
		turn.Text == message.Content
}

func (session *chatSession) suppressSessionTurn(
	turn queue.ChannelSessionTurn,
	messages []model.ChannelMessage,
) bool {
	if turn.Disposition == queue.ChannelSessionTurnEnqueued {
		for _, message := range messages {
			if message.Turn != nil && message.Turn.JobID == turn.JobID && message.Content == turn.Text {
				return true
			}
		}
	}
	return session.renderer.console.IsTerminal() && session.pendingTurn != nil &&
		session.pendingTurn.operationID == turn.OperationID
}

func (session *chatSession) suppressSessionControl(control queue.ChannelSessionControl) bool {
	return session.renderer.console.IsTerminal() && session.pendingControl != nil &&
		session.pendingControl.locallyEchoed && session.pendingControl.operationID == control.OperationID
}

func (session *chatSession) reconcileActiveJob(next *model.JobDetails, initial bool) error {
	previous := session.active
	if previous != nil && (next == nil || next.Job.ID != previous.Job.ID) {
		status, found := session.terminalSnapshotJobStatus(previous.Job.ID)
		if !found {
			return fmt.Errorf("active job %d disappeared without persisted terminal state", previous.Job.ID)
		}
		if !terminalJob(status) {
			return fmt.Errorf("active job %d changed identity while status is %q", previous.Job.ID, status)
		}
	}
	if next == nil {
		session.active = nil
		return nil
	}
	copy := *next
	copy.Steps = append([]model.Step(nil), next.Steps...)
	if previous == nil || previous.Job.ID != copy.Job.ID {
		session.active = &copy
		if initial {
			if err := session.renderer.system("reconnected to active job %d", copy.Job.ID); err != nil {
				return err
			}
		}
		return session.renderer.job(copy, nil)
	}
	session.active = &copy
	return session.renderer.job(copy, previous)
}

func (session *chatSession) terminalSnapshotJobStatus(jobID int64) (string, bool) {
	for _, message := range session.messages {
		if message.Turn != nil && message.Turn.JobID == jobID && terminalJob(message.Turn.Status) {
			return message.Turn.Status, true
		}
	}
	return "", false
}
