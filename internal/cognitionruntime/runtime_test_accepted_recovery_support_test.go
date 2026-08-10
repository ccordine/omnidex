package cognitionruntime

import "context"

func (h *runtimeHarness) RecoverAccepted(
	_ context.Context,
	_ Binding,
) (*AcceptedDecisionRecovery, error) {
	h.order = append(h.order, "accepted-recovery")
	if h.acceptedRecovery == nil {
		return nil, nil
	}
	copy := *h.acceptedRecovery
	copy.Prepared = copy.Prepared.clone()
	copy.Decision = copy.Decision.Clone()
	copy.ActionSchema = copy.ActionSchema.Clone()
	if copy.ExistingReconciliation != nil {
		replay := ReconciliationReplay{
			Command: copy.ExistingReconciliation.Command.Clone(),
			Receipt: copy.ExistingReconciliation.Receipt.Clone(),
		}
		copy.ExistingReconciliation = &replay
	}
	return &copy, nil
}
