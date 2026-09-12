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
	ResolveOngoingActionRelation(
		context.Context,
		assemblyline.RoleplayOngoingActionRelationInput,
	) (
		assemblyline.RoleplayOngoingActionRelation, int, error,
	)
	GenerateOngoingActionValue(
		context.Context,
		assemblyline.RoleplayOngoingActionValueInput,
	) (
		string, int, error,
	)
}

type objectiveRoleplayOngoingActionResult struct {
	Action              *string
	RequiresRestoration bool
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
	extracted, calls, err := extractRoleplayOngoingAction(
		ctx, station, assemblyline.RoleplayOngoingActionSourceUserAction,
		preparation.UserTurn.PersonaName, contribution, previous,
	)
	if err != nil {
		return nil, calls, fmt.Errorf("extract roleplay user ongoing action: %w", err)
	}
	resolved := extracted.Action
	if extracted.RequiresRestoration {
		resolved, err = restoreObjectiveOptionalModelText(
			authority, "roleplay user ongoing action", resolved,
		)
		if err != nil {
			return nil, calls, err
		}
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
) (objectiveRoleplayOngoingActionResult, int, error) {
	if ctx == nil {
		return objectiveRoleplayOngoingActionResult{}, 0, fmt.Errorf("roleplay ongoing-action extraction requires context authority")
	}
	if station == nil {
		return objectiveRoleplayOngoingActionResult{}, 0, fmt.Errorf("roleplay ongoing-action station is unavailable")
	}
	var previous *string
	if previousOngoingAction != nil {
		copy := *previousOngoingAction
		previous = &copy
	}
	relationInput := assemblyline.RoleplayOngoingActionRelationInput{
		CharacterName:         characterName,
		Source:                source,
		ExactContribution:     exactContribution,
		PreviousOngoingAction: previous,
	}
	if _, err := assemblyline.NewRoleplayOngoingActionRelationJob(relationInput); err != nil {
		return objectiveRoleplayOngoingActionResult{}, 0, err
	}
	relation, relationDispatches, err := station.ResolveOngoingActionRelation(ctx, relationInput)
	calls := relationDispatches
	if err != nil {
		return objectiveRoleplayOngoingActionResult{}, calls, err
	}
	if err := validateObjectiveLeafCallCount(
		"roleplay ongoing-action relation station", relationDispatches,
	); err != nil {
		return objectiveRoleplayOngoingActionResult{}, calls, err
	}
	if err := relation.ValidateFor(relationInput); err != nil {
		return objectiveRoleplayOngoingActionResult{}, calls, err
	}
	switch relation {
	case assemblyline.RoleplayOngoingActionAbsent:
		return objectiveRoleplayOngoingActionResult{}, calls, nil
	case assemblyline.RoleplayOngoingActionUnchanged:
		copy := *previous
		return objectiveRoleplayOngoingActionResult{Action: &copy}, calls, nil
	case assemblyline.RoleplayOngoingActionReplacement:
		valueInput := assemblyline.RoleplayOngoingActionValueInput{
			CharacterName: characterName, Source: source,
			ExactContribution: exactContribution,
		}
		if _, err := assemblyline.NewRoleplayOngoingActionValueJob(valueInput); err != nil {
			return objectiveRoleplayOngoingActionResult{}, calls, err
		}
		action, valueDispatches, err := station.GenerateOngoingActionValue(ctx, valueInput)
		calls += valueDispatches
		if err != nil {
			return objectiveRoleplayOngoingActionResult{}, calls, err
		}
		if err := validateObjectiveLeafCallCount(
			"roleplay ongoing-action value station", valueDispatches,
		); err != nil {
			return objectiveRoleplayOngoingActionResult{}, calls, err
		}
		if err := roleplay.ValidateOngoingActionText(action); err != nil {
			return objectiveRoleplayOngoingActionResult{}, calls, err
		}
		if previous != nil && action == *previous {
			copy := *previous
			return objectiveRoleplayOngoingActionResult{Action: &copy}, calls, nil
		}
		copy := action
		return objectiveRoleplayOngoingActionResult{
			Action: &copy, RequiresRestoration: true,
		}, calls, nil
	default:
		return objectiveRoleplayOngoingActionResult{}, calls, fmt.Errorf("roleplay ongoing-action relation is not registered")
	}
}
