package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

// RoleplayContextSearchRepresentation identifies every term-searchable
// authority already represented by the frozen responder projection and its
// mechanically acquired recent transcript suffix.
type RoleplayContextSearchRepresentation struct {
	CanonEventIDs           []string
	MemoryIDs               []string
	ConversationSourceIDs   []string
	SimulationEventContents []string
}

// HasAdditionalConversationSearchAuthority reports whether term-directed
// assistant retrieval can add an exact completed exchange beyond the already
// acquired recent suffix.
func (r *Repository) HasAdditionalConversationSearchAuthority(
	ctx context.Context,
	job model.Job,
	representedUserMessageIDs []int64,
) (bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return false, fmt.Errorf("conversation search availability requires PostgreSQL and context")
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return false, err
	}
	if !exists || binding.Mode != model.ChannelModeAssistant {
		return false, fmt.Errorf("conversation search availability requires an assistant channel job")
	}
	seen := make(map[int64]struct{}, len(representedUserMessageIDs))
	for _, messageID := range representedUserMessageIDs {
		if messageID < 1 || messageID >= binding.UserMessageID {
			return false, fmt.Errorf("represented conversation message %d is outside the frozen suffix", messageID)
		}
		if _, duplicate := seen[messageID]; duplicate {
			return false, fmt.Errorf("represented conversation message %d is duplicated", messageID)
		}
		seen[messageID] = struct{}{}
	}
	var additional, invalid bool
	if err := r.pool.QueryRow(ctx, additionalConversationSearchAuthorityQuery,
		binding.ChannelID, binding.UserMessageID, representedUserMessageIDs,
	).Scan(&additional, &invalid); err != nil {
		return false, fmt.Errorf("inspect additional conversation search authority: %w", err)
	}
	if invalid {
		return false, fmt.Errorf("conversation search universe contains invalid completed authority")
	}
	return additional, nil
}

// HasAdditionalRoleplaySearchAuthority reports whether the exact frozen
// roleplay search universe contains at least one authority not already
// represented by fixed responder context. It compares source identity where
// one exists and uses EXCEPT ALL for repeated simulation-event text.
func (r *Repository) HasAdditionalRoleplaySearchAuthority(
	ctx context.Context,
	worldID string,
	viewpointID model.RoleplayCharacterID,
	sceneID string,
	createdBefore time.Time,
	represented RoleplayContextSearchRepresentation,
) (bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return false, fmt.Errorf("roleplay search availability requires PostgreSQL and context")
	}
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(sceneID) == "" ||
		createdBefore.IsZero() {
		return false, fmt.Errorf("roleplay search availability requires frozen world, scene, and time authority")
	}
	if err := viewpointID.Validate(); err != nil {
		return false, fmt.Errorf("roleplay search availability viewpoint: %w", err)
	}
	for label, values := range map[string][]string{
		"canon event":  represented.CanonEventIDs,
		"memory":       represented.MemoryIDs,
		"conversation": represented.ConversationSourceIDs,
	} {
		if err := validateDistinctSearchRepresentation(label, values); err != nil {
			return false, err
		}
	}
	for index, content := range represented.SimulationEventContents {
		if strings.TrimSpace(content) == "" {
			return false, fmt.Errorf("represented simulation event %d is blank", index)
		}
	}
	var additional, invalidConversation bool
	if err := r.pool.QueryRow(ctx, additionalRoleplaySearchAuthorityQuery,
		worldID, viewpointID, sceneID, createdBefore,
		represented.CanonEventIDs, represented.MemoryIDs,
		represented.ConversationSourceIDs, represented.SimulationEventContents,
		roleplay.MaxSceneParticipants,
	).Scan(&additional, &invalidConversation); err != nil {
		return false, fmt.Errorf("inspect additional roleplay search authority: %w", err)
	}
	if invalidConversation {
		return false, fmt.Errorf("roleplay search universe contains invalid completed round authority")
	}
	return additional, nil
}

