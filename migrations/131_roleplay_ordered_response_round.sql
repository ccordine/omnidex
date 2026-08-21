BEGIN;

LOCK TABLE roleplay_simulation_turn_preparations,
    roleplay_simulation_preparation_jobs, jobs
    IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND metadata->>'channel_mode'='roleplay'
    ) THEN
        RAISE EXCEPTION 'cannot install ordered roleplay responses while a roleplay turn is active';
    END IF;
END $$;

DROP TRIGGER roleplay_simulation_preparations_immutable
    ON roleplay_simulation_turn_preparations;
DROP TRIGGER jobs_chat_turn_binding_immutable ON jobs;

UPDATE roleplay_simulation_turn_preparations
SET result=jsonb_set(
    jsonb_set(
        result,
        '{responders}',
        jsonb_build_array(jsonb_build_object(
            'position',0,
            'character_id',result->'narrative_authority'->>'viewpoint_id',
            'generation_config',result->'generation_config',
            'narrative_projection',result->'narrative_projection',
            'narrative_authority',result->'narrative_authority',
            'narrative_fingerprint',result->'narrative_fingerprint'
        )),
        TRUE
    ),
    '{responder_routes}',
    jsonb_build_array(jsonb_build_object(
        'position',0,
        'character_id',result->'narrative_authority'->>'viewpoint_id',
        'generation_config',result->'generation_config',
        'narrative_fingerprint',result->'narrative_fingerprint'
    )),
    TRUE
);

UPDATE jobs AS job
SET metadata=jsonb_set(
    jsonb_set(
        job.metadata,
        '{roleplay_responders}',preparation.result->'responder_routes',TRUE
    ),
    '{roleplay_viewpoint_character_id}',
    to_jsonb(preparation.result->'responder_routes'->0->>'character_id'),TRUE
)
FROM roleplay_simulation_preparation_jobs AS binding
JOIN roleplay_simulation_turn_preparations AS preparation
  ON preparation.operation_id=binding.preparation_id
WHERE job.id=binding.job_id;

CREATE FUNCTION roleplay_response_round_valid(result_value JSONB)
RETURNS BOOLEAN AS $$
DECLARE
    responder JSONB;
    route JSONB;
    expected_ids JSONB;
    actual_ids JSONB;
    user_character_id TEXT;
    index_value INTEGER;
BEGIN
    IF jsonb_typeof(result_value->'responders')<>'array' OR
       jsonb_typeof(result_value->'responder_routes')<>'array' OR
       jsonb_array_length(result_value->'responders') NOT BETWEEN 1 AND 16 OR
       jsonb_array_length(result_value->'responders')<>
           jsonb_array_length(result_value->'responder_routes') THEN
        RETURN FALSE;
    END IF;
    user_character_id := result_value->'user_turn'->>'character_id';
    SELECT COALESCE(jsonb_agg(participant.value ORDER BY participant.ordinal),'[]'::jsonb)
    INTO expected_ids
    FROM jsonb_array_elements(result_value->'participant_character_ids')
         WITH ORDINALITY AS participant(value,ordinal)
    WHERE user_character_id IS NULL OR participant.value #>> '{}'<>user_character_id;
    SELECT COALESCE(
        jsonb_agg(to_jsonb(item.value->>'character_id') ORDER BY item.ordinal),
        '[]'::jsonb
    )
    INTO actual_ids
    FROM jsonb_array_elements(result_value->'responder_routes')
         WITH ORDINALITY AS item(value,ordinal);
    IF result_value->'user_turn'->>'persona_kind'='legacy_untyped' THEN
        IF jsonb_array_length(actual_ids)<>1 OR
           NOT (result_value->'participant_character_ids' ? (actual_ids->>0)) THEN
            RETURN FALSE;
        END IF;
    ELSIF expected_ids<>actual_ids THEN
        RETURN FALSE;
    END IF;
    FOR index_value IN 0..jsonb_array_length(result_value->'responders')-1 LOOP
        responder := result_value->'responders'->index_value;
        route := result_value->'responder_routes'->index_value;
        IF jsonb_typeof(responder)<>'object' OR jsonb_typeof(route)<>'object' OR
           (responder->>'position')::integer<>index_value OR
           (route->>'position')::integer<>index_value OR
           responder->>'character_id'<>route->>'character_id' OR
           responder->'generation_config'<>route->'generation_config' OR
           responder->>'narrative_fingerprint'<>route->>'narrative_fingerprint' OR
           responder->'narrative_authority'->>'viewpoint_id'<>responder->>'character_id' OR
           responder->'narrative_authority'->>'world_id'<>result_value->>'world_id' OR
           responder->'narrative_authority'->>'scene_id'<>result_value->>'scene_id' OR
           responder->'narrative_authority'->>'scene_revision'<>result_value->>'scene_revision' OR
           responder->'narrative_authority'->>'fingerprint'<>
               responder->>'narrative_fingerprint' OR
           jsonb_typeof(responder->'narrative_projection')<>'object' OR
           jsonb_typeof(responder->'generation_config')<>'object' THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    responder := result_value->'responders'->0;
    RETURN result_value->'generation_config'=responder->'generation_config' AND
           result_value->'narrative_projection'=responder->'narrative_projection' AND
           result_value->'narrative_authority'=responder->'narrative_authority' AND
           result_value->>'narrative_fingerprint'=responder->>'narrative_fingerprint';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION roleplay_lifecycle_response_round_valid(responses_value JSONB)
