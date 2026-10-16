package codingagent

// ParserFailure retains a pure-inference failure for the existing queue to
// resume. It never grants permission to replay native execution or review.
type ParserFailure struct{ Cause error }

func (failure *ParserFailure) Error() string   { return failure.Cause.Error() }
func (failure *ParserFailure) Unwrap() error   { return failure.Cause }
func (failure *ParserFailure) Retryable() bool { return true }
