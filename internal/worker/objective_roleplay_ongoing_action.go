package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

type objectiveRoleplayOngoingActionStation interface {
	ResolveOngoingAction(context.Context, assemblyline.RoleplayOngoingActionInput) (
		assemblyline.RoleplayOngoingActionDecision, objectiveStationReceipt, error,
	)
}

func resolveRoleplayUserOngoingAction(
	ctx context.Context,
	station objectiveRoleplayOngoingActionStation,
	preparation roleplay.SimulationTurnAuthority,
	modelUserTurn roleplay.UserTurnAuthority,
	authority turnAuthority,
) (*queue.RoleplayUserOngoingActionCompletion, int, error) {
	contribution, required, err := modelUserTurn.OngoingActionContribution()
	if err != nil {
		return nil, 0, fmt.Errorf("resolve typed roleplay user action authority: %w", err)
	}
	if !required {
		return nil, 0, nil
	}
	previous, err := roleplay.CurrentOngoingActionForCharacter(
		preparation.NarrativeProjection, preparation.NarrativeAuthority,
		preparation.UserTurn.CharacterID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve roleplay user ongoing-action authority: %w", err)
	}
	resolved, calls, err := extractRoleplayOngoingAction(
		ctx, station, assemblyline.RoleplayOngoingActionSourceUserAction,
		preparation.UserTurn.PersonaName, contribution, previous,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("extract roleplay user ongoing action: %w", err)
	}
	resolved, err = restoreObjectiveOptionalModelText(
		authority, "roleplay user ongoing action", resolved,
	)
	if err != nil {
		return nil, 0, err
	}
	return &queue.RoleplayUserOngoingActionCompletion{
		CharacterID:           model.RoleplayCharacterID(preparation.UserTurn.CharacterID),
		PreviousOngoingAction: previous, OngoingAction: resolved,
	}, calls, nil
}

func extractRoleplayOngoingAction(
	ctx context.Context,
	station objectiveRoleplayOngoingActionStation,
	source assemblyline.RoleplayOngoingActionSource,
	characterName, exactContribution string,
	previousOngoingAction *string,
) (*string, int, error) {
	if ctx == nil {
		return nil, 0, fmt.Errorf("roleplay ongoing-action extraction requires context authority")
	}
	if station == nil {
		return nil, 0, fmt.Errorf("roleplay ongoing-action station is unavailable")
	}
	var previous *string
	if previousOngoingAction != nil {
		copy := *previousOngoingAction
		previous = &copy
	}
	input := assemblyline.RoleplayOngoingActionInput{
		CharacterName:         characterName,
		Source:                source,
		ExactContribution:     exactContribution,
		PreviousOngoingAction: previous,
	}
	if _, err := assemblyline.NewRoleplayOngoingActionJob(input); err != nil {
		return nil, 0, err
	}
	decision, receipt, err := station.ResolveOngoingAction(ctx, input)
	if err != nil {
		return nil, 0, err
	}
	if err := validateObjectiveStationReceipt("roleplay ongoing-action station", receipt); err != nil {
		return nil, 0, err
	}
	action, err := decision.ResolveFor(input)
	if err != nil {
		return nil, 0, err
	}
	if action == nil {
		return nil, receipt.Calls, nil
	}
	copy := *action
	return &copy, receipt.Calls, nil
}
