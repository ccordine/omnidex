package main

import "fmt"

func (session *chatSession) reconcilePersistedPendingOperations() error {
	if session.pendingTurn != nil && session.pendingControl != nil {
		return fmt.Errorf("interactive client has two unresolved operation identities")
	}
	if pending := session.pendingTurn; pending != nil {
		turn, persisted := session.turns[pending.operationID]
		if !persisted {
			return nil
		}
		if turn.Text != pending.exactText {
			return fmt.Errorf("persisted session turn differs from pending operation authority")
		}
		session.pendingTurn = nil
		return nil
	}
	if pending := session.pendingControl; pending != nil {
		control, persisted := session.controls[pending.operationID]
		if !persisted {
			return nil
		}
		if control.JobID != pending.jobID || control.Kind != sessionControlKind(pending.action) ||
			control.Text != pending.exactText {
			return fmt.Errorf("persisted /%s operation differs from pending authority", pending.action)
		}
		session.pendingControl = nil
	}
	return nil
}
