BEGIN;

LOCK TABLE roleplay_current_scenes, roleplay_scene_participants,
    roleplay_simulation_turn_preparations, roleplay_simulation_preparation_jobs,
    roleplay_simulation_turn_advances, jobs
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF to_regclass(current_schema()||'.roleplay_research_turns') IS NOT NULL THEN
        EXECUTE 'LOCK TABLE roleplay_research_turns IN ACCESS EXCLUSIVE MODE';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND metadata->>'channel_mode'='roleplay'
    ) THEN
        RAISE EXCEPTION 'cannot install roleplay initiative authority while a roleplay turn is active';
    END IF;
END $$;

DROP TRIGGER roleplay_scenes_binding_immutable ON roleplay_current_scenes;
DROP TRIGGER roleplay_simulation_preparations_immutable
    ON roleplay_simulation_turn_preparations;
DROP TRIGGER roleplay_simulation_turn_advances_immutable
    ON roleplay_simulation_turn_advances;
DROP TRIGGER jobs_chat_turn_binding_immutable ON jobs;

ALTER TABLE roleplay_current_scenes
    ADD COLUMN initiative_round BIGINT,
    ADD COLUMN initiative_turn BIGINT,
    ADD COLUMN fictional_time_tick BIGINT;

ALTER TABLE roleplay_simulation_turn_advances
    ADD COLUMN before_initiative_round BIGINT,
    ADD COLUMN before_initiative_turn BIGINT,
    ADD COLUMN before_fictional_time_tick BIGINT,
    ADD COLUMN after_initiative_round BIGINT,
    ADD COLUMN after_initiative_turn BIGINT,
    ADD COLUMN after_fictional_time_tick BIGINT;

CREATE FUNCTION roleplay_initiative_clock_valid(clock_value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN jsonb_typeof(clock_value)='object' AND
           clock_value ?& ARRAY['round','turn','fictional_time_tick'] AND
           clock_value - ARRAY['round','turn','fictional_time_tick']='{}'::jsonb AND
           jsonb_typeof(clock_value->'round')='number' AND
           jsonb_typeof(clock_value->'turn')='number' AND
           jsonb_typeof(clock_value->'fictional_time_tick')='number' AND
           (clock_value->>'round')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'turn')::bigint BETWEEN 1 AND 9007199254740991 AND
           (clock_value->>'fictional_time_tick')::bigint BETWEEN 0 AND 9007199254740990 AND
           (clock_value->>'turn')::bigint=(clock_value->>'fictional_time_tick')::bigint+1 AND
           (clock_value->>'round')::bigint<=(clock_value->>'turn')::bigint;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION roleplay_expected_responder_ids(result_value JSONB)
RETURNS JSONB AS $$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
    expected JSONB := '[]'::jsonb;
