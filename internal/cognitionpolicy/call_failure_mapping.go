package cognitionpolicy

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// CallResultError returns the one registered sentinel identity for a durable
// nonaccepted policy result. It is shared by local replay and queue recovery.
func CallResultError(result CallResult) error {
	var sentinel error
	switch result.Status {
	case CallResultRejected:
		switch result.FailureCode {
		case CallFailureResponseLimit:
			sentinel = ErrResponseLimit
		case CallFailureInvalidDecision:
			sentinel = ErrInvalidDecision
		case CallFailureAuthorityDenied:
			sentinel = errors.Join(ErrInvalidDecision, cognition.ErrAuthorityDenied)
		case CallFailureProviderUsage:
			sentinel = ErrProviderUsage
		case CallFailureProviderUsageLimit:
			sentinel = ErrProviderUsageLimit
		}
	case CallResultFailed:
		switch result.FailureCode {
		case CallFailureGeneration:
			sentinel = ErrGeneration
		case CallFailureProviderIdentity:
			sentinel = ErrProviderIdentity
		case CallFailureProviderRequest:
			sentinel = errors.Join(ErrGeneration, ErrInvalidEvidence)
		case CallFailurePolicyAuthority, CallFailureProviderEvidence:
			sentinel = ErrInvalidEvidence
		}
	}
	if sentinel == nil {
		return fmt.Errorf("%w: policy result failure code %q is not registered for status %q",
			ErrInvalidEvidence, result.FailureCode, result.Status)
	}
	return fmt.Errorf("%w: %s", sentinel, result.FailureMessage)
}
