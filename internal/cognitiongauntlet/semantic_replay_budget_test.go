package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestSemanticRuntimeBudgetIsDerivedFromFrozenCallCount(t *testing.T) {
	initial := cognition.RuntimeBudget{
		RemainingPolicyCalls: 4,
		MaxInputBytes:        4096, MaxInputTokens: 4098,
		MaxOutputBytes: 2048, MaxOutputTokens: 1024,
		MaxEvidenceRefs: 8, MaxActionArguments: 4,
		MaxLedgerProposals: 3, MaxAttentionRequests: 2,
		MaxExpectedEffectBytes: 256,
	}
	want := initial
	want.RemainingPolicyCalls = 2
	if err := validateSemanticRuntimeBudget(initial, want, 3, 2); err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name          string
		budget        cognition.RuntimeBudget
		ordinal       int64
		priorAttempts int
	}{
		{"inflated_remaining", withRemaining(want, 3), 3, 2},
		{"over_decremented", withRemaining(want, 1), 3, 2},
		{"ordinal_hole", want, 4, 2},
		{"prior_call_hole", want, 3, 1},
		{"changed_maximum", withInputBytes(want, want.MaxInputBytes-1), 3, 2},
		{"exhausted_overflow", withRemaining(want, 0), 6, 5},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if validateSemanticRuntimeBudget(
				initial, mutation.budget, mutation.ordinal, mutation.priorAttempts,
			) == nil {
				t.Fatal("semantic replay accepted a forged runtime budget")
			}
		})
	}

	// Replacement/progress preparations may share one next ordinal before the
	// one durable policy call is inserted. Their exact budget remains equal.
	if err := validateSemanticRuntimeBudget(initial, want, 3, 2); err != nil {
		t.Fatal(err)
	}
}

func withRemaining(value cognition.RuntimeBudget, remaining uint32) cognition.RuntimeBudget {
	value.RemainingPolicyCalls = remaining
	return value
}

func withInputBytes(value cognition.RuntimeBudget, bytes int) cognition.RuntimeBudget {
	value.MaxInputBytes = bytes
	return value
}
