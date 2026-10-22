package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func (client *Client) InterpretUserMessage(ctx context.Context, channel model.Channel, request sessionintent.Request) (sessionintent.Result, error) {
	if err := ValidateSessionTurnText(request.Text); err != nil {
		return sessionintent.Result{}, err
	}
	if _, err := model.ParseLifecycleOperationID(string(request.OperationID)); err != nil {
		return sessionintent.Result{}, err
	}
	var result sessionintent.Result
	err := client.doJSON(ctx, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/intent", request, &result, http.StatusOK)
	if err != nil {
		return result, err
	}
	if result.Error != "" || result.Clarification == "" && (result.Patch == nil) || result.OperationID != request.OperationID || result.Clarification != "" && len(result.Patch) > 0 || result.RequiresApproval && result.Pitch == "" {
		return result, fmt.Errorf("user interpretation differs from its exact message authority")
	}
	return result, nil
}

func (client *Client) ExecuteUserMessage(ctx context.Context, channel model.Channel, identity string, id model.LifecycleOperationID) (sessionintent.Receipt, error) {
	if err := channel.ID.Validate(); err != nil {
		return sessionintent.Receipt{}, err
	}
	if err := projectroot.ValidateClientWorkspaceIdentity(identity); err != nil {
		return sessionintent.Receipt{}, err
	}
	if _, err := model.ParseLifecycleOperationID(string(id)); err != nil {
		return sessionintent.Receipt{}, err
	}
	payload := struct {
		OperationID       model.LifecycleOperationID `json:"operation_id"`
		WorkspaceIdentity string                     `json:"workspace_identity"`
	}{id, identity}
	var receipt sessionintent.Receipt
	err := client.doJSON(ctx, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/intent/execute", payload, &receipt, http.StatusOK)
	if err == nil && (receipt.OperationID != id || receipt.Summary == "") {
		err = fmt.Errorf("user action receipt differs from its exact message authority")
	}
	return receipt, err
}

// CancelUserMessage never restarts semantic interpretation or dispatches an
// unresolved action. The receipt distinguishes cancellation from a prior commit.
func (client *Client) CancelUserMessage(ctx context.Context, channel model.Channel, request sessionintent.Request) (sessionintent.Receipt, error) {
	var receipt sessionintent.Receipt
	err := client.doJSON(ctx, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/intent/cancel", request, &receipt, http.StatusOK)
	if err == nil && (receipt.OperationID != request.OperationID || receipt.Summary == "") {
		err = fmt.Errorf("message interruption receipt changed authority")
	}
	return receipt, err
}
