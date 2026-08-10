package cognitionruntime

import "context"

func (h *runtimeHarness) ReplayTerminalPolicyOutcome(_ context.Context, _ Binding) (bool, error) {
	h.order = append(h.order, "terminal-policy")
	return h.terminalPolicyRecovered, h.terminalPolicyError
}

func (h *runtimeHarness) AbandonIndeterminate(_ context.Context, _ Binding) (*PolicyCallAbandonment, error) {
	h.order = append(h.order, "abandon-policy")
	if h.abandonment == nil {
		return nil, nil
	}
	copy := *h.abandonment
	return &copy, nil
}