RETURNS BOOLEAN AS $$
DECLARE
    response JSONB;
    fact JSONB;
    index_value INTEGER := 0;
    character_id TEXT;
    seen_characters TEXT[] := ARRAY[]::TEXT[];
BEGIN
    IF jsonb_typeof(responses_value)<>'array' OR
       jsonb_array_length(responses_value)>16 THEN
        RETURN FALSE;
    END IF;
    FOR response IN SELECT item.value FROM jsonb_array_elements(responses_value) AS item(value) LOOP
        IF jsonb_typeof(response)<>'object' OR
           NOT response ?& ARRAY[
               'position','character_id','output','facts','knowledge_character_ids'
           ] OR
           response - ARRAY[
               'position','character_id','output','facts','knowledge_character_ids'
           ]<>'{}'::jsonb OR
           jsonb_typeof(response->'position')<>'number' OR
           (response->>'position')::integer<>index_value OR
           jsonb_typeof(response->'character_id')<>'string' OR
           NOT ((response->>'character_id') ~ '^rpc_[0-9a-f]{32}$') OR
           jsonb_typeof(response->'output')<>'string' OR
           octet_length(response->>'output') NOT BETWEEN 1 AND 2048 OR
           btrim(response->>'output')='' OR
           jsonb_typeof(response->'facts')<>'array' OR
           jsonb_array_length(response->'facts')>8 OR
           jsonb_typeof(response->'knowledge_character_ids')<>'array' THEN
            RETURN FALSE;
        END IF;
        character_id := response->>'character_id';
        IF character_id=ANY(seen_characters) THEN
            RETURN FALSE;
        END IF;
        seen_characters := array_append(seen_characters,character_id);
        FOR fact IN SELECT item.value FROM jsonb_array_elements(response->'facts') AS item(value) LOOP
            IF jsonb_typeof(fact)<>'string' OR
               octet_length(fact #>> '{}') NOT BETWEEN 1 AND 512 OR
               btrim(fact #>> '{}')='' THEN
                RETURN FALSE;
            END IF;
        END LOOP;
        IF jsonb_array_length(response->'facts')=0 THEN
            IF jsonb_array_length(response->'knowledge_character_ids')<>0 THEN
                RETURN FALSE;
            END IF;
        ELSIF response->'knowledge_character_ids'<>jsonb_build_array(character_id) THEN
            RETURN FALSE;
        END IF;
        index_value := index_value+1;
    END LOOP;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

ALTER TABLE job_lifecycle_operations
    DROP CONSTRAINT job_lifecycle_operations_roleplay_payload_check,
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK (
        (kind='complete_step' AND
            command_payload ?& ARRAY[
                'operation_id','step_id','output','context_key','context_value'
            ] AND
            command_payload - ARRAY[
                'operation_id','step_id','output','context_key','context_value',
                'roleplay_responses'
            ]='{}'::jsonb AND
            roleplay_lifecycle_response_round_valid(
                COALESCE(command_payload->'roleplay_responses','[]'::jsonb)
            )) OR
        (kind='fail_step' AND
            command_payload ?& ARRAY['operation_id','step_id','error'] AND
            command_payload - ARRAY['operation_id','step_id','error']='{}'::jsonb) OR
        (kind IN ('submit_feedback','replan_job') AND
            command_payload ?& ARRAY['operation_id','job_id','feedback'] AND
            command_payload - ARRAY['operation_id','job_id','feedback']='{}'::jsonb) OR
        (kind='cancel_job' AND
            command_payload ?& ARRAY['operation_id','job_id','reason'] AND
            command_payload - ARRAY['operation_id','job_id','reason']='{}'::jsonb)
    ) NOT VALID;

ALTER TABLE roleplay_simulation_turn_preparations
    DROP CONSTRAINT roleplay_simulation_turn_preparations_result_check,
    ADD CONSTRAINT roleplay_simulation_turn_preparations_result_check CHECK (
        jsonb_typeof(result)='object' AND octet_length(result::text)<=524288 AND
        result ?& ARRAY[
            'preparation_id','channel_id','user_message_id','world_id','scene_id',
            'base_scene_revision','scene_revision','active_character_id','user_turn',
            'input_kind','explicit_action','participant_character_ids','generation_config',
            'narrative_projection','narrative_authority','narrative_fingerprint',
            'responders','responder_routes','created_at'
        ] AND roleplay_response_round_valid(result)
    );

CREATE OR REPLACE FUNCTION validate_roleplay_simulation_preparation()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN ai_channels AS channel ON channel.id=world.channel_id
        JOIN ai_channel_messages AS message ON message.channel_id=channel.id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=message.id AND user_turn.channel_id=channel.id
         AND user_turn.world_id=world.id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=world.id AND scene.id=NEW.scene_id
         AND scene.revision=NEW.base_scene_revision
         AND scene.current_character_id=NEW.active_character_id
        WHERE world.id=NEW.world_id AND channel.id=NEW.channel_id
          AND channel.mode='roleplay' AND message.id=NEW.user_message_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND NEW.result->'user_turn'=user_turn.authority
          AND ((NEW.input_kind='prose' AND user_turn.contribution_kind<>'command') OR
               (NEW.input_kind<>'prose' AND user_turn.contribution_kind='command'))
    ) OR NEW.result->>'preparation_id'<>NEW.operation_id OR
       NEW.result->>'channel_id'<>NEW.channel_id OR
       (NEW.result->>'user_message_id')::bigint<>NEW.user_message_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       (NEW.result->>'base_scene_revision')::bigint<>NEW.base_scene_revision OR
       (NEW.result->>'scene_revision')::bigint<>NEW.scene_revision OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->>'input_kind'<>NEW.input_kind OR
       (NEW.result->>'explicit_action')::boolean<>NEW.explicit_action OR
       NOT roleplay_response_round_valid(NEW.result) OR
       COALESCE(NEW.result->'pending_transition'->>'operation_id','')<>
           COALESCE(NEW.pending_transition_id,'') THEN
        RAISE EXCEPTION 'simulation preparation differs from its exact user, scene, and response-round authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_roleplay_preparation_job()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
        JOIN jobs AS job ON job.id=NEW.job_id
        WHERE preparation.operation_id=NEW.preparation_id AND job.pipeline='chat'
          AND job.instruction=message.content
          AND job.metadata->>'channel_id'=preparation.channel_id
          AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
          AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
          AND job.metadata->>'roleplay_world_id'=preparation.world_id
          AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
          AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
          AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
          AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
          AND job.metadata->>'roleplay_viewpoint_character_id'=
              preparation.result->'responder_routes'->0->>'character_id'
          AND job.metadata->'roleplay_participant_character_ids'=
              preparation.result->'participant_character_ids'
          AND job.metadata->'roleplay_generation_config'=preparation.result->'generation_config'
          AND job.metadata->'roleplay_responders'=preparation.result->'responder_routes'
          AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
    ) THEN
        RAISE EXCEPTION 'simulation job differs from its exact response-round preparation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_chat_turn_binding_update()
RETURNS TRIGGER AS $$
DECLARE binding_key TEXT;
BEGIN
    IF OLD.pipeline='chat' OR NEW.pipeline='chat' THEN
        IF NEW.pipeline IS DISTINCT FROM OLD.pipeline THEN
            RAISE EXCEPTION 'chat turn pipeline authority is immutable';
        END IF;
        FOREACH binding_key IN ARRAY ARRAY[
            'channel_id','channel_user_message_id','project_id','client_cwd',
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config',
            'roleplay_generation_config','roleplay_responders','roleplay_user_turn',
            'roleplay_simulation_preparation_id','roleplay_world_id','roleplay_scene_id',
            'roleplay_scene_revision','roleplay_input_kind',
            'roleplay_participant_character_ids','roleplay_narrative_fingerprint'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable',binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_simulation_preparations_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER jobs_chat_turn_binding_immutable
BEFORE UPDATE OF pipeline,metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();

COMMIT;
