package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) LoadSimulationTurnForJob(
	ctx context.Context,
	preparationID string,
	jobID int64,
) (SimulationTurnAuthority, error) {
	if err := s.validateContext(ctx); err != nil {
		return SimulationTurnAuthority{}, err
	}
	if err := validateIdentity(preparationID, transitionIdentity); err != nil {
		return SimulationTurnAuthority{}, err
	}
	if jobID < 1 {
		return SimulationTurnAuthority{}, fmt.Errorf("simulation turn load requires a positive job ID")
	}
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		SELECT preparation.result
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		JOIN jobs AS job ON job.id=binding.job_id
		JOIN ai_channel_messages AS message
		  ON message.id=preparation.user_message_id AND message.channel_id=preparation.channel_id
		JOIN ai_channels AS channel ON channel.id=preparation.channel_id AND channel.mode='roleplay'
		JOIN roleplay_worlds AS world
		  ON world.id=preparation.world_id AND world.channel_id=channel.id
		WHERE preparation.operation_id=$1 AND binding.job_id=$2
		  AND message.role='user' AND job.pipeline='chat' AND job.instruction=message.content
		  AND job.metadata->>'channel_id'=preparation.channel_id
		  AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
		  AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
		  AND job.metadata->>'roleplay_world_id'=preparation.world_id
		  AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
		  AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
		  AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
		  AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
		  AND job.metadata->>'roleplay_viewpoint_character_id'=preparation.active_character_id
		  AND job.metadata->'roleplay_participant_character_ids'=preparation.result->'participant_character_ids'
	`, preparationID, jobID).Scan(&payload)
	if err == pgx.ErrNoRows {
		return SimulationTurnAuthority{}, fmt.Errorf("%w: simulation preparation and job authority do not match", ErrSimulationIllegal)
	}
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	var authority SimulationTurnAuthority
	if err := json.Unmarshal(payload, &authority); err != nil {
		return SimulationTurnAuthority{}, fmt.Errorf("decode simulation turn authority: %w", err)
	}
	if err := authority.Validate(); err != nil {
		return SimulationTurnAuthority{}, fmt.Errorf("persisted simulation turn authority is invalid: %w", err)
	}
	return authority, nil
}
