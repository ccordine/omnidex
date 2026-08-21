package roleplay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// previewSimulationTurnTx derives the exact post-transition narrative inside a
// nested transaction and always rolls it back. The parent transaction persists
// only immutable preparation authority; no meter, inventory, scene, or
// transition change becomes authoritative before terminal completion.
func previewSimulationTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	locked lockedSimulationScene,
	operationID, requestHash, exactAction string,
	action *SimulationAction,
	responderIDs []string,
) (*SimulationTransitionResult, []SimulationResponderAuthority, error) {
	return previewSimulationTurnAtTx(
		ctx, tx, locked, operationID, requestHash, exactAction, action,
		time.Now().UTC().Truncate(time.Microsecond), responderIDs,
	)
}

func previewSimulationTurnAtTx(
	ctx context.Context,
	tx pgx.Tx,
	locked lockedSimulationScene,
	operationID, requestHash, exactAction string,
	action *SimulationAction,
	createdAt time.Time,
	responderIDs []string,
) (*SimulationTransitionResult, []SimulationResponderAuthority, error) {
	preview, err := tx.Begin(ctx)
	if err != nil {
		return nil, nil,
			fmt.Errorf("begin simulation preview: %w", err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = preview.Rollback(context.Background())
		}
	}()
	transition, _, err := applySimulationStateTx(
		ctx, preview, locked, operationID, requestHash, exactAction, action, createdAt,
	)
	if err != nil {
		return nil, nil, err
	}
	responders := make([]SimulationResponderAuthority, len(responderIDs))
	for index, characterID := range responderIDs {
		content, authority, projectionErr := projectSimulationNarrativeTx(
			ctx, preview, locked.Sheet.WorldID, characterID,
		)
		if projectionErr != nil {
			return nil, nil, projectionErr
		}
		generation, generationErr := projectCharacterGenerationTx(
			ctx, preview, locked.Sheet.WorldID, characterID,
		)
		if generationErr != nil {
			return nil, nil, generationErr
		}
		responders[index] = SimulationResponderAuthority{
			Position: index, CharacterID: characterID, GenerationConfig: generation.Config,
			NarrativeProjection: content, NarrativeAuthority: authority,
			NarrativeFingerprint: authority.Fingerprint,
		}
	}
	if err := preview.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return nil, nil,
			fmt.Errorf("rollback simulation preview: %w", err)
	}
	rolledBack = true
	return transition, responders, nil
}
