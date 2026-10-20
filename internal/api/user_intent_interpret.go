package api

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
)

func interpretSessionUserIntent(ctx context.Context, request sessionintent.Request, current sessionintent.Context, ask userintent.Ask) (sessionintent.Result, error) {
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
	proposed, err := userintent.Interpret(ctx, userintent.Input{Snapshot: snapshot, Contract: contract, Prompt: request.Text, Context: background, Writable: sessionintent.Writable, Validate: sessionintent.Validate}, ask)
	if err != nil {
		return result, err
	}
	result.Patch, result.Clarification = proposed.Patch, proposed.Question
	return result, nil
}
