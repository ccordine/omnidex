package main

import "fmt"

func (session *chatSession) reconcilePersistedPendingOperations() error {
	if pendingOperationCount(session) > 1 {
		return fmt.Errorf("interactive client has multiple unresolved operation identities")
	}
	if pending := session.pendingTurn; pending != nil {
		turn, persisted := session.turns[pending.operationID]
		if !persisted {
			return nil
		}
		if err := validatePendingSessionTurn(turn, pending); err != nil {
			return err
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
