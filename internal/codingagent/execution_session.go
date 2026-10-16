package codingagent

// ExecutionSession is code-owned native continuation metadata. It is never
// included in a model prompt or shared between tasks.
type ExecutionSession struct {
	ThreadID       string `json:"thread_id"`
	TurnID         string `json:"turn_id,omitempty"`
	PreviousTurnID string `json:"previous_turn_id,omitempty"`
	State          string `json:"state"` // idle, dispatching, running
}
