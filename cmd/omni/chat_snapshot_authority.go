package main

import (
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// retainSnapshotAuthority keeps only the current bounded server projection.
// Continuity is checked before replacement, so a long-lived terminal never
// becomes an unbounded duplicate transcript store.
func (session *chatSession) retainSnapshotAuthority(snapshot client.ChatSessionSnapshot) {
	messages := make(map[int64]model.ChannelMessage, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		messages[message.ID] = message
	}
	turns := make(map[queue.LifecycleOperationID]queue.ChannelSessionTurn, len(snapshot.Turns))
	for _, turn := range snapshot.Turns {
		turns[turn.OperationID] = turn
	}
	controls := make(map[queue.LifecycleOperationID]queue.ChannelSessionControl, len(snapshot.Controls))
	for _, control := range snapshot.Controls {
		controls[control.OperationID] = control
	}
	session.messages = messages
	session.turns = turns
	session.controls = controls
}
