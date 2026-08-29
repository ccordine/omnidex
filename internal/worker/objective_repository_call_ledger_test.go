package worker

import (
	"strings"
	"testing"
)

func TestObjectiveRepositoryCallLedgerRejectsInventedRoundsAndAttempts(t *testing.T) {
	t.Parallel()
	for name, run := range map[string]func() error{
		"zero relevance attempts": func() error {
			ledger := objectiveRepositoryAcquisitionCallLedger{}
			return ledger.recordRelevance(objectiveStationReceipt{})
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
	wantMax := maxObjectiveRepositoryRelevanceModelCalls
	if maxObjectiveRepositoryEvidenceModelCalls != wantMax {
		t.Fatalf("derived max=%d want %d", maxObjectiveRepositoryEvidenceModelCalls, wantMax)
	}
	if total != maxObjectiveRepositoryEvidenceModelCalls {
		t.Fatalf("total=%d max=%d", total, maxObjectiveRepositoryEvidenceModelCalls)
	}
}
