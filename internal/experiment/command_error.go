package experiment

import "fmt"

// ExitError is an observed process result, separate from transport or workspace
// observation failures. Callers never classify outcomes from error wording.
type ExitError struct{ Code int }

func (err *ExitError) Error() string {
	return fmt.Sprintf("Docker experiment command exited %d", err.Code)
}
