package cognitionstate

import "errors"

var (
	ErrImmutableEvidence      = errors.New("cognition state evidence is not immutable tool evidence")
	ErrInvalidMapping         = errors.New("invalid cognition state mapping")
	ErrInvalidPolicy          = errors.New("invalid cognition fact acceptance policy")
	ErrInvalidReconciliation  = errors.New("invalid cognition state reconciliation")
	ErrFactPolicyRejected     = errors.New("cognition fact acceptance policy rejected evidence")
	ErrMissingMaterial        = errors.New("cognition state evidence material is missing")
	ErrPolicyNotRegistered    = errors.New("cognition fact acceptance policy is not registered")
	ErrReconciliationCapacity = errors.New("cognition state reconciliation exceeds a code-owned capacity")
)
