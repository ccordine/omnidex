package cognitionruntime

import "errors"

var (
	ErrInvalidConfiguration = errors.New("invalid cognition runtime configuration")
	ErrInvalidBinding       = errors.New("invalid cognition runtime binding")
	ErrInvalidPreparedState = errors.New("invalid prepared cognition state")
	ErrInvalidJournalState  = errors.New("invalid cognition journal state")
	ErrInvalidProgress      = errors.New("invalid cognition episode progress")
	ErrInvalidSeal          = errors.New("invalid cognition terminal seal")
	ErrRunCycleLimit        = errors.New("cognition runtime cycle limit reached")
	ErrEnvironment          = errors.New("cognition environment execution failed")
)
