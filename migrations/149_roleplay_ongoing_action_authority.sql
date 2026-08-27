BEGIN;

LOCK TABLE roleplay_turn_completions, roleplay_characters,
    roleplay_simulation_turn_preparations, roleplay_simulation_turn_advances,
    roleplay_simulation_preparation_jobs, roleplay_user_turns,
    roleplay_scene_participants,
    roleplay_simulation_transitions, roleplay_research_completions,
    ai_channel_messages, job_lifecycle_operations, jobs,
    station_gap_openings
    IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND metadata->>'channel_mode'='roleplay'
    ) THEN
        RAISE EXCEPTION 'cannot install ongoing-action authority while a roleplay turn is active';
    END IF;
END $$;

CREATE TABLE roleplay_ongoing_action_states (
    id TEXT PRIMARY KEY CHECK (id ~ '^rpo_[0-9a-f]{32}$'),
    ordinal BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    source_completion_operation_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('response','user_action')),
    source_position INTEGER NOT NULL CHECK (
        (source_kind='response' AND source_position BETWEEN 0 AND 15) OR
        (source_kind='user_action' AND source_position=-1)
    ),
    source_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    action_text TEXT,
    authority_namespace TEXT NOT NULL DEFAULT 'SIMULATION_STATE' CHECK (
        authority_namespace='SIMULATION_STATE'
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_ongoing_action_states_character_fkey
        FOREIGN KEY (world_id,character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_ongoing_action_states_operation_fkey
        FOREIGN KEY (source_completion_operation_id)
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT roleplay_ongoing_action_states_completion_unique
        UNIQUE (source_completion_operation_id,source_position),
    CONSTRAINT roleplay_ongoing_action_states_text_check CHECK (
        action_text IS NULL OR (
            octet_length(action_text) BETWEEN 1 AND 512 AND
            action_text=btrim(action_text)
        )
    )
);

CREATE INDEX roleplay_ongoing_action_states_current_idx
    ON roleplay_ongoing_action_states (
        world_id,character_id,ordinal DESC,id DESC
    );

CREATE TABLE roleplay_ongoing_action_resolutions (
    completion_operation_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('response','user_action')),
    source_position INTEGER NOT NULL CHECK (
        (source_kind='response' AND source_position BETWEEN 0 AND 15) OR
        (source_kind='user_action' AND source_position=-1)
    ),
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    source_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    previous_state_id TEXT REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT,
    current_state_id TEXT REFERENCES roleplay_ongoing_action_states(id) ON DELETE RESTRICT,
    previous_action_text TEXT,
    action_text TEXT,
    changed BOOLEAN NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'SIMULATION_STATE' CHECK (
        authority_namespace='SIMULATION_STATE'
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (completion_operation_id,source_position),
    CONSTRAINT roleplay_ongoing_action_resolutions_operation_fkey
        FOREIGN KEY (completion_operation_id)
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT roleplay_ongoing_action_resolutions_character_fkey
        FOREIGN KEY (world_id,character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_ongoing_action_resolutions_state_identity_check CHECK (
        (previous_state_id IS NULL OR previous_state_id ~ '^rpo_[0-9a-f]{32}$') AND
        (current_state_id IS NULL OR current_state_id ~ '^rpo_[0-9a-f]{32}$')
    ),
    CONSTRAINT roleplay_ongoing_action_resolutions_previous_text_check CHECK (
        previous_action_text IS NULL OR (
            octet_length(previous_action_text) BETWEEN 1 AND 512 AND
            previous_action_text=btrim(previous_action_text)
        )
    ),
    CONSTRAINT roleplay_ongoing_action_resolutions_text_check CHECK (
        action_text IS NULL OR (
            octet_length(action_text) BETWEEN 1 AND 512 AND
            action_text=btrim(action_text)
        )
    ),
    CONSTRAINT roleplay_ongoing_action_resolutions_delta_check CHECK (
        changed=(previous_action_text IS DISTINCT FROM action_text) AND
        ((changed AND current_state_id IS NOT NULL) OR
         (NOT changed AND previous_state_id IS NOT DISTINCT FROM current_state_id))
    )
);

CREATE FUNCTION roleplay_user_action_source_valid(
    target_world_id TEXT,
    target_character_id TEXT,
    target_user_message_id BIGINT
)
RETURNS BOOLEAN AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN ai_channel_messages AS message
          ON message.id=user_turn.user_message_id
         AND message.channel_id=user_turn.channel_id
        WHERE preparation.world_id=target_world_id
          AND user_turn.user_message_id=target_user_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=target_character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND preparation.result->'participant_character_ids' ? target_character_id
          AND message.role='user' AND message.content=user_turn.exact_text
          AND EXISTS (
              SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
              WHERE part.value->>'kind'='action'
          )
    ) AS exact_user_action_source;
$$ LANGUAGE SQL STABLE;

CREATE FUNCTION roleplay_user_ongoing_action_payload_valid(payload_value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN jsonb_typeof(payload_value)='object' AND
           payload_value ?& ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ] AND
           payload_value - ARRAY[
               'character_id','previous_ongoing_action','ongoing_action'
           ]='{}'::jsonb AND
           jsonb_typeof(payload_value->'character_id')='string' AND
           (payload_value->>'character_id') ~ '^rpc_[0-9a-f]{32}$' AND
           COALESCE(jsonb_typeof(payload_value->'previous_ongoing_action'),'missing')
               IN ('string','null') AND
           COALESCE(jsonb_typeof(payload_value->'ongoing_action'),'missing')
               IN ('string','null') AND
           (
               jsonb_typeof(payload_value->'previous_ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'previous_ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'previous_ongoing_action'=
                       btrim(payload_value->>'previous_ongoing_action')
               )
           ) AND
           (
               jsonb_typeof(payload_value->'ongoing_action')<>'string' OR
               (
                   octet_length(payload_value->>'ongoing_action') BETWEEN 1 AND 512 AND
                   payload_value->>'ongoing_action'=btrim(payload_value->>'ongoing_action')
               )
           );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION validate_roleplay_ongoing_action_state_insert()
RETURNS TRIGGER AS $$
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action state requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.source_completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action state differs from its exact user-action preparation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION validate_roleplay_ongoing_action_resolution_insert()
RETURNS TRIGGER AS $$
DECLARE
    latest_state_id TEXT;
    latest_action_text TEXT;
    current_ordinal BIGINT;
    prior_state_id TEXT;
    prior_action_text TEXT;
BEGIN
	PERFORM 1
	FROM roleplay_characters AS character
	WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id
	FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'ongoing-action resolution requires one exact character serialization authority';
	END IF;
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS completion
        WHERE completion.operation_id=NEW.completion_operation_id
          AND completion.response_position=NEW.source_position
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact response completion';
    ELSIF NEW.source_kind='user_action' AND NOT roleplay_user_action_source_valid(
        NEW.world_id,NEW.character_id,NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'ongoing-action resolution differs from its exact user-action preparation';
    END IF;

    SELECT state.id,state.action_text
      INTO latest_state_id,latest_action_text
    FROM roleplay_ongoing_action_states AS state
    WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
    ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;

    IF NEW.changed THEN
        SELECT state.ordinal INTO current_ordinal
        FROM roleplay_ongoing_action_states AS state
        WHERE state.id=NEW.current_state_id
          AND state.world_id=NEW.world_id
          AND state.character_id=NEW.character_id
          AND state.source_completion_operation_id=NEW.completion_operation_id
          AND state.source_kind=NEW.source_kind
          AND state.source_position=NEW.source_position
          AND state.source_message_id=NEW.source_message_id
          AND state.action_text IS NOT DISTINCT FROM NEW.action_text;
        IF current_ordinal IS NULL OR latest_state_id IS DISTINCT FROM NEW.current_state_id OR
           latest_action_text IS DISTINCT FROM NEW.action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution lacks its exact current state';
        END IF;
        SELECT state.id,state.action_text
          INTO prior_state_id,prior_action_text
        FROM roleplay_ongoing_action_states AS state
        WHERE state.world_id=NEW.world_id AND state.character_id=NEW.character_id
          AND state.ordinal<current_ordinal
        ORDER BY state.ordinal DESC,state.id DESC LIMIT 1;
        IF prior_state_id IS DISTINCT FROM NEW.previous_state_id OR
           prior_action_text IS DISTINCT FROM NEW.previous_action_text THEN
            RAISE EXCEPTION 'changed ongoing-action resolution differs from its exact prior state';
        END IF;
    ELSIF latest_state_id IS DISTINCT FROM NEW.current_state_id OR
          latest_state_id IS DISTINCT FROM NEW.previous_state_id OR
          latest_action_text IS DISTINCT FROM NEW.action_text OR
          latest_action_text IS DISTINCT FROM NEW.previous_action_text THEN
        RAISE EXCEPTION 'unchanged ongoing-action resolution differs from exact current state';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION require_roleplay_ongoing_action_state_resolution()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
        WHERE resolution.completion_operation_id=NEW.source_completion_operation_id
          AND resolution.source_kind=NEW.source_kind
          AND resolution.source_position=NEW.source_position
          AND resolution.world_id=NEW.world_id
          AND resolution.character_id=NEW.character_id
          AND resolution.source_message_id=NEW.source_message_id
          AND resolution.current_state_id=NEW.id
          AND resolution.action_text IS NOT DISTINCT FROM NEW.action_text
          AND resolution.changed
    ) THEN
        RAISE EXCEPTION 'ongoing-action state lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION require_roleplay_ongoing_action_lifecycle_source()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_kind='response' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_turn_completions AS completion
          ON completion.operation_id=operation.operation_id
         AND completion.response_position=NEW.source_position
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND completion.world_id=NEW.world_id
          AND completion.viewpoint_character_id=NEW.character_id
          AND completion.source_message_id=NEW.source_message_id
          AND jsonb_typeof(operation.command_payload->'roleplay_responses')='array'
          AND jsonb_array_length(operation.command_payload->'roleplay_responses')>
              NEW.source_position
          AND operation.command_payload->'roleplay_responses'->NEW.source_position->>'character_id'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'previous_ongoing_action','null'::jsonb
          )
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=COALESCE(
              operation.command_payload->'roleplay_responses'->NEW.source_position->
                  'ongoing_action','null'::jsonb
          )
    ) THEN
        RAISE EXCEPTION 'ongoing-action response resolution lacks exact lifecycle payload authority';
    ELSIF NEW.source_kind='user_action' AND NOT EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.job_id=operation.job_id
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=binding.preparation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        WHERE operation.operation_id=NEW.completion_operation_id
          AND operation.kind='complete_step'
          AND operation.result_job_status='completed'
          AND operation.result_step_status='completed'
          AND preparation.world_id=NEW.world_id
          AND user_turn.user_message_id=NEW.source_message_id
          AND user_turn.persona_kind='character'
          AND user_turn.persona_character_id=NEW.character_id
          AND preparation.result->'user_turn'=user_turn.authority
          AND roleplay_user_ongoing_action_payload_valid(
              operation.command_payload->'roleplay_user_ongoing_action'
          )
          AND operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
              NEW.character_id
          AND COALESCE(to_jsonb(NEW.previous_action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
          AND COALESCE(to_jsonb(NEW.action_text),'null'::jsonb)=
              operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
    ) THEN
        RAISE EXCEPTION 'ongoing-action user resolution lacks exact lifecycle payload authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_ongoing_action_states_validate_insert
BEFORE INSERT ON roleplay_ongoing_action_states
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_state_insert();
CREATE TRIGGER roleplay_ongoing_action_resolutions_validate_insert
BEFORE INSERT ON roleplay_ongoing_action_resolutions
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_ongoing_action_resolution_insert();
CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_states_require_resolution
AFTER INSERT ON roleplay_ongoing_action_states
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_state_resolution();

CREATE TRIGGER roleplay_ongoing_action_states_immutable
BEFORE UPDATE OR DELETE ON roleplay_ongoing_action_states
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_ongoing_action_states_truncate_immutable
BEFORE TRUNCATE ON roleplay_ongoing_action_states
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_ongoing_action_resolutions_immutable
BEFORE UPDATE OR DELETE ON roleplay_ongoing_action_resolutions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_ongoing_action_resolutions_truncate_immutable
BEFORE TRUNCATE ON roleplay_ongoing_action_resolutions
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

INSERT INTO roleplay_ongoing_action_resolutions (
    completion_operation_id,source_kind,source_position,world_id,character_id,
    source_message_id,previous_state_id,current_state_id,
    previous_action_text,action_text,changed
)
SELECT completion.operation_id,'response',completion.response_position,completion.world_id,
       completion.viewpoint_character_id,completion.source_message_id,
       NULL,NULL,NULL,NULL,FALSE
FROM roleplay_turn_completions AS completion
ORDER BY completion.operation_id,completion.response_position;

INSERT INTO roleplay_ongoing_action_resolutions (
    completion_operation_id,source_kind,source_position,world_id,character_id,
    source_message_id,previous_state_id,current_state_id,
    previous_action_text,action_text,changed
)
SELECT operation.operation_id,'user_action',-1,preparation.world_id,
       user_turn.persona_character_id,user_turn.user_message_id,
       NULL,NULL,NULL,NULL,FALSE
FROM roleplay_simulation_turn_preparations AS preparation
JOIN roleplay_simulation_preparation_jobs AS binding
  ON binding.preparation_id=preparation.operation_id
JOIN job_lifecycle_operations AS operation
  ON operation.job_id=binding.job_id
 AND operation.kind='complete_step'
 AND operation.result_job_status='completed'
 AND operation.result_step_status='completed'
 AND operation.command_payload->>'context_key'='objective_result'
JOIN roleplay_user_turns AS user_turn
  ON user_turn.user_message_id=preparation.user_message_id
 AND user_turn.channel_id=preparation.channel_id
 AND user_turn.world_id=preparation.world_id
WHERE user_turn.persona_kind='character'
  AND preparation.result->'user_turn'=user_turn.authority
  AND preparation.result->'participant_character_ids' ? user_turn.persona_character_id
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
      WHERE part.value->>'kind'='action'
  )
