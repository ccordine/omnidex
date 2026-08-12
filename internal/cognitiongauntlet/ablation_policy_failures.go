package cognitiongauntlet

import (
	"errors"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func registeredAblationPolicyFailure(err error) bool {
	for _, loud := range []error{
		cognitionpolicy.ErrProviderIdentity, cognitionpolicy.ErrCallJournal,
		cognitionpolicy.ErrCallIndeterminate, cognitionpolicy.ErrEnvelopeLimit,
		cognitionpolicy.ErrInputLimit, cognitionpolicy.ErrInvalidConfig,
		cognitionpolicy.ErrInvalidEvidence, cognitionpolicy.ErrInvalidProjection,
		cognitionpolicy.ErrProjectionMismatch, cognitionpolicy.ErrInvalidBrain,
	} {
		if errors.Is(err, loud) {
			return false
		}
	}
	return errors.Is(err, cognitionpolicy.ErrGeneration) ||
		errors.Is(err, cognitionpolicy.ErrInvalidDecision) ||
		errors.Is(err, cognitionpolicy.ErrResponseLimit) ||
		errors.Is(err, cognitionpolicy.ErrProviderUsageLimit) ||
		errors.Is(err, cognitionpolicy.ErrCallRejected)
}
