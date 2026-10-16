package codingagent

import "github.com/gryph/omnidex/internal/model"

// Criteria and checks in the finite proposed work set are obligations, not
// model-selected optional work. Code owns that invariant before owner review.
func requirePlanningVerification(ledger *EngineeringLedger) {
	for index := range ledger.AcceptanceCriteria {
		ledger.AcceptanceCriteria[index].Required = true
	}
	for index := range ledger.Checks {
		ledger.Checks[index].Required = true
	}
}

func requirePlanningItemVerification(item *model.LedgerMutationItem) {
	if item.AcceptanceCriterion != nil {
		item.AcceptanceCriterion.Required = true
	}
	if item.Check != nil {
		item.Check.Required = true
	}
}