ORDER BY operation.operation_id;

CREATE CONSTRAINT TRIGGER roleplay_ongoing_action_resolutions_require_lifecycle_source
AFTER INSERT ON roleplay_ongoing_action_resolutions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_ongoing_action_lifecycle_source();

CREATE FUNCTION require_roleplay_lifecycle_user_action_resolution()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload ? 'roleplay_user_ongoing_action' AND
       NOT EXISTS (
           SELECT 1
           FROM roleplay_ongoing_action_resolutions AS resolution
           WHERE resolution.completion_operation_id=NEW.operation_id
             AND resolution.source_kind='user_action'
             AND resolution.source_position=-1
             AND resolution.character_id=
                 NEW.command_payload#>>'{roleplay_user_ongoing_action,character_id}'
             AND COALESCE(
                 to_jsonb(resolution.previous_action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
             AND COALESCE(
                 to_jsonb(resolution.action_text),'null'::jsonb
             )=NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             AND resolution.changed=(
                 NEW.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                 IS DISTINCT FROM
                 NEW.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
             )
       ) THEN
        RAISE EXCEPTION 'roleplay user ongoing-action lifecycle payload lacks its exact resolution receipt';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_user_action_requires_resolution
AFTER INSERT ON job_lifecycle_operations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_lifecycle_user_action_resolution();

CREATE OR REPLACE FUNCTION roleplay_lifecycle_response_round_valid(responses_value JSONB)
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
               'position','character_id','output','facts','knowledge_character_ids',
               'previous_ongoing_action','ongoing_action'
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
           jsonb_typeof(response->'knowledge_character_ids')<>'array' OR
           COALESCE(jsonb_typeof(response->'previous_ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           COALESCE(jsonb_typeof(response->'ongoing_action'),'missing')
               NOT IN ('missing','string','null') OR
           (jsonb_typeof(response->'previous_ongoing_action')='string' AND (
               octet_length(response->>'previous_ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'previous_ongoing_action'<>btrim(response->>'previous_ongoing_action')
           )) OR
           (jsonb_typeof(response->'ongoing_action')='string' AND (
               octet_length(response->>'ongoing_action') NOT BETWEEN 1 AND 512 OR
               response->>'ongoing_action'<>btrim(response->>'ongoing_action')
           )) THEN
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
                'roleplay_responses','roleplay_user_ongoing_action'
            ]='{}'::jsonb AND
            roleplay_lifecycle_response_round_valid(
                COALESCE(command_payload->'roleplay_responses','[]'::jsonb)
            ) AND (
                NOT command_payload ? 'roleplay_user_ongoing_action' OR
                roleplay_user_ongoing_action_payload_valid(
                    command_payload->'roleplay_user_ongoing_action'
                )
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

CREATE OR REPLACE FUNCTION roleplay_terminal_simulation_publication_valid(
    target_preparation_id TEXT,
    target_advance_operation_id TEXT
)
RETURNS BOOLEAN AS $$
    SELECT COUNT(*)=1
    FROM (
        SELECT preparation.operation_id
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=preparation.operation_id
         AND advance.job_id=binding.job_id
        JOIN jobs AS job ON job.id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.job_id=binding.job_id
         AND operation.kind='complete_step'
         AND operation.result_job_status='completed'
         AND operation.result_step_status='completed'
        WHERE preparation.operation_id=target_preparation_id
          AND (target_advance_operation_id IS NULL OR
               advance.operation_id=target_advance_operation_id)
          AND advance.world_id=preparation.world_id
          AND advance.scene_id=preparation.scene_id
          AND advance.before_revision=preparation.scene_revision
          AND advance.previous_character_id=preparation.active_character_id
          AND advance.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND advance.participant_character_ids=
              preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              advance.result->'before_initiative'
          AND advance.result->'after_initiative'=jsonb_build_object(
              'round',advance.after_initiative_round,
              'turn',advance.after_initiative_turn,
              'fictional_time_tick',advance.after_fictional_time_tick
          )
          AND job.pipeline='chat' AND job.status='completed'
          AND job.result=operation.command_payload->>'output'
          AND operation.command_payload->>'context_key'='objective_result'
          AND preparation.result->'user_turn'=user_turn.authority
          AND (
              (preparation.pending_transition_id IS NULL AND NOT EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.operation_id
              )) OR
              (preparation.pending_transition_id=preparation.operation_id AND EXISTS (
                  SELECT 1 FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.pending_transition_id
                    AND transition.world_id=preparation.world_id
                    AND transition.scene_id=preparation.scene_id
                    AND transition.actor_character_id=preparation.active_character_id
                    AND transition.before_revision=preparation.base_scene_revision
                    AND transition.after_revision=preparation.scene_revision
                    AND transition.result=preparation.result->'pending_transition'
              ))
          )
          AND (
              (
                  jsonb_typeof(operation.command_payload->'roleplay_responses')='array' AND
                  jsonb_array_length(operation.command_payload->'roleplay_responses')>0 AND
                  (SELECT COUNT(*) FROM roleplay_turn_completions AS fictional
                   WHERE fictional.operation_id=operation.operation_id)=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='response')=
                      jsonb_array_length(operation.command_payload->'roleplay_responses') AND
                  NOT EXISTS (
                      SELECT 1
                      FROM jsonb_array_elements(operation.command_payload->'roleplay_responses')
                           WITH ORDINALITY AS response(value,ordinal)
                      LEFT JOIN roleplay_turn_completions AS fictional
                        ON fictional.operation_id=operation.operation_id
                       AND fictional.response_position=ordinal-1
                      LEFT JOIN ai_channel_messages AS message
                        ON message.id=fictional.source_message_id
                      LEFT JOIN roleplay_ongoing_action_resolutions AS resolution
                        ON resolution.completion_operation_id=operation.operation_id
                       AND resolution.source_kind='response'
                       AND resolution.source_position=ordinal-1
                      WHERE (value->>'position')::integer<>ordinal-1 OR
                            fictional.world_id IS NULL OR
                            fictional.world_id<>preparation.world_id OR
                            fictional.viewpoint_character_id<>value->>'character_id' OR
                            fictional.viewpoint_character_id<>
                                preparation.result->'responder_routes'->(ordinal::integer-1)->>'character_id' OR
                            fictional.facts<>value->'facts' OR
                            fictional.knowledge_character_ids<>value->'knowledge_character_ids' OR
                            fictional.authority_namespace<>'FICTIONAL_CANON' OR
                            message.channel_id<>preparation.channel_id OR
                            message.role<>'assistant' OR message.content<>value->>'output' OR
                            resolution.completion_operation_id IS NULL OR
                            resolution.world_id<>fictional.world_id OR
                            resolution.character_id<>fictional.viewpoint_character_id OR
                            resolution.source_message_id<>fictional.source_message_id OR
                            resolution.authority_namespace<>'SIMULATION_STATE' OR
                            COALESCE(
                                to_jsonb(resolution.previous_action_text),'null'::jsonb
                            )<>COALESCE(value->'previous_ongoing_action','null'::jsonb) OR
                            COALESCE(
                                to_jsonb(resolution.action_text),'null'::jsonb
                            )<>COALESCE(value->'ongoing_action','null'::jsonb) OR
                            resolution.changed<>(
                                COALESCE(value->'previous_ongoing_action','null'::jsonb)
                                IS DISTINCT FROM
                                COALESCE(value->'ongoing_action','null'::jsonb)
                            )
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      WHERE real_world.operation_id=operation.operation_id
                  )
              ) OR (
                  NOT operation.command_payload ? 'roleplay_responses' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_turn_completions AS fictional
                      WHERE fictional.operation_id=operation.operation_id
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='response'
                  ) AND EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      JOIN ai_channel_messages AS message
                        ON message.id=real_world.source_message_id
                      WHERE real_world.operation_id=operation.operation_id
                        AND real_world.preparation_id=preparation.operation_id
                        AND real_world.job_id=binding.job_id
                        AND real_world.authority_namespace='REAL_WORLD'
                        AND message.channel_id=preparation.channel_id
                        AND message.role='assistant'
                        AND message.content=operation.command_payload->>'output'
                  )
              )
          )
          AND (
              (
                  user_turn.persona_kind='character' AND
                  EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  (SELECT COUNT(*)
                   FROM roleplay_ongoing_action_resolutions AS resolution
                   WHERE resolution.completion_operation_id=operation.operation_id
                     AND resolution.source_kind='user_action'
                     AND resolution.source_position=-1
                     AND resolution.world_id=preparation.world_id
                     AND resolution.character_id=user_turn.persona_character_id
                     AND resolution.source_message_id=user_turn.user_message_id
                     AND resolution.authority_namespace='SIMULATION_STATE'
                     AND (
                         (
                             operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             roleplay_user_ongoing_action_payload_valid(
                                 operation.command_payload->'roleplay_user_ongoing_action'
                             ) AND
                             operation.command_payload#>>'{roleplay_user_ongoing_action,character_id}'=
                                 resolution.character_id AND
                             COALESCE(
                                 to_jsonb(resolution.previous_action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}' AND
                             COALESCE(
                                 to_jsonb(resolution.action_text),'null'::jsonb
                             )=operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}' AND
                             resolution.changed=(
                                 operation.command_payload#>'{roleplay_user_ongoing_action,previous_ongoing_action}'
                                 IS DISTINCT FROM
                                 operation.command_payload#>'{roleplay_user_ongoing_action,ongoing_action}'
                             )
                         ) OR (
                             NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                             NOT resolution.changed AND
                             resolution.previous_state_id IS NULL AND
                             resolution.current_state_id IS NULL AND
                             resolution.previous_action_text IS NULL AND
                             resolution.action_text IS NULL
                         )
                     ))=1
              ) OR (
                  NOT EXISTS (
                      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
                      WHERE part.value->>'kind'='action'
                  ) AND
                  NOT operation.command_payload ? 'roleplay_user_ongoing_action' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_ongoing_action_resolutions AS resolution
                      WHERE resolution.completion_operation_id=operation.operation_id
                        AND resolution.source_kind='user_action'
                  )
              )
          )
    ) AS exact_terminal_publication;
