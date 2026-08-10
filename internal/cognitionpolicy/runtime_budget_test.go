package cognitionpolicy

import (
	"errors"
	"testing"
)

func TestRuntimeBudgetMustFitFrozenBrainBeforeEpisodeStart(t *testing.T) {
	projection := policyTestProjection(t, "authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	brain, budget := policyTestBrain(), snapshot.Budget()
	if err := ValidateRuntimeBudget(brain, budget); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*BrainRef){
		func(value *BrainRef) {
			value.ContextCeilingBytes = budget.MaxInputBytes - 1
			refreshPolicyTestSampling(value)
		},
		func(value *BrainRef) {
			value.Sampling.MaxOutputTokens = budget.MaxOutputTokens - 1
			refreshPolicyTestSampling(value)
		},
	} {
		candidate := brain
		mutate(&candidate)
		if err := ValidateRuntimeBudget(candidate, budget); !errors.Is(err, ErrInvalidBrain) {
			t.Fatalf("ValidateRuntimeBudget() error=%v, want ErrInvalidBrain", err)
		}
	}
}

func TestRuntimeBudgetRequiresExactRawInputTokenAuthority(t *testing.T) {
	projection := policyTestProjection(t, "exact raw budget")
	snapshot, _ := policyTestSnapshot(t, projection)
	brain, budget := policyTestBrain(), snapshot.Budget()
	budget.MaxInputTokens++
	if err := ValidateRuntimeBudget(brain, budget); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("ValidateRuntimeBudget() error=%v, want ErrInvalidBrain", err)
	}
}

func TestBrainRejectsUnregisteredRawProviderVersion(t *testing.T) {
	brain := policyTestBrain()
	brain.BackendVersion = "0.25.0"
	if err := brain.Validate(); !errors.Is(err, ErrInvalidBrain) {
		t.Fatalf("BrainRef.Validate() error=%v, want ErrInvalidBrain", err)
	}
}