BEGIN
    IF jsonb_typeof(participants)<>'array' OR
       jsonb_array_length(participants) NOT BETWEEN 1 AND 16 OR
       jsonb_typeof(result_value->'user_turn')<>'object' THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    IF result_value->'user_turn'->>'persona_kind'='legacy_untyped' THEN
        IF jsonb_typeof(result_value->'responder_routes')<>'array' OR
           jsonb_array_length(result_value->'responder_routes')<>1 OR
           NOT (participants ? (result_value->'responder_routes'->0->>'character_id')) THEN
            RETURN NULL;
        END IF;
        RETURN jsonb_build_array(result_value->'responder_routes'->0->>'character_id');
    END IF;
    IF (SELECT COUNT(*)<>COUNT(DISTINCT item.value #>> '{}')
        FROM jsonb_array_elements(participants) AS item(value)) OR
       EXISTS (
           SELECT 1 FROM jsonb_array_elements(participants) AS item(value)
           WHERE jsonb_typeof(item.value)<>'string' OR
                 NOT ((item.value #>> '{}') ~ '^rpc_[0-9a-f]{32}$')
       ) THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF active_index<0 THEN
        RETURN NULL;
    END IF;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
        IF user_character_id IS NULL OR NOT (participants ? user_character_id) THEN
            RETURN NULL;
        END IF;
    ELSIF result_value->'user_turn'->>'persona_kind'<>'narrator' THEN
        RETURN NULL;
    END IF;
    FOR offset_value IN 0..participant_count-1 LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            expected := expected || jsonb_build_array(candidate_id);
        END IF;
    END LOOP;
    IF jsonb_array_length(expected)<1 THEN
        RETURN NULL;
    END IF;
    RETURN expected;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION roleplay_next_initiative_character(result_value JSONB)
RETURNS TEXT AS $$
DECLARE
    participants JSONB := result_value->'participant_character_ids';
    participant_count INTEGER;
    active_index INTEGER := -1;
    offset_value INTEGER;
    candidate_id TEXT;
    user_character_id TEXT;
BEGIN
    IF roleplay_expected_responder_ids(result_value) IS NULL THEN
        RETURN NULL;
    END IF;
    participant_count := jsonb_array_length(participants);
    FOR offset_value IN 0..participant_count-1 LOOP
        IF participants->>offset_value=result_value->>'active_character_id' THEN
            active_index := offset_value;
            EXIT;
        END IF;
    END LOOP;
    IF result_value->'user_turn'->>'persona_kind'='character' THEN
        user_character_id := result_value->'user_turn'->>'character_id';
    END IF;
    FOR offset_value IN 1..participant_count LOOP
        candidate_id := participants->>((active_index+offset_value)%participant_count);
        IF user_character_id IS NULL OR candidate_id<>user_character_id THEN
            RETURN candidate_id;
        END IF;
    END LOOP;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION roleplay_initiative_advance_valid(
    before_round BIGINT,
    before_turn BIGINT,
    before_tick BIGINT,
    after_round BIGINT,
    after_turn BIGINT,
    after_tick BIGINT,
    previous_character TEXT,
    active_character TEXT,
    participants JSONB
)
RETURNS BOOLEAN AS $$
DECLARE
    previous_index BIGINT;
    active_index BIGINT;
BEGIN
    IF before_round NOT BETWEEN 1 AND 9007199254740991 OR
       before_turn NOT BETWEEN 1 AND 9007199254740990 OR
       before_tick NOT BETWEEN 0 AND 9007199254740989 OR
       before_turn<>before_tick+1 OR before_round>before_turn OR
       after_turn<>before_turn+1 OR after_tick<>before_tick+1 OR
       after_turn<>after_tick+1 OR after_round>after_turn THEN
        RETURN FALSE;
    END IF;
    SELECT item.ordinal INTO previous_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=previous_character;
    SELECT item.ordinal INTO active_index
    FROM jsonb_array_elements_text(participants) WITH ORDINALITY AS item(value,ordinal)
    WHERE item.value=active_character;
    RETURN previous_index IS NOT NULL AND active_index IS NOT NULL AND
           after_round=before_round+CASE WHEN active_index<=previous_index THEN 1 ELSE 0 END;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_advances AS advance
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=advance.preparation_id
        WHERE roleplay_next_initiative_character(preparation.result) IS NULL OR
              advance.previous_character_id IS DISTINCT FROM preparation.active_character_id OR
              advance.before_revision IS DISTINCT FROM preparation.scene_revision OR
              advance.participant_character_ids IS DISTINCT FROM
                  preparation.result->'participant_character_ids' OR
              advance.result->>'previous_character_id' IS DISTINCT FROM
                  advance.previous_character_id OR
              advance.result->>'active_character_id' IS DISTINCT FROM
                  advance.active_character_id OR
              advance.result->'participant_character_ids' IS DISTINCT FROM
                  advance.participant_character_ids OR
              (
                  advance.active_character_id IS DISTINCT FROM
                      roleplay_next_initiative_character(preparation.result) AND
                  advance.active_character_id IS DISTINCT FROM
                      preparation.result->'responder_routes'->0->>'character_id'
              )
    ) THEN
        RAISE EXCEPTION 'cannot reconstruct contradictory legacy turn-advance authority';
    END IF;
    IF EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_advances
        GROUP BY scene_id,before_revision HAVING COUNT(*)<>1
    ) THEN
        RAISE EXCEPTION 'cannot reconstruct forked legacy scene turn advances';
    END IF;
END $$;

CREATE TEMP TABLE roleplay_initiative_advance_backfill ON COMMIT DROP AS
WITH positioned AS (
    SELECT advance.operation_id,advance.scene_id,advance.before_revision,
           advance.after_revision,expected.active_character_id,
           CASE WHEN active.ordinal<=previous.ordinal THEN 1 ELSE 0 END AS wrapped
    FROM roleplay_simulation_turn_advances AS advance
    JOIN roleplay_simulation_turn_preparations AS preparation
      ON preparation.operation_id=advance.preparation_id
    JOIN LATERAL (
        SELECT roleplay_next_initiative_character(preparation.result) AS active_character_id
    ) AS expected ON expected.active_character_id IS NOT NULL
    JOIN LATERAL (
        SELECT item.ordinal
        FROM jsonb_array_elements_text(advance.participant_character_ids)
             WITH ORDINALITY AS item(value,ordinal)
        WHERE item.value=advance.previous_character_id
    ) AS previous ON TRUE
    JOIN LATERAL (
        SELECT item.ordinal
        FROM jsonb_array_elements_text(advance.participant_character_ids)
             WITH ORDINALITY AS item(value,ordinal)
        WHERE item.value=expected.active_character_id
    ) AS active ON TRUE
), numbered AS (
    SELECT positioned.*,
           row_number() OVER (
               PARTITION BY scene_id ORDER BY before_revision,operation_id
           ) AS initiative_turn,
           COALESCE(sum(wrapped) OVER (
               PARTITION BY scene_id ORDER BY before_revision,operation_id
               ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
           ),0) AS prior_wraps
    FROM positioned
)
SELECT operation_id,scene_id,before_revision,after_revision,active_character_id,
       1+prior_wraps AS before_round,
       initiative_turn AS before_turn,
       initiative_turn-1 AS before_tick,
       1+prior_wraps+wrapped AS after_round,
       initiative_turn+1 AS after_turn,
       initiative_turn AS after_tick,
       wrapped
FROM numbered;

DO $$
BEGIN
    IF (SELECT COUNT(*) FROM roleplay_initiative_advance_backfill)<>
       (SELECT COUNT(*) FROM roleplay_simulation_turn_advances) THEN
        RAISE EXCEPTION 'cannot derive initiative clocks from persisted turn advances';
    END IF;
END $$;

UPDATE roleplay_simulation_turn_advances AS advance
SET active_character_id=clock.active_character_id,
    before_initiative_round=clock.before_round,
    before_initiative_turn=clock.before_turn,
    before_fictional_time_tick=clock.before_tick,
    after_initiative_round=clock.after_round,
    after_initiative_turn=clock.after_turn,
    after_fictional_time_tick=clock.after_tick,
    result=jsonb_set(
        advance.result,'{active_character_id}',to_jsonb(clock.active_character_id),FALSE
    )
FROM roleplay_initiative_advance_backfill AS clock
WHERE clock.operation_id=advance.operation_id;

UPDATE roleplay_current_scenes AS scene
SET initiative_round=1+summary.wrap_count,
    initiative_turn=1+summary.turn_count,
    fictional_time_tick=summary.turn_count
FROM (
    SELECT scene_id,COUNT(*) AS turn_count,SUM(wrapped) AS wrap_count
    FROM roleplay_initiative_advance_backfill GROUP BY scene_id
) AS summary
WHERE summary.scene_id=scene.id;

UPDATE roleplay_current_scenes AS scene
SET current_character_id=latest.active_character_id
FROM (
    SELECT DISTINCT ON (scene_id) scene_id,after_revision,active_character_id
    FROM roleplay_initiative_advance_backfill
    ORDER BY scene_id,after_revision DESC,operation_id DESC
) AS latest
WHERE scene.id=latest.scene_id AND scene.revision=latest.after_revision;

UPDATE roleplay_current_scenes
SET initiative_round=1,initiative_turn=1,fictional_time_tick=0
WHERE initiative_round IS NULL;

CREATE TEMP TABLE roleplay_initiative_preparation_backfill ON COMMIT DROP AS
SELECT preparation.operation_id,
       1+COALESCE(SUM(clock.wrapped),0) AS initiative_round,
       1+COUNT(clock.operation_id) AS initiative_turn,
       COUNT(clock.operation_id) AS fictional_time_tick
FROM roleplay_simulation_turn_preparations AS preparation
LEFT JOIN roleplay_simulation_turn_advances AS advance
  ON advance.scene_id=preparation.scene_id
 AND advance.after_revision<=preparation.base_scene_revision
LEFT JOIN roleplay_initiative_advance_backfill AS clock
  ON clock.operation_id=advance.operation_id
GROUP BY preparation.operation_id;

CREATE FUNCTION roleplay_clocked_narrative_fingerprint(
    old_fingerprint TEXT, initiative_round BIGINT, initiative_turn BIGINT, initiative_tick BIGINT
)
RETURNS TEXT AS $$
    SELECT encode(public.digest(convert_to(
        old_fingerprint||':'||initiative_round::text||':'||initiative_turn::text||':'||initiative_tick::text,
        'UTF8'
    ),'sha256'),'hex');
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE FUNCTION roleplay_backfill_preparation_initiative(
    result_value JSONB, initiative_round BIGINT, initiative_turn BIGINT, initiative_tick BIGINT
)
RETURNS JSONB AS $$
DECLARE
    expected_ids JSONB := roleplay_expected_responder_ids(result_value);
    initiative JSONB := jsonb_build_object(
        'round',initiative_round,'turn',initiative_turn,'fictional_time_tick',initiative_tick
    );
    responders JSONB := '[]'::jsonb;
    routes JSONB := '[]'::jsonb;
    responder JSONB;
    route JSONB;
    character_id TEXT;
    new_fingerprint TEXT;
    index_value INTEGER;
BEGIN
    IF expected_ids IS NULL OR NOT roleplay_initiative_clock_valid(initiative) THEN
        RAISE EXCEPTION 'cannot derive exact responder initiative authority';
    END IF;
    FOR index_value IN 0..jsonb_array_length(expected_ids)-1 LOOP
        character_id := expected_ids->>index_value;
        SELECT item.value INTO STRICT responder
        FROM jsonb_array_elements(result_value->'responders') AS item(value)
        WHERE item.value->>'character_id'=character_id;
        SELECT item.value INTO STRICT route
        FROM jsonb_array_elements(result_value->'responder_routes') AS item(value)
        WHERE item.value->>'character_id'=character_id;
        new_fingerprint := roleplay_clocked_narrative_fingerprint(
            responder->>'narrative_fingerprint',initiative_round,initiative_turn,initiative_tick
        );
        responder := jsonb_set(responder,'{position}',to_jsonb(index_value),FALSE);
        responder := jsonb_set(
            responder,'{narrative_projection,scene,initiative}',initiative,TRUE
        );
        responder := jsonb_set(
            responder,'{narrative_authority,fingerprint}',to_jsonb(new_fingerprint),FALSE
        );
        responder := jsonb_set(
            responder,'{narrative_fingerprint}',to_jsonb(new_fingerprint),FALSE
        );
        route := jsonb_set(route,'{position}',to_jsonb(index_value),FALSE);
        route := jsonb_set(
            route,'{narrative_fingerprint}',to_jsonb(new_fingerprint),FALSE
        );
        responders := responders || jsonb_build_array(responder);
        routes := routes || jsonb_build_array(route);
    END LOOP;
    responder := responders->0;
    result_value := jsonb_set(result_value,'{responders}',responders,FALSE);
    result_value := jsonb_set(result_value,'{responder_routes}',routes,FALSE);
    result_value := jsonb_set(
        result_value,'{generation_config}',responder->'generation_config',FALSE
    );
    result_value := jsonb_set(
        result_value,'{narrative_projection}',responder->'narrative_projection',FALSE
    );
    result_value := jsonb_set(
        result_value,'{narrative_authority}',responder->'narrative_authority',FALSE
    );
    RETURN jsonb_set(
        result_value,'{narrative_fingerprint}',responder->'narrative_fingerprint',FALSE
    );
EXCEPTION WHEN NO_DATA_FOUND OR TOO_MANY_ROWS THEN
    RAISE EXCEPTION 'cannot bind initiative to an exact persisted responder';
END;
$$ LANGUAGE plpgsql;

UPDATE roleplay_simulation_turn_preparations AS preparation
SET result=roleplay_backfill_preparation_initiative(
    preparation.result,clock.initiative_round,clock.initiative_turn,clock.fictional_time_tick
)
FROM roleplay_initiative_preparation_backfill AS clock
WHERE clock.operation_id=preparation.operation_id;

UPDATE jobs AS job
SET metadata=jsonb_set(
        jsonb_set(
            jsonb_set(
                jsonb_set(
                    job.metadata,
                    '{roleplay_narrative_fingerprint}',
                    preparation.result->'narrative_fingerprint',TRUE
                ),
                '{roleplay_viewpoint_character_id}',
                to_jsonb(preparation.result->'responder_routes'->0->>'character_id'),TRUE
            ),
            '{roleplay_generation_config}',preparation.result->'generation_config',TRUE
        ),
        '{roleplay_responders}',preparation.result->'responder_routes',TRUE
    )
FROM roleplay_simulation_preparation_jobs AS binding
JOIN roleplay_simulation_turn_preparations AS preparation
  ON preparation.operation_id=binding.preparation_id
WHERE job.id=binding.job_id;

DO $$
BEGIN
    IF to_regclass(current_schema()||'.roleplay_research_turns') IS NOT NULL THEN
        EXECUTE 'DROP TRIGGER roleplay_research_turns_immutable ON roleplay_research_turns';
        EXECUTE $update$
            UPDATE roleplay_research_turns AS research
            SET narrative_fingerprint=preparation.result->>'narrative_fingerprint'
            FROM roleplay_simulation_turn_preparations AS preparation
            WHERE preparation.operation_id=research.preparation_id
        $update$;
        EXECUTE $trigger$
            CREATE TRIGGER roleplay_research_turns_immutable
            BEFORE UPDATE OR DELETE ON roleplay_research_turns
            FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation()
        $trigger$;
    END IF;
END $$;

UPDATE roleplay_simulation_turn_advances AS advance
SET narrative_fingerprint=roleplay_clocked_narrative_fingerprint(
        advance.narrative_fingerprint,advance.after_initiative_round,
        advance.after_initiative_turn,advance.after_fictional_time_tick
    ),
    result=jsonb_set(
        jsonb_set(
            jsonb_set(
                advance.result,
                '{before_initiative}',jsonb_build_object(
                    'round',advance.before_initiative_round,
                    'turn',advance.before_initiative_turn,
                    'fictional_time_tick',advance.before_fictional_time_tick
                ),TRUE
            ),
            '{after_initiative}',jsonb_build_object(
                'round',advance.after_initiative_round,
                'turn',advance.after_initiative_turn,
                'fictional_time_tick',advance.after_fictional_time_tick
            ),TRUE
        ),
        '{narrative_fingerprint}',to_jsonb(roleplay_clocked_narrative_fingerprint(
            advance.narrative_fingerprint,advance.after_initiative_round,
            advance.after_initiative_turn,advance.after_fictional_time_tick
        )),FALSE
    );

DROP FUNCTION roleplay_backfill_preparation_initiative(JSONB,BIGINT,BIGINT,BIGINT);
DROP FUNCTION roleplay_clocked_narrative_fingerprint(TEXT,BIGINT,BIGINT,BIGINT);

-- The backfill updates tables that already carry deferred integrity triggers.
-- Drain those events before changing either table's shape.
SET CONSTRAINTS ALL IMMEDIATE;
SET CONSTRAINTS ALL DEFERRED;

ALTER TABLE roleplay_current_scenes
    ALTER COLUMN initiative_round SET DEFAULT 1,
    ALTER COLUMN initiative_round SET NOT NULL,
    ALTER COLUMN initiative_turn SET DEFAULT 1,
    ALTER COLUMN initiative_turn SET NOT NULL,
    ALTER COLUMN fictional_time_tick SET DEFAULT 0,
    ALTER COLUMN fictional_time_tick SET NOT NULL,
    ADD CONSTRAINT roleplay_current_scenes_initiative_check CHECK (
        initiative_round BETWEEN 1 AND 9007199254740991 AND
        initiative_turn BETWEEN 1 AND 9007199254740991 AND
        fictional_time_tick BETWEEN 0 AND 9007199254740990 AND
        initiative_turn=fictional_time_tick+1 AND initiative_round<=initiative_turn
    );

ALTER TABLE roleplay_simulation_turn_advances
    ALTER COLUMN before_initiative_round SET NOT NULL,
    ALTER COLUMN before_initiative_turn SET NOT NULL,
    ALTER COLUMN before_fictional_time_tick SET NOT NULL,
    ALTER COLUMN after_initiative_round SET NOT NULL,
    ALTER COLUMN after_initiative_turn SET NOT NULL,
    ALTER COLUMN after_fictional_time_tick SET NOT NULL,
    ADD CONSTRAINT roleplay_simulation_turn_advances_scene_revision_unique
        UNIQUE (scene_id,before_revision),
    ADD CONSTRAINT roleplay_simulation_turn_advances_initiative_check CHECK (
        roleplay_initiative_advance_valid(
            before_initiative_round,before_initiative_turn,before_fictional_time_tick,
            after_initiative_round,after_initiative_turn,after_fictional_time_tick,
            previous_character_id,active_character_id,participant_character_ids
        )
    ),
    DROP CONSTRAINT roleplay_simulation_turn_advances_result_check,
    ADD CONSTRAINT roleplay_simulation_turn_advances_result_check CHECK (
        jsonb_typeof(result)='object' AND octet_length(result::text)<=32768 AND
        result ?& ARRAY[
            'operation_id','preparation_id','world_id','scene_id','previous_character_id',
            'active_character_id','before_revision','after_revision','before_initiative',
            'after_initiative','participant_character_ids','narrative_fingerprint','created_at'
        ] AND
        result->'before_initiative'=jsonb_build_object(
            'round',before_initiative_round,'turn',before_initiative_turn,
            'fictional_time_tick',before_fictional_time_tick
        ) AND
        result->'after_initiative'=jsonb_build_object(
            'round',after_initiative_round,'turn',after_initiative_turn,
            'fictional_time_tick',after_fictional_time_tick
        ) AND jsonb_typeof(result->'participant_character_ids')='array'
    );

CREATE OR REPLACE FUNCTION roleplay_response_round_valid(result_value JSONB)
RETURNS BOOLEAN AS $$
DECLARE
    responder JSONB;
    route JSONB;
    expected_ids JSONB;
    actual_ids JSONB;
    frozen_initiative JSONB;
    index_value INTEGER;
BEGIN
    IF jsonb_typeof(result_value->'responders')<>'array' OR
       jsonb_typeof(result_value->'responder_routes')<>'array' OR
       jsonb_array_length(result_value->'responders') NOT BETWEEN 1 AND 16 OR
       jsonb_array_length(result_value->'responders')<>
           jsonb_array_length(result_value->'responder_routes') THEN
        RETURN FALSE;
    END IF;
    expected_ids := roleplay_expected_responder_ids(result_value);
    SELECT COALESCE(
        jsonb_agg(to_jsonb(item.value->>'character_id') ORDER BY item.ordinal),
        '[]'::jsonb
    ) INTO actual_ids
    FROM jsonb_array_elements(result_value->'responder_routes')
         WITH ORDINALITY AS item(value,ordinal);
    IF expected_ids IS NULL OR expected_ids<>actual_ids THEN
        RETURN FALSE;
    END IF;
    FOR index_value IN 0..jsonb_array_length(result_value->'responders')-1 LOOP
        responder := result_value->'responders'->index_value;
        route := result_value->'responder_routes'->index_value;
        IF index_value=0 THEN
            frozen_initiative := responder#>'{narrative_projection,scene,initiative}';
        END IF;
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
           jsonb_typeof(responder->'generation_config')<>'object' OR
           NOT roleplay_initiative_clock_valid(
               responder#>'{narrative_projection,scene,initiative}'
           ) OR responder#>'{narrative_projection,scene,initiative}'<>frozen_initiative THEN
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
          AND NEW.result#>'{narrative_projection,scene,initiative}'=jsonb_build_object(
              'round',scene.initiative_round,'turn',scene.initiative_turn,
              'fictional_time_tick',scene.fictional_time_tick
          )
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
        RAISE EXCEPTION 'simulation preparation differs from its exact user, scene, initiative, and response-round authority';
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

