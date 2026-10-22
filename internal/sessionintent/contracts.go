// Package sessionintent defines portable session state proposal DTOs.
package sessionintent

import (
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/userintent"
)

type Item struct {
	ID          model.LedgerItemID      `json:"id"`
	Text        string                  `json:"text"`
	ReviewState model.LedgerReviewState `json:"review_state"`
	Disposition model.LedgerDisposition `json:"disposition"`
}

// These identities are supplied by code, never reconstructed by a model.
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
type Proposal struct {
	ID    model.LifecycleOperationID `json:"id"`
	Pitch string                     `json:"pitch"`
}

type Context struct {
	Recoverable   bool                      `json:"recoverable"`
	NativeChoices []string                  `json:"native_choices,omitempty"`
	Recent        []Message                 `json:"recent"`
	Facts         []string                  `json:"facts"`
	Pending       *Proposal                 `json:"pending,omitempty"`
	State         model.ChannelSessionState `json:"state"`
	JobID         int64                     `json:"job_id"`
	Generation    int64                     `json:"generation"`
	JobStatus     string                    `json:"job_status"`
	Planning      bool                      `json:"planning"`
	LedgerID      int64                     `json:"ledger_id"`
	Revision      int64                     `json:"revision"`
	PlanState     string                    `json:"plan_state"`
	InputID       *int64                    `json:"input_id,omitempty"`
	Prompt        string                    `json:"prompt,omitempty"`
	Items         []Item                    `json:"items"`
}

type Request struct {
	Mode           Mode                       `json:"mode,omitempty"`
	Recipient      Recipient                  `json:"recipient,omitempty"`
	OperationID    model.LifecycleOperationID `json:"operation_id"`
	Text           string                     `json:"text"`
	State          model.ChannelSessionState  `json:"state"`
	LedgerRevision int64                      `json:"ledger_revision"`
	SelectedItem   model.LedgerItemID         `json:"selected_item,omitempty"`
}

type Result struct {
	Error            string                     `json:"error,omitempty"`
	Pitch            string                     `json:"pitch,omitempty"`
	RequiresApproval bool                       `json:"requires_approval"`
	OperationID      model.LifecycleOperationID `json:"operation_id"`
	Patch            []userintent.Patch         `json:"patch"`
	Clarification    string                     `json:"clarification,omitempty"`
}

type Receipt struct {
	Interrupted        bool                       `json:"interrupted"`
	ExitClient         bool                       `json:"exit_client"`
	OperationID        model.LifecycleOperationID `json:"operation_id"`
	Patch              []userintent.Patch         `json:"patch"`
	JobID              int64                      `json:"job_id"`
	Generation         int64                      `json:"generation"`
	Status             string                     `json:"status"`
	Revision           int64                      `json:"revision"`
	MessageID          int64                      `json:"message_id,omitempty"`
	TurnOperationID    model.LifecycleOperationID `json:"turn_operation_id,omitempty"`
	ControlOperationID model.LifecycleOperationID `json:"control_operation_id,omitempty"`
	Summary            string                     `json:"summary"`
}

// Snapshot is a projection of authoritative persisted state and the current
// conversation. Authority is read-only. Message and native reply are absent
// until the owner supplies content; they are data, never executable work items.
type Snapshot struct {
	Authority     Context  `json:"authority"`
	JobStatus     string   `json:"job_status"`
	PlanState     string   `json:"plan_state"`
	Items         []Item   `json:"items"`
	Message       *Content `json:"message"`
	NativeReply   *string  `json:"native_reply"`
	Open          bool     `json:"open"`
	StatusVisible bool     `json:"status_visible"`
	Approval      *bool    `json:"approval"`
}
type Content struct {
	Text      string `json:"text"`
	Recipient string `json:"recipient"`
}

func State(c Context) Snapshot {
	authority := c
	authority.Facts = nil
	authority.Recent = nil
	return Snapshot{Authority: authority, JobStatus: c.JobStatus, PlanState: c.PlanState, Items: c.Items, Open: true}
}
