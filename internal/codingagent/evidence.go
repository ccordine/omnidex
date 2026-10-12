package codingagent

import (
	"context"
)

type Entry struct {
	ID             int64       `json:"id,omitempty"`
	Generation     int64       `json:"generation"`
	Phase          string      `json:"phase"`
	TaskID         string      `json:"task_id,omitempty"`
	Round          int64       `json:"round,omitempty"`
	CallID         int64       `json:"call_id,omitempty"`
	OperationID    string      `json:"operation_id,omitempty"`
	QuestionID     int64       `json:"question_id,omitempty"`
	WaitingAttempt int64       `json:"waiting_attempt,omitempty"`
	Role           Role        `json:"role,omitempty"`
	Assignment     *Assignment `json:"assignment,omitempty"`
	Workload       *Workload   `json:"workload,omitempty"`
	Result         *Result     `json:"result,omitempty"`
	// Bytes are base64 encoded in JSON so JSONB preserves the original wire frame.
	Frame             []byte             `json:"frame,omitempty"`
	Findings          []Finding          `json:"findings,omitempty"`
	Checks            []CheckResult      `json:"checks,omitempty"`
	Questions         []string           `json:"questions,omitempty"`
	Answers           []string           `json:"answers,omitempty"`
	Notes             []string           `json:"notes,omitempty"`
	ConstructionFacts []ConstructionFact `json:"construction_facts,omitempty"`
}

// EvidenceStore is the existing queue's journal boundary. It is not a second
// scheduler: the enclosing claimed step owns leases, cancellation and ordering.
type EvidenceStore interface {
	Append(context.Context, Entry) (int64, error)
	Read(context.Context) ([]Entry, error)
}

type Verifier interface {
	ExecuteChecks(context.Context, Assignment, []Check) ([]CheckResult, error)
}
type Environment interface {
	ReadFiles(context.Context) ([]string, error)
	PrepareFiles(context.Context, Task) error
	Attest(context.Context) error
	WriteFile(context.Context, FileWrite) error
}

type QuestionsPending struct{ Questions []string }

func (pending *QuestionsPending) Error() string { return "scoped coding task is waiting for answers" }
