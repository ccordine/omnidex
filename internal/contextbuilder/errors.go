package contextbuilder

import "errors"

var (
	ErrInvalidSpec       = errors.New("invalid context specification")
	ErrInvalidInput      = errors.New("invalid context build input")
	ErrMaterialMismatch  = errors.New("context material does not match the working set")
	ErrStaleReference    = errors.New("stale context reference")
	ErrRequiredSelector  = errors.New("required context selector is unresolved")
	ErrBudgetExceeded    = errors.New("required context exceeds its hard budget")
	ErrInvalidProjection = errors.New("invalid context projection")
)
