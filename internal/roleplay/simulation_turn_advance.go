package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

func AdvanceTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnAdvanceRequest,
) (SimulationTurnAdvanceResult, error) {
	if ctx == nil || tx == nil {
		return SimulationTurnAdvanceResult{}, fmt.Errorf("simulation turn advance requires transaction authority")
	}
	if err := validateTurnAdvanceRequest(request); err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	requestHash, err := simulationRequestHash("turn-advance.v1", request)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	if result, found, err := loadTurnAdvanceTx(ctx, tx, request.OperationID, requestHash); err != nil || found {
		return result, err
	}
	preparation, err := loadBoundPreparationTx(ctx, tx, request)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	locked, err := lockSimulationSceneTx(ctx, tx, preparation.WorldID, preparation.SceneID)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	if result, found, err := loadTurnAdvanceTx(ctx, tx, request.OperationID, requestHash); err != nil || found {
		return result, err
	}
	if request.ExpectedRevision != preparation.SceneRevision || locked.Sheet.Revision != preparation.SceneRevision ||
		locked.Sheet.ActiveCharacterID != preparation.ActiveCharacterID ||
		!slices.Equal(simulationParticipantIDs(locked.Participants), preparation.ParticipantCharacterIDs) {
		return SimulationTurnAdvanceResult{}, fmt.Errorf("%w: prepared turn authority changed", ErrSimulationStaleRevision)
	}
	nextCharacterID, err := nextSceneCharacter(locked)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	afterRevision, err := updateSceneRevisionTx(
		ctx, tx, locked.Sheet.ID, locked.Sheet.Revision, nextCharacterID,
	)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	fingerprint, err := simulationNarrativeFingerprintTx(ctx, tx, preparation.WorldID, nextCharacterID)
	if err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	result := SimulationTurnAdvanceResult{
		OperationID: request.OperationID, PreparationID: request.PreparationID,
		WorldID: preparation.WorldID, SceneID: preparation.SceneID,
		PreviousCharacterID: preparation.ActiveCharacterID, ActiveCharacterID: nextCharacterID,
		BeforeRevision: locked.Sheet.Revision, AfterRevision: afterRevision,
		ParticipantCharacterIDs: append([]string(nil), preparation.ParticipantCharacterIDs...),
		NarrativeFingerprint:    fingerprint, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := persistTurnAdvanceTx(ctx, tx, requestHash, request, result); err != nil {
		return SimulationTurnAdvanceResult{}, err
	}
	return result, nil
}

func loadBoundPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnAdvanceRequest,
) (SimulationTurnAuthority, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT preparation.result
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		JOIN jobs AS job ON job.id=binding.job_id
		WHERE preparation.operation_id=$1 AND preparation.channel_id=$2
		  AND preparation.user_message_id=$3 AND binding.job_id=$4
		  AND job.pipeline='chat' AND job.metadata->>'channel_id'=$2
		  AND job.metadata->>'channel_user_message_id'=$3::text
		  AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
		  AND job.metadata->>'roleplay_world_id'=preparation.world_id
		  AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
		  AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
		  AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
		  AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
		  AND job.metadata->>'roleplay_viewpoint_character_id'=preparation.active_character_id
		  AND job.metadata->'roleplay_participant_character_ids'=preparation.result->'participant_character_ids'
	`, request.PreparationID, request.ChannelID, request.UserMessageID, request.JobID).Scan(&payload)
	if err == pgx.ErrNoRows {
		return SimulationTurnAuthority{}, fmt.Errorf("%w: preparation, message, and job authority do not match", ErrSimulationIllegal)
	}
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	return decodeTurnAuthority(payload)
}

func validateTurnAdvanceRequest(request SimulationTurnAdvanceRequest) error {
	if err := validateIdentity(request.OperationID, transitionIdentity); err != nil {
		return err
	}
	if err := validateIdentity(request.PreparationID, transitionIdentity); err != nil {
		return err
	}
	if request.OperationID == request.PreparationID {
		return fmt.Errorf("turn advance requires an identity distinct from preparation")
	}
	if err := validateChannelID(request.ChannelID); err != nil {
		return err
	}
	if request.UserMessageID < 1 || request.JobID < 1 || request.ExpectedRevision < 1 {
		return fmt.Errorf("turn advance requires positive message, job, and revision authority")
	}
	return nil
}

func persistTurnAdvanceTx(
	ctx context.Context,
	tx pgx.Tx,
	requestHash string,
	request SimulationTurnAdvanceRequest,
	result SimulationTurnAdvanceResult,
) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	participants, err := json.Marshal(result.ParticipantCharacterIDs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_turn_advances (
			operation_id,preparation_id,job_id,world_id,scene_id,
			before_revision,after_revision,previous_character_id,active_character_id,
			participant_character_ids,narrative_fingerprint,request_sha256,result,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13::jsonb,$14)
	`, request.OperationID, request.PreparationID, request.JobID,
		result.WorldID, result.SceneID, result.BeforeRevision, result.AfterRevision,
		result.PreviousCharacterID, result.ActiveCharacterID, string(participants),
		result.NarrativeFingerprint, requestHash, string(payload), result.CreatedAt)
	return simulationDefinitionError("simulation turn advance", err)
}