CREATE OR REPLACE FUNCTION validate_roleplay_simulation_advance()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id AND binding.job_id=NEW.job_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=preparation.world_id AND scene.id=preparation.scene_id
        JOIN roleplay_scene_participants AS previous
          ON previous.scene_id=scene.id AND previous.character_id=NEW.previous_character_id
        JOIN roleplay_scene_participants AS active
          ON active.scene_id=scene.id AND active.character_id=NEW.active_character_id
        WHERE preparation.operation_id=NEW.preparation_id
          AND preparation.world_id=NEW.world_id AND preparation.scene_id=NEW.scene_id
          AND preparation.scene_revision=NEW.before_revision
          AND preparation.active_character_id=NEW.previous_character_id
          AND NEW.active_character_id=roleplay_next_initiative_character(preparation.result)
          AND NEW.participant_character_ids=preparation.result->'participant_character_ids'
          AND preparation.result#>'{narrative_projection,scene,initiative}'=
              jsonb_build_object(
                  'round',NEW.before_initiative_round,'turn',NEW.before_initiative_turn,
                  'fictional_time_tick',NEW.before_fictional_time_tick
              )
          AND scene.revision=NEW.after_revision
          AND scene.current_character_id=NEW.active_character_id
          AND scene.initiative_round=NEW.after_initiative_round
          AND scene.initiative_turn=NEW.after_initiative_turn
          AND scene.fictional_time_tick=NEW.after_fictional_time_tick
    ) OR NOT roleplay_initiative_advance_valid(
           NEW.before_initiative_round,NEW.before_initiative_turn,NEW.before_fictional_time_tick,
           NEW.after_initiative_round,NEW.after_initiative_turn,NEW.after_fictional_time_tick,
           NEW.previous_character_id,NEW.active_character_id,NEW.participant_character_ids
       ) OR NEW.result->>'operation_id'<>NEW.operation_id OR
       NEW.result->>'preparation_id'<>NEW.preparation_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       NEW.result->>'previous_character_id'<>NEW.previous_character_id OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->'participant_character_ids'<>NEW.participant_character_ids OR
       NEW.result->>'narrative_fingerprint'<>NEW.narrative_fingerprint OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision THEN
        RAISE EXCEPTION 'simulation turn advance does not match exact preparation, scene, initiative, or result authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

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
                  NOT EXISTS (
                      SELECT 1
                      FROM jsonb_array_elements(operation.command_payload->'roleplay_responses')
                           WITH ORDINALITY AS response(value,ordinal)
                      LEFT JOIN roleplay_turn_completions AS fictional
                        ON fictional.operation_id=operation.operation_id
                       AND fictional.response_position=ordinal-1
                      LEFT JOIN ai_channel_messages AS message
                        ON message.id=fictional.source_message_id
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
                            message.role<>'assistant' OR message.content<>value->>'output'
                  ) AND NOT EXISTS (
                      SELECT 1 FROM roleplay_research_completions AS real_world
                      WHERE real_world.operation_id=operation.operation_id
                  )
              ) OR (
                  NOT operation.command_payload ? 'roleplay_responses' AND
                  NOT EXISTS (
                      SELECT 1 FROM roleplay_turn_completions AS fictional
                      WHERE fictional.operation_id=operation.operation_id
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
    ) AS exact_terminal_publication;
$$ LANGUAGE SQL STABLE;

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

CREATE FUNCTION require_roleplay_scene_initiative_advance()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(
        NEW.current_character_id,NEW.initiative_round,
        NEW.initiative_turn,NEW.fictional_time_tick
    ) IS NOT DISTINCT FROM ROW(
        OLD.current_character_id,OLD.initiative_round,
        OLD.initiative_turn,OLD.fictional_time_tick
    ) THEN
        RETURN NULL;
    END IF;
    -- Cast editing may remove the active character. In that one case the
    -- scene writer deterministically rebases the cursor to the first
    -- remaining participant without advancing fictional time.
    IF NEW.revision=OLD.revision+1 AND
       ROW(
           NEW.initiative_round,NEW.initiative_turn,NEW.fictional_time_tick
       ) IS NOT DISTINCT FROM ROW(
           OLD.initiative_round,OLD.initiative_turn,OLD.fictional_time_tick
       ) AND
       NOT EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=OLD.current_character_id
       ) AND
       EXISTS (
           SELECT 1 FROM roleplay_scene_participants AS participant
           WHERE participant.scene_id=NEW.id
             AND participant.character_id=NEW.current_character_id
             AND participant.turn_position=0
       ) THEN
        RETURN NULL;
    END IF;
    IF (
        SELECT COUNT(*)
        FROM roleplay_simulation_turn_advances AS advance
        WHERE advance.world_id=NEW.world_id AND advance.scene_id=NEW.id
          AND advance.before_revision=OLD.revision
          AND advance.after_revision=NEW.revision
          AND advance.previous_character_id=OLD.current_character_id
          AND advance.active_character_id=NEW.current_character_id
          AND advance.before_initiative_round=OLD.initiative_round
          AND advance.before_initiative_turn=OLD.initiative_turn
          AND advance.before_fictional_time_tick=OLD.fictional_time_tick
          AND advance.after_initiative_round=NEW.initiative_round
          AND advance.after_initiative_turn=NEW.initiative_turn
          AND advance.after_fictional_time_tick=NEW.fictional_time_tick
    )<>1 THEN
        RAISE EXCEPTION 'scene initiative mutation requires one exact authoritative turn advance';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_scenes_binding_immutable
