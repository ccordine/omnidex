package worker

import "fmt"

type objectiveDatabaseRawLeafCallLedger struct {
	receipts []objectiveDatabaseRawLeafReceipt
}

type objectiveDatabaseRawLeafReceipt struct {
	label   string
	receipt objectiveStationReceipt
}

func (ledger *objectiveDatabaseRawLeafCallLedger) record(
	label string,
	receipt objectiveStationReceipt,
) error {
	if ledger == nil {
		return fmt.Errorf("database %s leaf has no receipt ledger", label)
	}
	if err := validateObjectiveStationReceipt("database "+label+" leaf", receipt); err != nil {
		return err
	}
	ledger.receipts = append(ledger.receipts, objectiveDatabaseRawLeafReceipt{
		label: label, receipt: receipt,
	})
	return nil
}

func (ledger objectiveDatabaseRawLeafCallLedger) partial() objectiveStationReceipt {
	receipt := objectiveStationReceipt{Reused: len(ledger.receipts) > 0}
	for _, recorded := range ledger.receipts {
		receipt.Calls += recorded.receipt.Calls
		receipt.Reused = receipt.Reused && recorded.receipt.Reused
	}
	return receipt
}

func (ledger objectiveDatabaseRawLeafCallLedger) complete(
	reportedCalls int,
) (objectiveStationReceipt, error) {
	receipt := ledger.partial()
	if len(ledger.receipts) < 1 {
		return objectiveStationReceipt{}, fmt.Errorf(
			"database semantic station completed without one exact leaf receipt",
		)
	}
	for _, recorded := range ledger.receipts {
		if err := validateObjectiveStationReceipt(
			"database "+recorded.label+" leaf", recorded.receipt,
		); err != nil {
			return objectiveStationReceipt{}, err
		}
	}
	if reportedCalls != receipt.Calls {
		return receipt, fmt.Errorf(
			"database semantic station reported %d calls but its exact leaf receipts prove %d",
			reportedCalls, receipt.Calls,
		)
	}
	return receipt, nil
}

type objectiveDatabaseBoundedCallLedger struct {
	receipts []objectiveDatabaseBoundedCallReceipt
}

type objectiveDatabaseBoundedCallReceipt struct {
	scope        string
	label        string
	receipt      objectiveStationReceipt
	maximumCalls int
}

func (ledger *objectiveDatabaseBoundedCallLedger) record(
	scope string,
	label string,
	receipt objectiveStationReceipt,
	maximumCalls int,
) error {
	if ledger == nil {
		return fmt.Errorf("%s %s has no receipt ledger", scope, label)
	}
	if err := validateObjectiveBoundedStationReceipt(
		scope+" "+label, receipt, maximumCalls,
	); err != nil {
		return err
	}
	ledger.receipts = append(ledger.receipts, objectiveDatabaseBoundedCallReceipt{
		scope: scope, label: label, receipt: receipt, maximumCalls: maximumCalls,
	})
	return nil
}

func (ledger objectiveDatabaseBoundedCallLedger) partial() objectiveStationReceipt {
	receipt := objectiveStationReceipt{Reused: len(ledger.receipts) > 0}
	for _, recorded := range ledger.receipts {
		receipt.Calls += recorded.receipt.Calls
		receipt.Reused = receipt.Reused && recorded.receipt.Reused
	}
	return receipt
}

func (ledger objectiveDatabaseBoundedCallLedger) totalForSuccess(
	scope string,
) (
	objectiveStationReceipt,
	error,
) {
	if len(ledger.receipts) < 1 {
		return objectiveStationReceipt{}, fmt.Errorf(
			"%s has no exact semantic receipt", scope,
		)
	}
	for _, recorded := range ledger.receipts {
		if recorded.scope != scope {
			return objectiveStationReceipt{}, fmt.Errorf(
				"%s contains a receipt for mismatched scope %q",
				scope, recorded.scope,
			)
		}
		if err := validateObjectiveBoundedStationReceipt(
			recorded.scope+" "+recorded.label,
			recorded.receipt,
			recorded.maximumCalls,
		); err != nil {
			return objectiveStationReceipt{}, err
		}
	}
	receipt := ledger.partial()
	if receipt.Reused {
		if receipt.Calls != 0 {
			return objectiveStationReceipt{}, fmt.Errorf(
				"%s reuse reported %d provider calls", scope, receipt.Calls,
			)
		}
		return receipt, nil
	}
	if receipt.Calls < 1 {
		return objectiveStationReceipt{}, fmt.Errorf(
			"%s has no fresh calls or durable reuse proof", scope,
		)
	}
	return receipt, nil
}

func (ledger objectiveDatabaseBoundedCallLedger) count() int {
	return len(ledger.receipts)
}

func (ledger objectiveDatabaseBoundedCallLedger) freshCalls() int {
	return ledger.partial().Calls
}

type objectiveDatabaseAcquisitionCallLedger struct {
	objectiveDatabaseBoundedCallLedger
}

func (ledger *objectiveDatabaseAcquisitionCallLedger) record(
	label string,
	receipt objectiveStationReceipt,
	maximumCalls int,
) error {
	if ledger == nil {
		return fmt.Errorf("database acquisition %s has no receipt ledger", label)
	}
	return ledger.objectiveDatabaseBoundedCallLedger.record(
		"database acquisition", label, receipt, maximumCalls,
	)
}

func (ledger objectiveDatabaseAcquisitionCallLedger) totalForSuccess() (
	objectiveStationReceipt,
	error,
) {
	return ledger.objectiveDatabaseBoundedCallLedger.totalForSuccess(
		"database acquisition",
	)
}
