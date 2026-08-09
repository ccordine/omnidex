package taskstate

import "fmt"

func validateNodeTransition(node Node, command TransitionNodeCommand) error {
	if terminalNode(node.Status) {
		return fmt.Errorf("%w: terminal node %q cannot reopen", ErrInvalidTransition, node.ID)
	}
	if command.To == node.Status {
		return ErrNoStateChange
	}
	if command.To != NodeDone && (command.CompletedStepID != nil || len(command.VerificationRefs) != 0) {
		return fmt.Errorf("%w: completion fields are forbidden for status %q", ErrInvalidCommand, command.To)
	}
	switch {
	case node.Status == NodeReady && command.To == NodeActive:
		if executableNode(node.Kind) && node.AssignedStepID == nil {
			return fmt.Errorf("%w: executable node must have an assigned step before activation", ErrInvalidState)
		}
		if command.Reason != "" {
			return fmt.Errorf("%w: ready-to-active transition does not accept a reason", ErrInvalidCommand)
		}
		return nil
	case node.Status == NodeActive && command.To == NodeDone:
		if command.CompletedStepID == nil || *command.CompletedStepID <= 0 {
			return fmt.Errorf("%w: completion requires a positive completed step ID", ErrInvalidCommand)
		}
		if executableNode(node.Kind) {
			if node.AssignedStepID == nil || *node.AssignedStepID != *command.CompletedStepID {
				return fmt.Errorf("%w: completed step must match the assigned step", ErrInvalidState)
			}
		} else if node.AssignedStepID != nil {
			return fmt.Errorf("%w: aggregate nodes cannot carry assigned steps", ErrInvalidState)
		}
		if err := validateRefs(command.VerificationRefs); err != nil {
			return err
		}
		if !hasEvidenceRef(command.VerificationRefs) {
			return fmt.Errorf("%w: completion requires verification evidence", ErrEvidenceRequired)
		}
		if command.Reason != "" {
			return fmt.Errorf("%w: successful completion does not accept a failure reason", ErrInvalidCommand)
		}
		return nil
	case node.Status == NodeActive && (command.To == NodeBlocked || command.To == NodeFailed):
		return requireExactText(command.Reason, "transition reason")
	case node.Status == NodeBlocked && command.To == NodeReady:
		return requireExactText(command.Reason, "block resolution")
	case command.To == NodeCanceled:
		return requireExactText(command.Reason, "cancellation reason")
	default:
		return fmt.Errorf("%w: %q to %q", ErrInvalidTransition, node.Status, command.To)
	}
}

func (command CloseLedgerCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "close task ledgers"); err != nil {
		return Event{}, err
	}
	if !terminalLedger(command.Status) {
		return Event{}, fmt.Errorf("%w: close target %q is not terminal", ErrInvalidCommand, command.Status)
	}
	if err := requireExactText(command.Reason, "ledger close reason"); err != nil {
		return Event{}, err
	}
	if err := validateOptionalStep(command.StepID, "ledger close step ID"); err != nil {
		return Event{}, err
	}
	if command.Status == LedgerClosed {
		for _, node := range ledger.nodes {
			if node.Status != NodeDone {
				return Event{}, fmt.Errorf("%w: successful close requires every node to be done; node %q is %q", ErrInvalidState, node.ID, node.Status)
			}
		}
	}
	return Event{Kind: EventLedgerClosed, LedgerStatus: command.Status,
		StepID: cloneInt64(command.StepID), Reason: command.Reason}, nil
}
