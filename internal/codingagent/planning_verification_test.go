package codingagent

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestParserAddedVerificationCannotBecomeOptional(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewProposed)
	for _, item := range []model.LedgerMutationItem{
		{AcceptanceCriterion: &ledger.AcceptanceCriteria[0]}, {Check: &ledger.Checks[0]},
	} {
		if item.AcceptanceCriterion != nil {
			item.AcceptanceCriterion.Required = false
		}
		if item.Check != nil {
			item.Check.Required = false
		}
		content, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{"kind": "add", "item": item, "rationale": "stated verification"}}})
		operations, err := decodePlanningOperations(content, ledger)
		if err != nil || len(operations) != 1 {
			t.Fatalf("parse verification: %v", err)
		}
		parsed := operations[0].Item
		if parsed.AcceptanceCriterion != nil && !parsed.AcceptanceCriterion.Required || parsed.Check != nil && !parsed.Check.Required {
			t.Fatal("AI made finite planned verification optional")
		}
	}
}

func TestParserCannotAmendCodeOwnedVerificationPolicy(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	for _, value := range []string{"false", "null"} {
		content, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{
			"kind": "amend", "target_id": ledger.Checks[0].ID, "item": map[string]any{"check": map[string]any{"required": json.RawMessage(value)}}, "rationale": "change",
		}}})
		if _, err := decodePlanningOperations(content, ledger); err == nil {
			t.Fatal("parser rewrote the code-owned verification policy")
		}
	}
}
