package model

// CodingAgentPause is a transport projection of one durable question. It does
// not grant execution authority; feedback revalidates both locked observations.
type CodingAgentPause struct {
	Schema         string   `json:"schema"`
	Generation     int64    `json:"generation"`
	StepID         int64    `json:"step_id"`
	WaitingAttempt int64    `json:"waiting_attempt"`
	Questions      []string `json:"questions"`
}
