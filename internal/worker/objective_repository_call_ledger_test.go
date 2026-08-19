package worker

import (
	"strings"
	"testing"
)

func TestObjectiveRepositoryCallLedgerRejectsInventedRoundsAndAttempts(t *testing.T) {
	t.Parallel()
	for name, run := range map[string]func() error{
		"zero search attempts": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordSearchTerm(objectiveStationReceipt{})
		},
		"too many relevance attempts": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordRelevance(objectiveStationReceipt{Calls: maxTypedWorkerAttempts + 1})
		},
		"second search round": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			if err := ledger.recordSearchTerm(objectiveStationReceipt{Calls: 1}); err != nil {
				return err
			}
			return ledger.recordSearchTerm(objectiveStationReceipt{Calls: 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil {
				t.Fatalf("invalid ledger state %q was accepted", name)
			}
		})
	}

	ledger := objectiveRepositoryAcquisitionCallLedger{relevanceCalls: []int{1, 1}}
	if _, err := ledger.totalForSuccess(); err == nil || !strings.Contains(err.Error(), "search-term") {
		t.Fatalf("repeated relevance without expansion error=%v", err)
	}
}

func TestObjectiveRepositoryCallLedgerDerivesExactMaximum(t *testing.T) {
	t.Parallel()
	ledger := objectiveRepositoryAcquisitionCallLedger{}
	if err := ledger.recordRelevance(objectiveStationReceipt{Calls: maxTypedWorkerAttempts}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordSearchTerm(objectiveStationReceipt{Calls: maxTypedWorkerAttempts}); err != nil {
		t.Fatal(err)
	}
	for round := 1; round < maxObjectiveRepositoryRelevanceRounds; round++ {
		if err := ledger.recordRelevance(objectiveStationReceipt{Calls: maxTypedWorkerAttempts}); err != nil {
			t.Fatal(err)
		}
	}
	total, err := ledger.totalForSuccess()
	if err != nil {
		t.Fatal(err)
	}
	const wantMax = 15
	if maxObjectiveRepositoryEvidenceModelCalls != wantMax {
		t.Fatalf("derived max=%d want %d", maxObjectiveRepositoryEvidenceModelCalls, wantMax)
	}
	if total != maxObjectiveRepositoryEvidenceModelCalls {
		t.Fatalf("total=%d max=%d", total, maxObjectiveRepositoryEvidenceModelCalls)
	}
}
