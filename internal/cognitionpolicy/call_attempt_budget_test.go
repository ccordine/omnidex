package cognitionpolicy

import "testing"

func TestCallAttemptRequiresBudgetBoundToFrozenBrain(t *testing.T) {
	projection := policyTestProjection(t, "budget-bound call")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	brain := policyTestAttestedBrain()
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*CallAttempt){
		"inflated input tokens": func(value *CallAttempt) {
			value.RuntimeBudget.MaxInputTokens++
		},
		"input bytes beyond brain": func(value *CallAttempt) {
			value.RuntimeBudget.MaxInputBytes = value.Brain.ContextCeilingBytes + 1
			value.RuntimeBudget.MaxInputTokens = value.RuntimeBudget.MaxInputBytes +
				value.Brain.Sampling.InputSpecialTokenReserve
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			forged := attempt
			mutate(&forged)
			forged.ID = callAttemptID(forged)
			if err := forged.Validate(); err == nil {
				t.Fatal("call attempt accepted a budget detached from its Brain")
			}
		})
	}
}