func validateDistinctSearchRepresentation(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("represented %s %d is blank", label, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("represented %s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

const additionalConversationSearchAuthorityQuery = `
	WITH completed AS (
		SELECT message.id AS user_message_id,message.content AS user_content,
		       MIN(job.instruction) AS instruction,MIN(job.result) AS result,
		       BOOL_AND(job.result IS NOT NULL) AS result_present,
		       COUNT(*) AS completion_count
		FROM ai_channel_messages AS message
		JOIN jobs AS job
		  ON job.pipeline='chat' AND job.status='completed'
		 AND job.metadata->>'channel_id'=$1
		 AND job.metadata->>'channel_user_message_id'=message.id::text
		WHERE message.channel_id=$1 AND message.role='user' AND message.id<$2
		GROUP BY message.id,message.content
	), exact_exchanges AS (
		SELECT completed.user_message_id
		FROM completed
		LEFT JOIN LATERAL (
			SELECT message.role,message.content
			FROM ai_channel_messages AS message
			WHERE message.channel_id=$1 AND message.id>completed.user_message_id
			ORDER BY message.id ASC LIMIT 1
		) AS assistant ON TRUE
		WHERE completed.completion_count=1
		  AND completed.result_present
		  AND completed.instruction=completed.user_content
		  AND assistant.role='assistant' AND assistant.content=completed.result
	), invalid_exchanges AS (
		SELECT completed.user_message_id
		FROM completed
		LEFT JOIN LATERAL (
			SELECT message.role,message.content
			FROM ai_channel_messages AS message
			WHERE message.channel_id=$1 AND message.id>completed.user_message_id
			ORDER BY message.id ASC LIMIT 1
		) AS assistant ON TRUE
		WHERE completed.completion_count<>1
		   OR NOT completed.result_present
		   OR completed.instruction IS DISTINCT FROM completed.user_content
		   OR assistant.role IS DISTINCT FROM 'assistant'
		   OR assistant.content IS DISTINCT FROM completed.result
	)
	SELECT EXISTS(
		SELECT user_message_id FROM exact_exchanges
		EXCEPT
		SELECT represented FROM unnest($3::bigint[]) AS represented
	), EXISTS(SELECT 1 FROM invalid_exchanges)
`

const additionalRoleplaySearchAuthorityQuery = `
	WITH conversation_base AS (
		SELECT preparation.operation_id AS preparation_id,
		       preparation.channel_id,preparation.world_id,
		       preparation.user_message_id,job.id AS job_id,
		       job.instruction,job.result,message.content AS user_content
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS preparation_job
		  ON preparation_job.preparation_id=preparation.operation_id
		JOIN jobs AS job
		  ON job.id=preparation_job.job_id
		 AND job.pipeline='chat' AND job.status='completed'
		JOIN roleplay_user_turns AS user_turn
		  ON user_turn.user_message_id=preparation.user_message_id
		 AND user_turn.channel_id=preparation.channel_id
		JOIN ai_channel_messages AS message
		  ON message.id=user_turn.user_message_id
		 AND message.channel_id=user_turn.channel_id AND message.role='user'
		WHERE preparation.world_id=$1 AND preparation.scene_id=$3
		  AND preparation.result->'participant_character_ids' ? $2
		  AND preparation.created_at<=$4
	), response_rows AS (
		SELECT base.job_id,completion.response_position,'fictional'::text AS kind,
		       message.id AS message_id,message.content
		FROM conversation_base AS base
		JOIN job_lifecycle_operations AS operation
		  ON operation.job_id=base.job_id AND operation.kind='complete_step'
		 AND operation.result_job_status='completed'
		 AND operation.result_step_status='completed'
		JOIN roleplay_turn_completions AS completion
		  ON completion.operation_id=operation.operation_id
		 AND completion.world_id=base.world_id
		JOIN roleplay_characters AS character
		  ON character.world_id=completion.world_id
		 AND character.id=completion.viewpoint_character_id
		JOIN ai_channel_messages AS message
		  ON message.id=completion.source_message_id
		 AND message.channel_id=base.channel_id AND message.role='assistant'
		UNION ALL
		SELECT base.job_id,0,'research'::text,message.id,message.content
		FROM conversation_base AS base
		JOIN roleplay_research_completions AS completion
		  ON completion.preparation_id=base.preparation_id
		 AND completion.job_id=base.job_id
		JOIN job_lifecycle_operations AS operation
		  ON operation.operation_id=completion.operation_id
		 AND operation.job_id=base.job_id AND operation.kind='complete_step'
		 AND operation.result_job_status='completed'
		 AND operation.result_step_status='completed'
		JOIN roleplay_research_turns AS research
		  ON research.preparation_id=completion.preparation_id
		 AND research.world_id=base.world_id
		JOIN roleplay_characters AS character
		  ON character.world_id=research.world_id AND character.id=research.character_id
		JOIN ai_channel_messages AS message
		  ON message.id=completion.source_message_id
		 AND message.channel_id=base.channel_id AND message.role='assistant'
	), conversation_rounds AS (
		SELECT base.user_message_id,base.job_id,base.instruction,base.result,
		       base.user_content,COUNT(response.message_id) AS response_count,
		       COUNT(DISTINCT response.kind) AS kind_count,
		       MIN(response.kind) AS minimum_kind,
		       MIN(response.response_position) AS minimum_position,
		       MAX(response.response_position) AS maximum_position,
		       COUNT(DISTINCT response.response_position) AS position_count,
		       COUNT(DISTINCT response.message_id) AS message_count,
		       MIN(response.message_id) AS first_message_id,
		       MAX(response.message_id) AS last_message_id,
		       string_agg(response.content,E'\n\n'
		           ORDER BY response.response_position,response.message_id) AS response_content
		FROM conversation_base AS base
		LEFT JOIN response_rows AS response ON response.job_id=base.job_id
		GROUP BY base.user_message_id,base.job_id,base.instruction,base.result,base.user_content
	), conversation_candidates AS (
		SELECT 'channel-message-' || base.user_message_id::text || '-through-' ||
		       base.last_message_id::text AS source_id
		FROM conversation_rounds AS base
		WHERE base.result IS NOT NULL
		  AND base.instruction=base.user_content
		  AND base.response_count BETWEEN 1 AND $9
		  AND base.kind_count=1
		  AND base.minimum_position=0
		  AND base.maximum_position=base.response_count-1
		  AND base.position_count=base.response_count
		  AND base.message_count=base.response_count
		  AND base.first_message_id>base.user_message_id
		  AND (base.minimum_kind<>'research' OR base.response_count=1)
		  AND base.response_content=base.result
	), invalid_conversation AS (
		SELECT base.job_id
		FROM conversation_rounds AS base
		WHERE NOT COALESCE(
			base.result IS NOT NULL
			AND base.instruction=base.user_content
			AND base.response_count BETWEEN 1 AND $9
			AND base.kind_count=1
			AND base.minimum_position=0
			AND base.maximum_position=base.response_count-1
			AND base.position_count=base.response_count
			AND base.message_count=base.response_count
			AND base.first_message_id>base.user_message_id
			AND (base.minimum_kind<>'research' OR base.response_count=1)
			AND base.response_content=base.result,
			FALSE
		)
	), additional_canon AS (
		SELECT event.id
		FROM roleplay_character_knowledge AS knowledge
		JOIN roleplay_canon_events AS event
		  ON event.world_id=knowledge.world_id AND event.id=knowledge.canon_event_id
		WHERE knowledge.world_id=$1 AND knowledge.character_id=$2
		  AND event.created_at<=$4
		EXCEPT
		SELECT represented FROM unnest($5::text[]) AS represented
	), additional_memory AS (
		SELECT memory.id
		FROM roleplay_characters AS viewpoint
		JOIN roleplay_characters AS placement
		  ON placement.library_character_id=viewpoint.library_character_id
		JOIN roleplay_character_memories AS memory
		  ON memory.character_id=placement.id AND memory.world_id=placement.world_id
		WHERE viewpoint.world_id=$1 AND viewpoint.id=$2 AND memory.created_at<=$4
		EXCEPT
		SELECT represented FROM unnest($6::text[]) AS represented
	), additional_conversation AS (
		SELECT source_id FROM conversation_candidates
		EXCEPT
		SELECT represented FROM unnest($7::text[]) AS represented
	), additional_event AS (
		SELECT event.content
		FROM roleplay_simulation_transitions AS transition
		CROSS JOIN LATERAL jsonb_array_elements_text(
			transition.result->'narrative_events'
		) AS event(content)
		WHERE transition.world_id=$1 AND transition.scene_id=$3
		  AND transition.observer_character_ids ? $2
		  AND transition.created_at<=$4
		EXCEPT ALL
		SELECT represented FROM unnest($8::text[]) AS represented
	)
	SELECT EXISTS(SELECT 1 FROM additional_canon)
	    OR EXISTS(SELECT 1 FROM additional_memory)
	    OR EXISTS(SELECT 1 FROM additional_conversation)
	    OR EXISTS(SELECT 1 FROM additional_event),
	    EXISTS(SELECT 1 FROM invalid_conversation)
`
