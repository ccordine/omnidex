package cognitionpolicy

import "errors"

var (
	ErrEnvelopeLimit      = errors.New("cognition policy envelope limit exceeded")
	ErrCallJournal        = errors.New("cognition policy call journal failed")
	ErrGeneration         = errors.New("cognition policy generation failed")
	ErrCallIndeterminate  = errors.New("cognition policy call outcome is indeterminate")
	ErrCallRejected       = errors.New("cognition policy call was rejected")
	ErrInputLimit         = errors.New("cognition policy input limit exceeded")
	ErrInvalidConfig      = errors.New("invalid cognition policy configuration")
	ErrInvalidDecision    = errors.New("invalid cognition policy decision")
	ErrInvalidEvidence    = errors.New("invalid cognition policy evidence")
	ErrInvalidBrain       = errors.New("invalid cognition policy brain reference")
	ErrInvalidProjection  = errors.New("invalid cognition policy projection")
	ErrProjectionMismatch = errors.New("cognition policy projection mismatch")
	ErrProviderIdentity   = errors.New("cognition policy provider identity changed")
	ErrResponseLimit      = errors.New("cognition policy response limit exceeded")
)
