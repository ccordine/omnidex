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
) (*SimulationTransitionResult, NarrativeSimulationProjection, SimulationNarrativeAuthority, error) {
	preview, err := tx.Begin(ctx)
	if err != nil {
		return nil, NarrativeSimulationProjection{}, SimulationNarrativeAuthority{},
			fmt.Errorf("begin simulation preview: %w", err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = preview.Rollback(context.Background())
		}
	}()
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	transition, _, err := applySimulationStateTx(
		ctx, preview, locked, operationID, requestHash, exactAction, action, createdAt,
	)
	if err != nil {
		return nil, NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	content, authority, err := projectSimulationNarrativeTx(
		ctx, preview, locked.Sheet.WorldID, locked.Sheet.ActiveCharacterID,
	)
	if err != nil {
		return nil, NarrativeSimulationProjection{}, SimulationNarrativeAuthority{}, err
	}
	if err := preview.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return nil, NarrativeSimulationProjection{}, SimulationNarrativeAuthority{},
			fmt.Errorf("rollback simulation preview: %w", err)
	}
	rolledBack = true
	return transition, content, authority, nil
}
