package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

type CodingLedgerRevision struct {
	LedgerID              int64                             `json:"ledger_id"`
	JobID                 int64                             `json:"job_id"`
	Generation            int64                             `json:"generation"`
	Revision              int64                             `json:"revision"`
	BaseRevision          int64                             `json:"base_revision"`
	OperationID           string                            `json:"operation_id"`
	State                 string                            `json:"state"`
	CoveragePassed        bool                              `json:"coverage_passed"`
	AuditID               string                            `json:"audit_id,omitempty"`
	OwnerTurnID           string                            `json:"owner_turn_id,omitempty"`
	OwnerTurnDispositions []model.OwnerStatementDisposition `json:"owner_turn_dispositions,omitempty"`
	LatestDelta           CodingLedgerDiff                  `json:"latest_delta"`
	CreatedAt             time.Time                         `json:"created_at"`
	UpdatedAt             time.Time                         `json:"updated_at"`
	Ledger                model.EngineeringLedger           `json:"ledger"`
	Routes                []CodingLedgerRoute               `json:"routes"`
	Applied               bool                              `json:"applied"`
}

type CodingLedgerDiff struct {
	LedgerID     int64                `json:"ledger_id"`
	FromRevision int64                `json:"from_revision"`
	ToRevision   int64                `json:"to_revision"`
	Added        []model.LedgerItemID `json:"added"`
	Amended      []model.LedgerItemID `json:"amended"`
	Removed      []model.LedgerItemID `json:"removed"`
	Restored     []model.LedgerItemID `json:"restored"`
}

type PlanningTurn struct {
	OperationID, OwnerTurnID, Text string
	ObservedRevision               int64
}
type DirectPlanningAction struct {
	Kind        model.LedgerMutationKind `json:"kind"`
	TargetID    model.LedgerItemID       `json:"target_id"`
	ReviewState model.LedgerReviewState  `json:"review_state,omitempty"`
	Rank        *int64                   `json:"rank,omitempty"`
	Rationale   string                   `json:"rationale"`
}

func (client *Client) CodingLedger(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64) (CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return CodingLedgerRevision{}, err
	}
	query := url.Values{"workspace_root": {channel.WorkspaceRoot}, "workspace_identity": {workspaceIdentity}}
	var result CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d/plan?%s", jobID, query.Encode()), nil, &result, http.StatusOK); err != nil {
		return CodingLedgerRevision{}, err
	}
	if result.JobID != jobID || result.LedgerID < 1 || result.Revision < 1 || result.Ledger.Revision != result.Revision {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger response differs from requested authority")
	}
	if err := result.Ledger.ValidateDraft(); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("invalid coding ledger response: %w", err)
	}
	return result, nil
}

func (client *Client) MutateCodingLedgerDirect(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64, turn PlanningTurn, action DirectPlanningAction) (CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return CodingLedgerRevision{}, err
	}
	payload := struct {
		OperationID       string               `json:"operation_id"`
		OwnerTurnID       string               `json:"owner_turn_id"`
		OwnerTurnText     string               `json:"owner_turn_text"`
		Revision          int64                `json:"revision"`
		Action            DirectPlanningAction `json:"action"`
		WorkspaceRoot     string               `json:"workspace_root"`
		WorkspaceIdentity string               `json:"workspace_identity"`
	}{turn.OperationID, turn.OwnerTurnID, turn.Text, turn.ObservedRevision, action, channel.WorkspaceRoot, workspaceIdentity}
	var result CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/plan/decisions", jobID), payload, &result, http.StatusOK); err != nil {
		return CodingLedgerRevision{}, err
	}
	return result, nil
}

func (client *Client) FreezeCodingLedger(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64, turn PlanningTurn) (CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return CodingLedgerRevision{}, err
	}
	payload := struct {
		OperationID       string `json:"operation_id"`
		OwnerTurnID       string `json:"owner_turn_id"`
		OwnerTurnText     string `json:"owner_turn_text"`
		Revision          int64  `json:"revision"`
		WorkspaceRoot     string `json:"workspace_root"`
		WorkspaceIdentity string `json:"workspace_identity"`
	}{turn.OperationID, turn.OwnerTurnID, turn.Text, turn.ObservedRevision, channel.WorkspaceRoot, workspaceIdentity}
	var result CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/plan/freeze", jobID), payload, &result, http.StatusOK); err != nil {
		return CodingLedgerRevision{}, err
	}
	if result.State != "frozen" {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger freeze response has state %q", result.State)
	}
	return result, nil
}
