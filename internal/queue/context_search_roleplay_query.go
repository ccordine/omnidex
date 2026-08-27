package queue

const roleplayContextSearchQuery = `
	WITH query_terms AS (
		SELECT websearch_to_tsquery('simple',term) AS query
		FROM unnest($5::text[]) AS term
	), conversation_base AS (
		SELECT preparation.operation_id AS preparation_id,
		       preparation.channel_id,preparation.world_id,
		       preparation.user_message_id,job.id AS job_id,
		       job.instruction,COALESCE(job.result,'') AS job_result,
		       job.result IS NOT NULL AS result_present,
		       message.content AS user_content,
		       turn_authority.authority AS user_turn_authority
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS preparation_job
		  ON preparation_job.preparation_id=preparation.operation_id
		JOIN jobs AS job
		  ON job.id=preparation_job.job_id
		 AND job.pipeline='chat' AND job.status='completed'
		JOIN roleplay_user_turns AS turn_authority
		  ON turn_authority.user_message_id=preparation.user_message_id
		 AND turn_authority.channel_id=preparation.channel_id
		JOIN ai_channel_messages AS message
		  ON message.id=turn_authority.user_message_id
		 AND message.channel_id=turn_authority.channel_id
		 AND message.role='user'
		WHERE preparation.world_id=$1 AND preparation.scene_id=$3
		  AND preparation.result->'participant_character_ids' ? $2
		  AND preparation.created_at<=$4
	), conversation_response_rows AS (
		SELECT base.preparation_id,completion.response_position,
		       'fictional'::text AS completion_kind,
		       message.id AS message_id,character.name AS speaker_name,
		       message.content
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
		SELECT base.preparation_id,0,'research'::text,
		       message.id,character.name,message.content
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
		  ON character.world_id=research.world_id
		 AND character.id=research.character_id
		JOIN ai_channel_messages AS message
		  ON message.id=completion.source_message_id
		 AND message.channel_id=base.channel_id AND message.role='assistant'
	), conversation_rounds AS (
		SELECT base.preparation_id,base.job_id,base.instruction,
		       base.job_result,base.result_present,base.user_message_id,
		       base.user_content,base.user_turn_authority,
		       COALESCE(MAX(response.message_id),0) AS last_response_message_id,
		       COALESCE(jsonb_agg(jsonb_build_object(
		           'position',response.response_position,
		           'completion_kind',response.completion_kind,
		           'message_id',response.message_id,
		           'speaker_name',response.speaker_name,
		           'content',response.content
		       ) ORDER BY response.response_position,response.message_id)
		       FILTER (WHERE response.message_id IS NOT NULL),'[]'::jsonb) AS responses,
		       COALESCE(string_agg(
		           response.speaker_name || E' response:\n' || response.content,
		           E'\n' ORDER BY response.response_position,response.message_id
		       ),'') AS response_context
		FROM conversation_base AS base
		LEFT JOIN conversation_response_rows AS response
		  ON response.preparation_id=base.preparation_id
		GROUP BY base.preparation_id,base.job_id,base.instruction,
		         base.job_result,base.result_present,base.user_message_id,
		         base.user_content,base.user_turn_authority
	), conversation_candidates AS (
		SELECT 'conversation_exchange'::text AS namespace,
		       'channel-message-' || user_message_id::text || '-through-' ||
		           last_response_message_id::text AS source_id,
		       CASE user_turn_authority->>'contribution_kind'
		           WHEN 'command' THEN response_context
		           ELSE user_turn_authority->>'persona_name' || ' [' ||
		               (user_turn_authority->>'contribution_kind') ||
		               E'] contribution:\n' || user_content ||
		               CASE WHEN response_context='' THEN ''
		                    ELSE E'\n' || response_context END
		       END AS content,
		       3 AS source_priority,user_message_id AS source_ordinal,
		       jsonb_build_object(
		           'job_id',job_id,'instruction',instruction,
		           'job_result',job_result,'result_present',result_present,
		           'user_message_id',user_message_id,'user_content',user_content,
		           'user_turn',user_turn_authority,'responses',responses
		       ) AS conversation_authority
		FROM conversation_rounds
	), candidates AS (
		SELECT 'fictional_canon'::text AS namespace,event.id AS source_id,
		       event.content,1 AS source_priority,event.ordinal AS source_ordinal,
		       NULL::jsonb AS conversation_authority
		FROM roleplay_character_knowledge AS knowledge
		JOIN roleplay_canon_events AS event
		  ON event.world_id=knowledge.world_id AND event.id=knowledge.canon_event_id
		WHERE knowledge.world_id=$1 AND knowledge.character_id=$2
		  AND event.created_at<=$4
		UNION ALL
		SELECT 'character_memory',memory.id,memory.content,2,memory.ordinal,NULL::jsonb
		FROM roleplay_characters AS viewpoint
		JOIN roleplay_characters AS placement
		  ON placement.library_character_id=viewpoint.library_character_id
		JOIN roleplay_character_memories AS memory
		  ON memory.character_id=placement.id AND memory.world_id=placement.world_id
		WHERE viewpoint.world_id=$1 AND viewpoint.id=$2
		  AND memory.created_at<=$4
		UNION ALL
		SELECT namespace,source_id,content,source_priority,source_ordinal,
		       conversation_authority
		FROM conversation_candidates
		UNION ALL
		SELECT 'simulation_event',transition.operation_id || '-' ||
		       event.ordinality::text,event.content,4,transition.ordinal,NULL::jsonb
		FROM roleplay_simulation_transitions AS transition
		CROSS JOIN LATERAL jsonb_array_elements_text(
			transition.result->'narrative_events'
		) WITH ORDINALITY AS event(content,ordinality)
		WHERE transition.world_id=$1 AND transition.scene_id=$3
		  AND transition.observer_character_ids ? $2
		  AND transition.created_at<=$4
	), ranked AS (
		SELECT candidates.namespace,candidates.source_id,candidates.content,
		       candidates.source_priority,candidates.source_ordinal,
		       candidates.conversation_authority,
		       MAX(ts_rank_cd(
		           to_tsvector('simple',candidates.content),query_terms.query
		       )) AS rank
		FROM candidates
		JOIN query_terms
		  ON to_tsvector('simple',candidates.content) @@ query_terms.query
		GROUP BY candidates.namespace,candidates.source_id,candidates.content,
		         candidates.source_priority,candidates.source_ordinal,
		         candidates.conversation_authority
	)
	SELECT namespace,source_id,content,conversation_authority
	FROM ranked
	ORDER BY rank DESC,source_priority ASC,source_ordinal DESC,source_id ASC
	LIMIT $6
`
