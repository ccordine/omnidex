package cognitiongauntlet

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestRuntimeCancellationClassifiesOnlyRegisteredModelAndBudgetFailures(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		source error
		code   cognitionruntime.CancellationCode
	}{
		"policy generation": {
			source: errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrGeneration),
			code:   cognitionruntime.CancellationPolicyFailure,
		},
		"invalid decision": {
			source: errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrInvalidDecision),
			code:   cognitionruntime.CancellationPolicyFailure,
		},
		"response limit": {
			source: errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrResponseLimit),
			code:   cognitionruntime.CancellationPolicyFailure,
		},
		"call budget": {
			source: cognition.ErrCoordinatorBudgetExhausted,
			code:   cognitionruntime.CancellationRunBudgetExhausted,
		},
		"cycle budget": {
			source: cognitionruntime.ErrRunCycleLimit,
			code:   cognitionruntime.CancellationRunBudgetExhausted,
		},
	} {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := classifyRuntimeCancellation(test.source)
			if !ok || got.code != test.code || got.message == "" {
				t.Fatalf("classification=%+v registered=%t", got, ok)
			}
		})
	}
	for _, source := range []error{
		nil,
		errors.New("PostgreSQL write failed"),
		cognition.ErrPolicyFailed,
		cognition.ErrInvalidDecision,
		cognition.ErrInvalidAction,
		cognition.ErrInvalidEvidence,
		cognition.ErrAuthorityDenied,
		cognitionruntime.ErrInvalidPreparedState,
		cognitionruntime.ErrEnvironment,
		errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrProviderIdentity),
		errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrCallJournal),
		errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrCallIndeterminate),
		errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrEnvelopeLimit),
		errors.Join(cognition.ErrPolicyFailed, cognitionpolicy.ErrInvalidProjection),
		errors.Join(cognitionpolicy.ErrGeneration, cognitionpolicy.ErrProviderIdentity),
	} {
		if _, ok := classifyRuntimeCancellation(source); ok {
			t.Fatalf("integrity failure %v was converted to an ordinary cancellation", source)
		}
	}
}
