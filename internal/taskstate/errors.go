package taskstate

import (
	"errors"
	"fmt"
)

var (
	ErrAuthorityDenied   = errors.New("task state authority denied")
	ErrCommandIDConflict = errors.New("task state command identity conflict")
	ErrEvidenceRequired  = errors.New("task state evidence required")
	ErrInvalidCommand    = errors.New("invalid task state command")
	ErrInvalidState      = errors.New("invalid task state")
	ErrInvalidTransition = errors.New("invalid task state transition")
	ErrNoStateChange     = errors.New("task state command made no change")
	ErrNotFound          = errors.New("task state record not found")
	ErrVersionConflict   = errors.New("task state version conflict")
)

type VersionConflictError struct {
	Expected uint64
	Actual   uint64
}

func (err VersionConflictError) Error() string {
	return fmt.Sprintf("%v: expected=%d actual=%d", ErrVersionConflict, err.Expected, err.Actual)
}

func (err VersionConflictError) Is(target error) bool {
	return target == ErrVersionConflict
}
