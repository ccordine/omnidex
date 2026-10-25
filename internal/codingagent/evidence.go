package codingagent

import (
	"context"
	"encoding/json"
)

type Entry struct {
	PlanningProposal *EngineeringLedger    `json:"planning_proposal,omitempty"`
	TaskOrdering     *TaskOrderingEvidence `json:"task_ordering,omitempty"`
	ID               int64                 `json:"id,omitempty"`
	Generation       int64                 `json:"generation"`
	Phase            string                `json:"phase"`
	TaskID           string                `json:"task_id,omitempty"`
	Round            int64                 `json:"round,omitempty"`
	CallID           int64                 `json:"call_id,omitempty"`
	Role             Role                  `json:"role,omitempty"`
	Assignment       *Assignment           `json:"assignment,omitempty"`
	Result           *Result               `json:"result,omitempty"`
	// Bytes are base64 encoded in JSON so JSONB preserves the original wire frame.
	Frame               []byte                `json:"frame,omitempty"`
	Findings            []Finding             `json:"findings,omitempty"`
	WrittenFiles        []string              `json:"written_files,omitempty"`
	Notes               []string              `json:"notes,omitempty"`
	ConstructionFacts   []ConstructionFact    `json:"construction_facts,omitempty"`
	ResponseKind        ExecutionResponseKind `json:"response_kind,omitempty"`
	Session             *ExecutionSession     `json:"session,omitempty"`
	NativeInput         *NativeInputRequest   `json:"native_input,omitempty"`
	InputID             int64                 `json:"input_id,omitempty"`
	NativeReply         json.RawMessage       `json:"native_reply,omitempty"`
	ParserCallID        int64                 `json:"parser_call_id,omitempty"`
	ReviewResponse      *ReviewResponseResult `json:"review_response,omitempty"`
	Continuation        *TaskContinuation     `json:"continuation,omitempty"`
	NewerOwnerDirection *PlanningDirection    `json:"newer_owner_direction,omitempty"`
}

// EvidenceStore is the existing queue's journal boundary. It is not a second
// scheduler: the enclosing claimed step owns leases, cancellation and ordering.
type EvidenceStore interface {
	Append(context.Context, Entry) (int64, error)
	Read(context.Context) ([]Entry, error)
	ReadTaskClarifications(context.Context, string) ([]TaskClarification, error)
}

type Environment interface {
	Attest(context.Context) error
}
