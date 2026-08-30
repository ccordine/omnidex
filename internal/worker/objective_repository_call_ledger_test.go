package worker

import (
	"strings"
	"testing"
)

func TestObjectiveRepositoryCallLedgerRejectsInventedRoundsAndAttempts(t *testing.T) {
	t.Parallel()
	for name, run := range map[string]func() error{
		"zero calls without reuse proof": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordRelevance(objectiveStationReceipt{})
		},
		"calls with reuse proof": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordRelevance(objectiveStationReceipt{Calls: 1, Reused: true})
		},
		"too many relevance attempts": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordRelevance(objectiveStationReceipt{Calls: maxObjectiveRepositoryRelevanceModelCalls + 1})
		},
		"second relevance round": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			if err := ledger.recordRelevance(objectiveStationReceipt{Calls: 1}); err != nil {
				return err
			}
			return ledger.recordRelevance(objectiveStationReceipt{Calls: 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil {
				t.Fatalf("invalid ledger state %q was accepted", name)
			}
		})
	}

	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if _, err := ledger.totalForSuccess(); err == nil || !strings.Contains(err.Error(), "relevance-call") {
		t.Fatalf("missing relevance error=%v", err)
	}
}

func TestObjectiveRepositoryCallLedgerAcceptsFullyRestoredRound(t *testing.T) {
	t.Parallel()
	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if err := ledger.recordRelevance(objectiveStationReceipt{Reused: true}); err != nil {
		t.Fatal(err)
	}
	total, err := ledger.totalForSuccess()
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || !ledger.relevanceRecorded || !ledger.relevanceReceipt.Reused {
		t.Fatalf("total=%d ledger=%+v", total, ledger)
	}
}

func TestObjectiveRepositoryCallLedgerRejectsMalformedRecordedReceipt(t *testing.T) {
	t.Parallel()
	for name, receipt := range map[string]objectiveStationReceipt{
		"zero calls without reuse proof": {},
		"calls with reuse proof":         {Calls: 1, Reused: true},
	} {
		name, receipt := name, receipt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ledger := objectiveRepositoryAcquisitionCallLedger{
				relevanceReceipt: receipt, relevanceRecorded: true,
			}
			if _, err := ledger.totalForSuccess(); err == nil {
				t.Fatalf("malformed recorded receipt %+v was accepted", receipt)
			}
		})
	}
}

func TestObjectiveRepositoryCallLedgerDerivesExactMaximum(t *testing.T) {
	t.Parallel()
	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if err := ledger.recordRelevance(objectiveStationReceipt{Calls: maxObjectiveRepositoryRelevanceModelCalls}); err != nil {
		t.Fatal(err)
	}
	total, err := ledger.totalForSuccess()
	if err != nil {
		t.Fatal(err)
	}
	wantMax := maxObjectiveRepositoryEvidenceCapsules * exactSemanticLeafCalls
	if maxObjectiveRepositoryEvidenceModelCalls != wantMax {
		t.Fatalf("derived max=%d want %d", maxObjectiveRepositoryEvidenceModelCalls, wantMax)
	}
	if total != maxObjectiveRepositoryEvidenceModelCalls {
		t.Fatalf("total=%d max=%d", total, maxObjectiveRepositoryEvidenceModelCalls)
	}
}
