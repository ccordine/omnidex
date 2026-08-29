package roleplay

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// LoadFreshSimulationTurnForJob returns the immutable prepared turn only after
// code has re-derived that same narrative from current authority. This keeps a
// stale scene, persona, memory, or deterministic transition from consuming any
// inference before terminal materialization rejects it.
func (s *Store) LoadFreshSimulationTurnForJob(
	ctx context.Context,
	preparationID string,
	jobID int64,
) (SimulationTurnAuthority, error) {
	preparation, err := s.LoadSimulationTurnForJob(ctx, preparationID, jobID)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	defer tx.Rollback(context.Background())
	loaded, requestHash, exactText, err := loadMaterializationAuthorityTx(
		ctx, tx, SimulationTurnMaterializationRequest{
			PreparationID: preparation.PreparationID,
			ChannelID:     preparation.ChannelID,
			UserMessageID: preparation.UserMessageID,
			JobID:         jobID,
		},
	)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if loaded.NarrativeFingerprint != preparation.NarrativeFingerprint {
		return SimulationTurnAuthority{}, fmt.Errorf(
			"%w: loaded preparation fingerprint changed", ErrSimulationConflict,
		)
	}
	locked, err := lockSimulationSceneTx(ctx, tx, preparation.WorldID, preparation.SceneID)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if locked.Sheet.Revision != preparation.BaseSceneRevision ||
		locked.Sheet.ActiveCharacterID != preparation.ActiveCharacterID ||
		!slices.Equal(simulationParticipantIDs(locked.Participants), preparation.ParticipantCharacterIDs) {
		return SimulationTurnAuthority{}, fmt.Errorf(
			"%w: scene changed after this turn was submitted; restore and retry against the current scene",
			ErrSimulationStaleRevision,
		)
	}
	action, err := materializationAction(preparation.InputKind, exactText)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	createdAt := preparation.CreatedAt
	if preparation.PendingTransition != nil {
		createdAt = preparation.PendingTransition.CreatedAt
	}
	responderIDs := make([]string, len(preparation.Responders))
	for index, responder := range preparation.Responders {
		responderIDs[index] = responder.CharacterID
	}
	transition, responders, err := previewSimulationTurnAtTx(
		ctx, tx, locked, preparation.PreparationID, requestHash,
		exactActionText(action, exactText), action, createdAt, responderIDs,
	)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if !equalPreparedTransitionPointers(preparation.PendingTransition, transition) {
		return SimulationTurnAuthority{}, fmt.Errorf(
			"%w: deterministic simulation result changed after this turn was submitted; restore and retry",
			ErrSimulationStaleRevision,
		)
	}
	if err := requirePreparedResponderRound(preparation.Responders, responders); err != nil {
		return SimulationTurnAuthority{}, err
	}
	return preparation, nil
}

func requirePreparedResponderRound(
	expected []SimulationResponderAuthority,
	actual []SimulationResponderAuthority,
) error {
	if len(expected) != len(actual) {
		return fmt.Errorf(
			"%w: responding cast changed after this turn was submitted; restore and retry against the current cast",
			ErrSimulationStaleRevision,
		)
	}
	for index := range expected {
		if expected[index].Position != actual[index].Position ||
			expected[index].CharacterID != actual[index].CharacterID {
			return fmt.Errorf(
				"%w: responding cast order changed at position %d; restore and retry against the current cast",
				ErrSimulationStaleRevision, index,
			)
		}
		if expected[index].GenerationConfig != actual[index].GenerationConfig {
			return fmt.Errorf(
				"%w: responding character generation changed at position %d; restore and retry against the current character configuration",
				ErrSimulationStaleRevision, index,
			)
		}
		if err := requirePreparedNarrative(
			expected[index].NarrativeProjection, expected[index].NarrativeAuthority,
			actual[index].NarrativeProjection, actual[index].NarrativeAuthority,
		); err != nil {
			return err
		}
		if expected[index].NarrativeFingerprint != actual[index].NarrativeFingerprint {
			return fmt.Errorf(
				"%w: responding character narrative changed at position %d; restore and retry against the current turn state",
				ErrSimulationStaleRevision, index,
			)
		}
	}
	return nil
}

func equalPreparedTransitionPointers(expected, actual *SimulationTransitionResult) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return samePreparedTransition(*expected, *actual)
}
