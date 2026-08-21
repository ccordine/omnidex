package roleplay

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"
)

// MaterializeSimulationTurnTx publishes one previously previewed transition.
// The caller owns the terminal transaction that also persists the assistant
// response, canon/research receipt, and turn advance.
func MaterializeSimulationTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnMaterializationRequest,
) error {
	preparation, requestHash, exactText, err := loadMaterializationAuthorityTx(ctx, tx, request)
	if err != nil {
		return err
	}
	if preparation.PendingTransition != nil {
		stored, found, err := loadSimulationTransitionTx(
			ctx, tx, preparation.PendingTransition.OperationID, requestHash,
		)
		if err != nil {
			return err
		}
		if found {
			if !samePreparedTransition(*preparation.PendingTransition, stored) {
				return fmt.Errorf("%w: materialized transition differs from preparation", ErrSimulationConflict)
			}
			return nil
		}
	}
	locked, err := lockSimulationSceneTx(ctx, tx, preparation.WorldID, preparation.SceneID)
	if err != nil {
		return err
	}
	if locked.Sheet.Revision != preparation.BaseSceneRevision ||
		locked.Sheet.ActiveCharacterID != preparation.ActiveCharacterID ||
		!slices.Equal(simulationParticipantIDs(locked.Participants), preparation.ParticipantCharacterIDs) {
		return fmt.Errorf("%w: simulation state changed after preparation", ErrSimulationStaleRevision)
	}
	if preparation.PendingTransition == nil {
		return requirePreparedNarrativeTx(ctx, tx, preparation)
	}
	action, err := materializationAction(preparation.InputKind, exactText)
	if err != nil {
		return err
	}
	actual, _, err := applySimulationStateTx(
		ctx, tx, locked, preparation.PendingTransition.OperationID,
		requestHash, exactActionText(action, exactText), action,
		preparation.PendingTransition.CreatedAt,
	)
	if err != nil {
		return err
	}
	if actual == nil || !samePreparedTransition(*preparation.PendingTransition, *actual) {
		return fmt.Errorf("%w: materialized transition differs from preparation", ErrSimulationConflict)
	}
	return requirePreparedNarrativeTx(ctx, tx, preparation)
}

// RequireSimulationTurnMaterializedReplayTx validates the immutable transition
// receipt without attempting to mutate already-advanced scene state.
func RequireSimulationTurnMaterializedReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnMaterializationRequest,
) error {
	preparation, requestHash, _, err := loadMaterializationAuthorityTx(ctx, tx, request)
	if err != nil {
		return err
	}
	if preparation.PendingTransition == nil {
		return nil
	}
	stored, found, err := loadSimulationTransitionTx(
		ctx, tx, preparation.PendingTransition.OperationID, requestHash,
	)
	if err != nil {
		return err
	}
	if !found || !samePreparedTransition(*preparation.PendingTransition, stored) {
		return fmt.Errorf("%w: prepared transition receipt is absent or changed", ErrSimulationConflict)
	}
	return nil
}

func loadMaterializationAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnMaterializationRequest,
) (SimulationTurnAuthority, string, string, error) {
	if ctx == nil || tx == nil || request.JobID < 1 || request.UserMessageID < 1 {
		return SimulationTurnAuthority{}, "", "", fmt.Errorf("simulation materialization requires exact transaction, message, and job authority")
	}
	if err := validateIdentity(request.PreparationID, transitionIdentity); err != nil {
		return SimulationTurnAuthority{}, "", "", err
	}
	if err := validateChannelID(request.ChannelID); err != nil {
		return SimulationTurnAuthority{}, "", "", err
	}
	var payload []byte
	var requestHash, exactText string
	err := tx.QueryRow(ctx, `
		SELECT preparation.result,preparation.request_sha256,message.content
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		JOIN jobs AS job ON job.id=binding.job_id
		JOIN ai_channel_messages AS message
		  ON message.id=preparation.user_message_id AND message.channel_id=preparation.channel_id
		WHERE preparation.operation_id=$1 AND preparation.channel_id=$2
		  AND preparation.user_message_id=$3 AND binding.job_id=$4
		  AND message.role='user' AND job.pipeline='chat' AND job.instruction=message.content
		  AND job.metadata->>'channel_id'=preparation.channel_id
		  AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
		  AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
		  AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
		  AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
		  AND job.metadata->'roleplay_responders'=preparation.result->'responder_routes'
		  AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
	`, request.PreparationID, request.ChannelID, request.UserMessageID, request.JobID).Scan(
		&payload, &requestHash, &exactText,
	)
	if err == pgx.ErrNoRows {
		return SimulationTurnAuthority{}, "", "", fmt.Errorf("%w: preparation and terminal job authority do not match", ErrSimulationIllegal)
	}
	if err != nil {
		return SimulationTurnAuthority{}, "", "", err
	}
	preparation, err := decodeTurnAuthority(payload)
	if err != nil {
		return SimulationTurnAuthority{}, "", "", err
	}
	return preparation, requestHash, exactText, nil
}

func materializationAction(kind SimulationTurnInputKind, exactText string) (*SimulationAction, error) {
	if kind != SimulationTurnAction {
		return nil, nil
	}
	action, err := ParseSimulationAction(exactText)
	if err != nil {
		return nil, fmt.Errorf("%w: prepared action no longer parses: %v", ErrSimulationIllegal, err)
	}
	return &action, nil
}

func samePreparedTransition(expected, actual SimulationTransitionResult) bool {
	return reflect.DeepEqual(expected, actual)
}

func requirePreparedNarrativeTx(
	ctx context.Context,
	tx pgx.Tx,
	preparation SimulationTurnAuthority,
) error {
	actual := make([]SimulationResponderAuthority, len(preparation.Responders))
	for index, responder := range preparation.Responders {
		projection, authority, err := projectSimulationNarrativeTx(
			ctx, tx, preparation.WorldID, responder.CharacterID,
		)
		if err != nil {
			return err
		}
		generation, err := projectCharacterGenerationTx(
			ctx, tx, preparation.WorldID, responder.CharacterID,
		)
		if err != nil {
			return err
		}
		actual[index] = SimulationResponderAuthority{
			Position: index, CharacterID: responder.CharacterID,
			GenerationConfig: generation.Config, NarrativeProjection: projection,
			NarrativeAuthority: authority, NarrativeFingerprint: authority.Fingerprint,
		}
	}
	return requirePreparedResponderRound(preparation.Responders, actual)
}
