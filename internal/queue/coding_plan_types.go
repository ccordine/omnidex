package queue

import (
	"time"

	"github.com/gryph/omnidex/internal/model"
)

// CodingLedgerRoute is the immutable inference route recorded with every
// normalized ledger revision.
type CodingLedgerRoute struct {
	Role                  string `json:"role"`
	ConfigurationRevision int64  `json:"configuration_revision"`
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	ReasoningEffort       string `json:"reasoning_effort"`
	ServiceTier           string `json:"service_tier"`
}

type CreateCodingLedgerCommand struct {
	JobID                int64                      `json:"job_id"`
	OperationID          string                     `json:"operation_id"`
	OwnerTurnID          string                     `json:"owner_turn_id"`
	OwnerTurnText        string                     `json:"owner_turn_text"`
	OwnerTurnDisposition model.OwnerTurnDisposition `json:"owner_turn_disposition"`
	Ledger               model.EngineeringLedger    `json:"ledger"`
	Routes               []CodingLedgerRoute        `json:"routes"`
}

type ApplyCodingLedgerMutationCommand struct {
	LedgerID      int64                `json:"ledger_id"`
	OwnerTurnText string               `json:"owner_turn_text"`
	Mutation      model.LedgerMutation `json:"mutation"`
	Routes        []CodingLedgerRoute  `json:"routes,omitempty"`
}

type FreezeCodingLedgerCommand struct {
	LedgerID      int64  `json:"ledger_id"`
	Revision      int64  `json:"revision"`
	OperationID   string `json:"operation_id"`
	OwnerTurnID   string `json:"owner_turn_id"`
	OwnerTurnText string `json:"owner_turn_text"`
}

type CodingPlanningAmendment struct {
	OperationID model.LifecycleOperationID `json:"operation_id"`
	Revision    int64                      `json:"revision"`
	Text        string                     `json:"text"`
}

type CodingLedgerRevision struct {
	PlanningMessages      []model.LedgerPlanningMessage     `json:"planning_messages,omitempty"`
	PendingAmendments     []CodingPlanningAmendment         `json:"pending_amendments,omitempty"`
	LedgerID              int64                             `json:"ledger_id"`
	JobID                 int64                             `json:"job_id"`
	Generation            int64                             `json:"generation"`
	Revision              int64                             `json:"revision"`
	BaseRevision          int64                             `json:"base_revision"`
	OperationID           string                            `json:"operation_id"`
	State                 string                            `json:"state"`
	OwnerTurnID           string                            `json:"owner_turn_id,omitempty"`
	OwnerTurnDispositions []model.OwnerStatementDisposition `json:"owner_turn_dispositions,omitempty"`
	LatestDelta           CodingLedgerDiff                  `json:"latest_delta"`
	CreatedAt             time.Time                         `json:"created_at"`
	UpdatedAt             time.Time                         `json:"updated_at"`
	Ledger                model.EngineeringLedger           `json:"ledger"`
	Routes                []CodingLedgerRoute               `json:"routes"`
	Applied               bool                              `json:"applied"`
}

type CodingLedgerReadOptions struct {
	Revision      int64
	Limit         int
	Offset        int
	Kinds         []string
	ReviewState   model.LedgerReviewState
	Search        string
	Removed       *bool
	ItemIDs       []model.LedgerItemID
	RelatedTaskID model.LedgerItemID
}

type CodingLedgerPage struct {
	Limit  int
	Offset int
}

type CodingLedgerDiff struct {
	LedgerID     int64                `json:"ledger_id"`
	FromRevision int64                `json:"from_revision"`
	ToRevision   int64                `json:"to_revision"`
	Added        []model.LedgerItemID `json:"added"`
	Amended      []model.LedgerItemID `json:"amended"`
	Removed      []model.LedgerItemID `json:"removed"`
	Restored     []model.LedgerItemID `json:"restored"`
	Limit        int                  `json:"limit"`
	Offset       int                  `json:"offset"`
	HasMore      bool                 `json:"has_more"`
}

type CodingLedgerItemVersion struct {
	ItemID      model.LedgerItemID      `json:"item_id"`
	Kind        string                  `json:"kind"`
	Revision    int64                   `json:"revision"`
	Disposition model.LedgerDisposition `json:"disposition"`
	ReviewState model.LedgerReviewState `json:"review_state,omitempty"`
	Rank        int64                   `json:"rank"`
	Payload     []byte                  `json:"payload"`
	Rationale   string                  `json:"rationale"`
}
