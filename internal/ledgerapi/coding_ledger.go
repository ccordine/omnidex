// Package ledgerapi owns the HTTP representation of the coding ledger.
// Persistence projections must be mapped into these types before transport.
package ledgerapi

import (
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type CodingLedgerRoute struct {
	Role                  string `json:"role"`
	ConfigurationRevision int64  `json:"configuration_revision"`
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	ReasoningEffort       string `json:"reasoning_effort"`
	ServiceTier           string `json:"service_tier"`
}

// CodingLedgerDelta is the semantic change recorded with one revision. Page
// controls belong only to repository history queries and are not part of this
// response contract.
type CodingLedgerDelta struct {
	LedgerID     int64                `json:"ledger_id"`
	FromRevision int64                `json:"from_revision"`
	ToRevision   int64                `json:"to_revision"`
	Added        []model.LedgerItemID `json:"added"`
	Amended      []model.LedgerItemID `json:"amended"`
	Removed      []model.LedgerItemID `json:"removed"`
	Restored     []model.LedgerItemID `json:"restored"`
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
	LatestDelta           CodingLedgerDelta                 `json:"latest_delta"`
	CreatedAt             time.Time                         `json:"created_at"`
	UpdatedAt             time.Time                         `json:"updated_at"`
	Ledger                model.EngineeringLedger           `json:"ledger"`
	Routes                []CodingLedgerRoute               `json:"routes"`
	Applied               bool                              `json:"applied"`
}
