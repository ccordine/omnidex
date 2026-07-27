package tools

import "errors"

type callRejectedError struct {
	err error
}

func (e *callRejectedError) Error() string {
	if e == nil || e.err == nil {
		return "tool call rejected"
	}
	return e.err.Error()
}

func (e *callRejectedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func RejectCall(err error) error {
	if err == nil {
		return nil
	}
	if IsCallRejected(err) {
		return err
	}
	return &callRejectedError{err: err}
}

func IsCallRejected(err error) bool {
	var target *callRejectedError
	return errors.As(err, &target)
}
