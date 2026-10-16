package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPlanningAuditUncoveredDirectActionReturnsVisibleJudgment(t *testing.T) {
	err := uncoveredPlanningCoverageError(codingagent.PlanningCoverageJudgment{
		Explanation: "the removal leaves an owner statement uncovered",
		Suggestions: []string{"restore the item", "record an unresolved question"},
	})
	if !errors.Is(err, queue.ErrCodingLedgerState) ||
		!strings.Contains(err.Error(), "the removal leaves an owner statement uncovered") ||
		!strings.Contains(err.Error(), "restore the item; record an unresolved question") {
		t.Fatalf("uncovered direct action error = %v", err)
	}
}

func TestPlanningAuditPersistedReceiptResumesExactRevision(t *testing.T) {
	receipt := persistedPlanningAuditReceipt(queue.CodingLedgerPlanningAudit{
		LedgerID: 7, AuditID: "planning-audit:freeze-1", Revision: 4,
		Ready: true, Explanation: "the persisted proposal is ready",
	})
	if receipt.AuditID != "planning-audit:freeze-1" || receipt.Revision != 4 || !receipt.Ready ||
		receipt.Judgment.Question != codingagent.AuditPlanningQuestion ||
		receipt.Judgment.BaseRevision != 4 || !receipt.Judgment.Covered ||
		receipt.Judgment.Explanation != "the persisted proposal is ready" {
		t.Fatalf("persisted audit receipt = %#v", receipt)
	}
}
