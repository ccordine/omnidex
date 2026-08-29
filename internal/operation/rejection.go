package operation

import "errors"

type rejectedError struct {
	err error
}

func (e *rejectedError) Error() string {
	if e == nil || e.err == nil {
		return "operation rejected"
	}
	return e.err.Error()
}

func (e *rejectedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// Reject marks a deterministic operation request as invalid without hiding its cause.
func Reject(err error) error {
	if err == nil {
		return nil
	}
	if IsRejected(err) {
		return err
	}
	return &rejectedError{err: err}
}

// IsRejected reports whether deterministic validation rejected an operation.
func IsRejected(err error) bool {
	var target *rejectedError
	return errors.As(err, &target)
}