$$ LANGUAGE SQL STABLE;

CREATE OR REPLACE FUNCTION station_owns_portable_work(
    station TEXT, work_kind TEXT, payload JSONB
)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_context_needs' THEN station='coding_requirements'
        WHEN 'application_intent' THEN station='coding_requirements'
        WHEN 'repository_requirements' THEN station='coding_requirements'
        WHEN 'application_job_specification' THEN station='coding_workload'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_deployment_intent' THEN station='coding_service_deployment_intent'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_service_state_interface' THEN station='coding_service_state_interface'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
        -- Retained only for immutable historical opening rows. The insert
        -- guard rejects new bundled work and its corrections.
        WHEN 'application_service_endpoint_contract' THEN station='coding_service_endpoint_contract'
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        -- These mappings are retained only so immutable historical opening rows
        -- remain valid. Current runtime code does not dispatch them, and the
        -- insert guard rejects every new opening and correction for them.
        WHEN 'application_requirements' THEN station='coding_requirements'
        WHEN 'application_file_content' THEN station='coding_workload'
        WHEN 'application_job_specification_repair' THEN station='coding_workload'
        WHEN 'application_job_specification_review' THEN
            station IN ('coding_workload','coding_workload_review')
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'memory_context_selection' THEN station='memory_context_selection'
        WHEN 'roleplay_narrative_continuity' THEN station='roleplay_narrative_continuity'
        WHEN 'context_search_terms' THEN station='context_search_terms'
        WHEN 'context_relevance' THEN station='context_relevance'
        WHEN 'context_minification' THEN station='context_minification'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response' THEN station='conversation_response'
        WHEN 'roleplay_canon_extraction' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_ongoing_action' THEN station='roleplay_ongoing_action'
        WHEN 'roleplay_voice_rewrite' THEN station='roleplay_voice_rewrite'
        WHEN 'roleplay_voice_preservation' THEN station='roleplay_voice_preservation'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'database_schema_selection' THEN station='database_schema_selection'
        WHEN 'database_query_intent' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'
        WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'known_artifact_truth' THEN station='coding_known_artifact_truth'
        WHEN 'declaration_artifact_boundary' THEN station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(
            station,payload->'original'->>'kind',payload->'original'->'payload'
        ),FALSE)
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
BEGIN
    IF (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions
        WHERE source_kind='response')<>
       (SELECT COUNT(*) FROM roleplay_turn_completions) OR
       (SELECT COUNT(*) FROM roleplay_ongoing_action_resolutions
        WHERE source_kind='user_action')<>
       (SELECT COUNT(*)
        FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
        JOIN job_lifecycle_operations AS operation
          ON operation.job_id=binding.job_id
         AND operation.kind='complete_step'
         AND operation.result_job_status='completed'
         AND operation.result_step_status='completed'
         AND operation.command_payload->>'context_key'='objective_result'
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        WHERE user_turn.persona_kind='character'
          AND preparation.result->'user_turn'=user_turn.authority
          AND preparation.result->'participant_character_ids' ?
              user_turn.persona_character_id
          AND EXISTS (
              SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
              WHERE part.value->>'kind'='action'
          )) OR
       EXISTS (SELECT 1 FROM roleplay_ongoing_action_states) OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_ongoing_action_resolutions'::regclass
             AND tgname='roleplay_ongoing_action_resolutions_require_lifecycle_source'
             AND tgdeferrable AND tginitdeferred AND NOT tgisinternal
       ) OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='job_lifecycle_operations'::regclass
             AND tgname='roleplay_lifecycle_user_action_requires_resolution'
             AND tgdeferrable AND tginitdeferred AND NOT tgisinternal
       ) OR
       station_owns_portable_work(
           'roleplay_ongoing_action','roleplay_ongoing_action','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR
       station_owns_portable_work(
           'conversation_response','roleplay_ongoing_action','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR
       EXISTS (
           SELECT 1 FROM station_gap_openings AS opening
           WHERE station_owns_portable_work(
               opening.station,opening.work_kind,opening.portable_payload::jsonb
           ) IS DISTINCT FROM TRUE
       ) THEN
        RAISE EXCEPTION 'roleplay ongoing-action authority postcondition failed';
    END IF;
END $$;

COMMIT;
