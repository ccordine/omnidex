package cognitiontransport

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

var (
	ErrAuthentication = errors.New("cognition environment authentication failed")
	ErrInvalidWire    = errors.New("invalid cognition environment wire message")
	ErrRemote         = errors.New("cognition environment remote failure")
)

type RemoteError struct {
	Code    string
	Message string
}

func (failure RemoteError) Error() string {
	return fmt.Sprintf("%v: code=%s: %s", ErrRemote, failure.Code, failure.Message)
}

func (failure RemoteError) Unwrap() []error {
	errors := []error{ErrRemote}
	switch failure.Code {
	case "stale_authority":
		errors = append(errors, cognition.ErrAuthorityDenied)
	case "stale_revision":
		errors = append(errors, cognition.ErrInvalidRevision)
	case "invalid_evidence":
		errors = append(errors, cognition.ErrInvalidEvidence)
	case "unsupported_completion":
		errors = append(errors, cognition.ErrInvalidCompletionCheck)
	}
	return errors
}
