package api

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
)

func interpretSessionUserIntent(ctx context.Context, request sessionintent.Request, current sessionintent.Context, ask userintent.Ask) (sessionintent.Result, error) {
	if err := request.ValidateRouting(current); err != nil {
		return sessionintent.Result{}, err
	}
	if request.Mode == sessionintent.Interact && sessionintent.InteractionRecipient(current) != sessionintent.Approval && (sessionintent.InteractionRecipient(current) != sessionintent.Native || len(current.NativeChoices) == 0) {
		return sessionintent.DirectContent(request, current)
	}
	result := sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{}}
	background := userintent.Context{Facts: append([]string{}, current.Facts...), Recent: []userintent.Message{}}
	for _, m := range current.Recent {
		background.Recent = append(background.Recent, userintent.Message{Role: m.Role, Text: m.Text})
	}
	if current.Pending != nil {
		background.Pending = current.Pending.Pitch
	}
	snapshot, err := json.Marshal(sessionintent.State(current))
	if err != nil {
		return result, err
	}
	contract, err := sessionintent.StateContract(current)
	if err != nil {
		return result, err
	}
	writable := sessionintent.Writable
	if request.Mode == sessionintent.Interact {
		if request.Recipient == "" {
			request.Recipient = sessionintent.InteractionRecipient(current)
		}
		path := "/approval"
		if request.Recipient == sessionintent.Native {
			path = "/native_reply"
		}
		writable = func(candidate string) bool { return candidate == path }
	}
	proposed, err := userintent.Interpret(ctx, userintent.Input{Snapshot: snapshot, Contract: contract, Prompt: request.Text, Context: background, Writable: writable, Validate: sessionintent.Validate}, ask)
	if err != nil {
		return result, err
	}
	result.Patch, result.Clarification = proposed.Patch, proposed.Question
	return result, nil
}
