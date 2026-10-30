package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

type PlanningTurn struct {
	OperationID, OwnerTurnID, Text string
	ObservedRevision               int64
}
type DirectPlanningAction struct {
	Move        int                      `json:"move,omitempty"`
	Item        *model.LedgerTask        `json:"item,omitempty"`
	Kind        model.LedgerMutationKind `json:"kind"`
	TargetID    model.LedgerItemID       `json:"target_id"`
	ReviewState model.LedgerReviewState  `json:"review_state,omitempty"`
	Rank        *int64                   `json:"rank,omitempty"`
	Rationale   string                   `json:"rationale"`
}

func (client *Client) PlanCodingLedgerItem(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64, turn PlanningTurn, itemID model.LedgerItemID, manual ...bool) (model.Job, error) {
	payload := struct {
		Manual            bool               `json:"manual,omitempty"`
		OperationID       string             `json:"operation_id"`
		ItemID            model.LedgerItemID `json:"item_id"`
		Text              string             `json:"text"`
		Revision          int64              `json:"revision"`
		WorkspaceRoot     string             `json:"workspace_root"`
		WorkspaceIdentity string             `json:"workspace_identity"`
	}{len(manual) > 0 && manual[0], turn.OperationID, itemID, turn.Text, turn.ObservedRevision, channel.WorkspaceRoot, workspaceIdentity}
	return client.postLifecycleControl(ctx, channel, workspaceIdentity, jobID, model.LifecycleOperationID(turn.OperationID), "plan/item-planner", payload)
}

func (client *Client) CodingLedger(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64) (ledgerapi.CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
	}
	query := url.Values{"workspace_root": {channel.WorkspaceRoot}, "workspace_identity": {workspaceIdentity}}
	var result ledgerapi.CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d/plan?%s", jobID, query.Encode()), nil, &result, http.StatusOK); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
	}
	if result.JobID != jobID || result.LedgerID < 1 || result.Revision < 1 || result.Ledger.Revision != result.Revision {
		return ledgerapi.CodingLedgerRevision{}, fmt.Errorf("coding ledger response differs from requested authority")
	}
	if err := result.Ledger.ValidateDraft(); err != nil {
		return ledgerapi.CodingLedgerRevision{}, fmt.Errorf("invalid coding ledger response: %w", err)
	}
	return result, nil
}

func (client *Client) MutateCodingLedgerDirect(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64, turn PlanningTurn, action DirectPlanningAction) (ledgerapi.CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
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
	var result ledgerapi.CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/plan/decisions", jobID), payload, &result, http.StatusOK); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
	}
	return result, nil
}

func (client *Client) FreezeCodingLedger(ctx context.Context, channel model.Channel, workspaceIdentity string, jobID int64, turn PlanningTurn) (ledgerapi.CodingLedgerRevision, error) {
	if err := validateCodingPlanAuthority(channel, workspaceIdentity, jobID); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
	}
	payload := struct {
		OperationID       string `json:"operation_id"`
		OwnerTurnID       string `json:"owner_turn_id"`
		OwnerTurnText     string `json:"owner_turn_text"`
		Revision          int64  `json:"revision"`
		WorkspaceRoot     string `json:"workspace_root"`
		WorkspaceIdentity string `json:"workspace_identity"`
	}{turn.OperationID, turn.OwnerTurnID, turn.Text, turn.ObservedRevision, channel.WorkspaceRoot, workspaceIdentity}
	var result ledgerapi.CodingLedgerRevision
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/plan/freeze", jobID), payload, &result, http.StatusOK); err != nil {
		return ledgerapi.CodingLedgerRevision{}, err
	}
	if result.State != "frozen" {
		return ledgerapi.CodingLedgerRevision{}, fmt.Errorf("coding ledger freeze response has state %q", result.State)
	}
	return result, nil
}