BEFORE UPDATE ON roleplay_current_scenes
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_binding_change();
CREATE CONSTRAINT TRIGGER roleplay_scenes_require_initiative_advance
AFTER UPDATE ON roleplay_current_scenes
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_scene_initiative_advance();
CREATE TRIGGER roleplay_simulation_preparations_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_turn_advances_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_advances
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER jobs_chat_turn_binding_immutable
BEFORE UPDATE OF pipeline,metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_current_scenes'::regclass
          AND tgname='roleplay_scenes_require_initiative_advance'
          AND tgdeferrable AND tginitdeferred AND NOT tgisinternal
    ) OR EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations
        WHERE NOT roleplay_response_round_valid(result)
    ) OR EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_advances
        WHERE NOT roleplay_initiative_advance_valid(
            before_initiative_round,before_initiative_turn,before_fictional_time_tick,
            after_initiative_round,after_initiative_turn,after_fictional_time_tick,
            previous_character_id,active_character_id,participant_character_ids
        )
    ) OR EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_advances AS advance
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=advance.preparation_id
        WHERE advance.active_character_id<>
              roleplay_next_initiative_character(preparation.result)
    ) OR EXISTS (
        SELECT 1 FROM roleplay_current_scenes
        WHERE NOT roleplay_initiative_clock_valid(jsonb_build_object(
            'round',initiative_round,'turn',initiative_turn,
            'fictional_time_tick',fictional_time_tick
        ))
    ) OR EXISTS (
        SELECT 1
        FROM roleplay_current_scenes AS scene
        JOIN LATERAL (
            SELECT advance.*
            FROM roleplay_simulation_turn_advances AS advance
            WHERE advance.scene_id=scene.id
            ORDER BY advance.after_revision DESC,advance.operation_id DESC LIMIT 1
        ) AS latest ON latest.after_revision=scene.revision
        WHERE ROW(
            scene.current_character_id,scene.initiative_round,
            scene.initiative_turn,scene.fictional_time_tick
        ) IS DISTINCT FROM ROW(
            latest.active_character_id,latest.after_initiative_round,
            latest.after_initiative_turn,latest.after_fictional_time_tick
        )
    ) THEN
        RAISE EXCEPTION 'roleplay initiative authority postcondition failed';
    END IF;
END $$;

COMMIT;
