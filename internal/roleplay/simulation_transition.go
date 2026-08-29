package roleplay

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func applySimulationStateTx(
	ctx context.Context,
	tx pgx.Tx,
	locked lockedSimulationScene,
	operationID, requestHash, exactAction string,
	action *SimulationAction,
	createdAt time.Time,
) (*SimulationTransitionResult, bool, error) {
	if createdAt.IsZero() {
		return nil, false, fmt.Errorf("simulation transition requires an exact creation time")
	}
	createdAt = createdAt.UTC().Truncate(time.Microsecond)
	effects := make([]SimulationEffect, 0, MaxTransitionEffects)
	narrativeEvents := make([]string, 0, 2)
	resolvedAction := SimulationAction{Kind: SimulationActionAutomatic}
	if action != nil {
		resolvedAction = *action
		if err := applyExplicitSimulationActionTx(
			ctx, tx, operationID, locked.Sheet.WorldID, locked.Sheet.ActiveCharacterID, *action,
			&effects, &narrativeEvents,
		); err != nil {
			return nil, false, err
		}
	}
	autoUsed, err := applySimulationAutoUseTx(
		ctx, tx, locked.Sheet.WorldID, locked.Sheet.ActiveCharacterID, &effects, &narrativeEvents,
	)
	if err != nil {
		return nil, false, err
	}
	if action == nil && !autoUsed {
		return nil, false, nil
	}
	if len(effects) < 1 || len(effects) > MaxTransitionEffects {
		return nil, false, fmt.Errorf("%w: transition effects are outside their bound", ErrSimulationNotConfigured)
	}
	if len(narrativeEvents) < 1 || len(narrativeEvents) > 2 {
		return nil, false, fmt.Errorf("%w: transition narrative events are outside their bound", ErrSimulationNotConfigured)
	}
	afterRevision, err := updateSceneRevisionTx(
		ctx, tx, locked.Sheet.ID, locked.Sheet.Revision, locked.Sheet.ActiveCharacterID,
	)
	if err != nil {
		return nil, false, err
	}
	result := SimulationTransitionResult{
		Schema: SimulationTransitionSchemaV1, OperationID: operationID,
		WorldID: locked.Sheet.WorldID, SceneID: locked.Sheet.ID,
		ActorCharacterID: locked.Sheet.ActiveCharacterID,
		BeforeRevision:   locked.Sheet.Revision, AfterRevision: afterRevision,
		Action: resolvedAction, Effects: effects, NarrativeEvents: narrativeEvents,
		CreatedAt: createdAt,
	}
	observerCharacterIDs := simulationParticipantIDs(locked.Participants)
	if err := persistSimulationTransitionTx(
		ctx, tx, requestHash, exactAction, observerCharacterIDs, result,
	); err != nil {
		return nil, false, err
	}
	return &result, autoUsed, nil
}
