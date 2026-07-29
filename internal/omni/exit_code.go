package omni

import (
	"errors"
	"fmt"
)

type ExitCodeError struct {
	Code int
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func IsExitCodeError(err error) (int, bool) {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}
