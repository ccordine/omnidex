BEGIN;

LOCK TABLE roleplay_canon_events, roleplay_character_knowledge,
    roleplay_character_memories, roleplay_turn_completions,
    roleplay_simulation_turn_preparations,
    roleplay_simulation_preparation_jobs, roleplay_user_turns,
    ai_channel_messages, job_lifecycle_operations
    IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE roleplay_user_canon_completions (
    operation_id TEXT PRIMARY KEY
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    preparation_id TEXT NOT NULL UNIQUE
        REFERENCES roleplay_simulation_turn_preparations(operation_id)
        ON DELETE RESTRICT,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    source_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    persona_kind TEXT NOT NULL CHECK (persona_kind IN ('character','narrator')),
    actor_character_id TEXT,
    facts JSONB NOT NULL,
    knowledge_character_ids JSONB NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'FICTIONAL_CANON',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_user_canon_operation_check CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    CONSTRAINT roleplay_user_canon_persona_check CHECK (
        (persona_kind='character' AND actor_character_id IS NOT NULL) OR
        (persona_kind='narrator' AND actor_character_id IS NULL)
    ),
    CONSTRAINT roleplay_user_canon_facts_check CHECK (
        jsonb_typeof(facts)='array' AND jsonb_array_length(facts)<=8
    ),
    CONSTRAINT roleplay_user_canon_knowledge_check CHECK (
        jsonb_typeof(knowledge_character_ids)='array' AND
        jsonb_array_length(knowledge_character_ids)<=16 AND
        (jsonb_array_length(facts)>0 OR
         jsonb_array_length(knowledge_character_ids)=0)
    ),
    CONSTRAINT roleplay_user_canon_authority_check CHECK (
        authority_namespace='FICTIONAL_CANON'
    ),
    CONSTRAINT roleplay_user_canon_actor_fkey
        FOREIGN KEY (world_id,actor_character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT
);

CREATE FUNCTION roleplay_user_canon_character_ids_valid(candidate JSONB)
RETURNS BOOLEAN AS $function$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate)<=16
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate) AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR item.value#>>'{}' !~ '^rpc_[0-9a-f]{32}$'
       )
       AND jsonb_array_length(candidate)=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate) AS item(value)
       );
$function$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE FUNCTION validate_roleplay_user_canon_completion()
RETURNS TRIGGER AS $function$
DECLARE
    frozen_participants JSONB;
    stored_persona_kind TEXT;
    stored_actor_character_id TEXT;
    stored_contribution_kind TEXT;
    expected_recipients JSONB;
BEGIN
    SELECT preparation.result->'participant_character_ids',
           user_turn.persona_kind,user_turn.persona_character_id,
           user_turn.contribution_kind
      INTO frozen_participants,stored_persona_kind,
           stored_actor_character_id,stored_contribution_kind
    FROM roleplay_simulation_turn_preparations AS preparation
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    JOIN ai_channel_messages AS message
      ON message.id=user_turn.user_message_id
     AND message.channel_id=user_turn.channel_id
    WHERE preparation.operation_id=NEW.preparation_id
      AND preparation.world_id=NEW.world_id
      AND preparation.user_message_id=NEW.source_message_id
      AND message.role='user' AND message.content=user_turn.exact_text;

    IF NOT FOUND OR stored_contribution_kind IN ('command','legacy_untyped') OR
       stored_persona_kind IS DISTINCT FROM NEW.persona_kind OR
       stored_actor_character_id IS DISTINCT FROM NEW.actor_character_id OR
       NOT roleplay_transition_observers_are_exact(frozen_participants) OR
       (stored_persona_kind='character' AND
        NOT frozen_participants ? stored_actor_character_id) THEN
        RAISE EXCEPTION
            'roleplay user canon completion differs from frozen user-turn authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.facts) AS item(value)
        WHERE jsonb_typeof(item.value)<>'string' OR
              octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512 OR
              btrim(item.value#>>'{}')=''
    ) OR jsonb_array_length(NEW.facts)<>(
        SELECT COUNT(DISTINCT item.value#>>'{}')
        FROM jsonb_array_elements(NEW.facts) AS item(value)
    ) OR NOT roleplay_user_canon_character_ids_valid(
        NEW.knowledge_character_ids
    ) OR EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(NEW.knowledge_character_ids) AS recipient(id)
        WHERE NOT EXISTS (
            SELECT 1 FROM roleplay_characters AS character
            WHERE character.world_id=NEW.world_id
              AND character.id=recipient.id
        )
    ) THEN
        RAISE EXCEPTION 'roleplay user canon facts or recipients are invalid';
    END IF;

    expected_recipients := CASE
        WHEN jsonb_array_length(NEW.facts)=0 THEN '[]'::jsonb
        WHEN stored_persona_kind='character' THEN
            jsonb_build_array(stored_actor_character_id)
        ELSE frozen_participants
    END;
    IF NEW.knowledge_character_ids<>expected_recipients THEN
        RAISE EXCEPTION
            'roleplay user canon recipients differ from frozen observer authority';
    END IF;
    RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_user_canon_completions_authority
BEFORE INSERT ON roleplay_user_canon_completions
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_canon_completion();

CREATE FUNCTION roleplay_user_canon_payload_valid(candidate JSONB)
RETURNS BOOLEAN AS $function$
    SELECT jsonb_typeof(candidate)='object'
       AND candidate ?& ARRAY['facts','knowledge_character_ids']
       AND candidate-ARRAY['facts','knowledge_character_ids']='{}'::jsonb
       AND jsonb_typeof(candidate->'facts')='array'
       AND jsonb_array_length(candidate->'facts')<=8
       AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
            WHERE jsonb_typeof(item.value)<>'string'
               OR octet_length(item.value#>>'{}') NOT BETWEEN 1 AND 512
               OR btrim(item.value#>>'{}')=''
       )
       AND jsonb_array_length(candidate->'facts')=(
            SELECT COUNT(DISTINCT item.value#>>'{}')
            FROM jsonb_array_elements(candidate->'facts') AS item(value)
       )
       AND roleplay_user_canon_character_ids_valid(
            candidate->'knowledge_character_ids'
       )
       AND (
            jsonb_array_length(candidate->'facts')>0 OR
            jsonb_array_length(candidate->'knowledge_character_ids')=0
       );
$function$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE job_lifecycle_operations
    DROP CONSTRAINT job_lifecycle_operations_roleplay_payload_check,
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK (
        (kind='complete_step' AND
            command_payload ?& ARRAY[
                'operation_id','step_id','output','context_key','context_value'
            ] AND
            command_payload - ARRAY[
                'operation_id','step_id','output','context_key','context_value',
                'roleplay_responses','roleplay_user_canon',
                'roleplay_user_ongoing_action'
            ]='{}'::jsonb AND
            roleplay_lifecycle_response_round_valid(
                COALESCE(command_payload->'roleplay_responses','[]'::jsonb)
            ) AND (
                NOT command_payload ? 'roleplay_user_canon' OR
                roleplay_user_canon_payload_valid(
                    command_payload->'roleplay_user_canon'
                )
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

DROP TRIGGER roleplay_canon_event_source_authority ON roleplay_canon_events;

CREATE OR REPLACE FUNCTION roleplay_event_source_matches_world()
RETURNS TRIGGER AS $function$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_worlds AS world
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id
        WHERE world.id=NEW.world_id AND message.id=NEW.source_message_id
          AND (
              message.role='assistant' OR
              (message.role='user' AND EXISTS (
                  SELECT 1
                  FROM roleplay_user_canon_completions AS completion
                  WHERE completion.world_id=NEW.world_id
                    AND completion.source_message_id=message.id
                    AND completion.facts ? NEW.content
              ))
          )
    ) THEN
        RAISE EXCEPTION
            'roleplay canon event source must be an assistant message in the world channel or an exact receipt-backed user contribution';
    END IF;
    RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_canon_event_source_authority
BEFORE INSERT ON roleplay_canon_events
FOR EACH ROW EXECUTE FUNCTION roleplay_event_source_matches_world();

CREATE FUNCTION roleplay_user_canon_materialization_exact(
    target_operation_id TEXT
)
RETURNS BOOLEAN AS $function$
    WITH completion AS (
        SELECT world_id,source_message_id,facts,knowledge_character_ids
        FROM roleplay_user_canon_completions
        WHERE operation_id=target_operation_id
    ), event_projection AS (
        SELECT completion.world_id,completion.source_message_id,
               completion.facts,completion.knowledge_character_ids,
               COALESCE(
                   jsonb_agg(event.content ORDER BY event.ordinal)
                       FILTER (WHERE event.id IS NOT NULL),
                   '[]'::jsonb
               ) AS event_facts
        FROM completion
        LEFT JOIN roleplay_canon_events AS event
          ON event.world_id=completion.world_id
         AND event.source_message_id=completion.source_message_id
        GROUP BY completion.world_id,completion.source_message_id,
                 completion.facts,completion.knowledge_character_ids
    )
    SELECT COALESCE((
        SELECT projection.event_facts=projection.facts
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND (SELECT COUNT(*)
                FROM roleplay_canon_events AS event
                JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id)=
               jsonb_array_length(projection.facts)*
               jsonb_array_length(projection.knowledge_character_ids)
           AND NOT EXISTS (
                SELECT 1
                FROM roleplay_canon_events AS event
                CROSS JOIN jsonb_array_elements_text(
                    projection.knowledge_character_ids
                ) AS recipient(character_id)
                LEFT JOIN roleplay_character_knowledge AS knowledge
                  ON knowledge.world_id=event.world_id
                 AND knowledge.canon_event_id=event.id
                 AND knowledge.character_id=recipient.character_id
                LEFT JOIN roleplay_character_memories AS memory
                  ON memory.world_id=event.world_id
                 AND memory.source_event_id=event.id
                 AND memory.character_id=recipient.character_id
                WHERE event.world_id=projection.world_id
                  AND event.source_message_id=projection.source_message_id
                  AND (knowledge.id IS NULL OR memory.id IS NULL OR
                       memory.content<>event.content)
           )
        FROM event_projection AS projection
    ),FALSE);
$function$ LANGUAGE SQL STABLE STRICT;

CREATE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt()
RETURNS TRIGGER AS $function$
DECLARE
    user_canon JSONB := NEW.command_payload->'roleplay_user_canon';
    stored_contribution_kind TEXT;
    stored_preparation_id TEXT;
    receipt_facts JSONB;
    receipt_recipients JSONB;
    receipt_count INTEGER;
BEGIN
    IF NEW.kind<>'complete_step' THEN
        RETURN NEW;
    END IF;

    SELECT preparation.operation_id,user_turn.contribution_kind
      INTO stored_preparation_id,stored_contribution_kind
    FROM roleplay_simulation_preparation_jobs AS binding
    JOIN roleplay_simulation_turn_preparations AS preparation
      ON preparation.operation_id=binding.preparation_id
    JOIN roleplay_user_turns AS user_turn
      ON user_turn.user_message_id=preparation.user_message_id
     AND user_turn.channel_id=preparation.channel_id
     AND user_turn.world_id=preparation.world_id
    WHERE binding.job_id=NEW.job_id;

    SELECT COUNT(*) INTO receipt_count
    FROM roleplay_user_canon_completions AS completion
    WHERE completion.operation_id=NEW.operation_id;
    IF receipt_count=1 THEN
        SELECT completion.facts,completion.knowledge_character_ids
          INTO receipt_facts,receipt_recipients
        FROM roleplay_user_canon_completions AS completion
        WHERE completion.operation_id=NEW.operation_id;
    END IF;

    IF NOT NEW.command_payload ? 'roleplay_responses' THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'nonfictional completion cannot carry roleplay user canon';
        END IF;
        RETURN NEW;
    END IF;
    IF stored_preparation_id IS NULL OR stored_contribution_kind IS NULL THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle lacks frozen preparation authority';
    END IF;
    IF stored_contribution_kind IN ('command','legacy_untyped') THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'roleplay command cannot carry user-contribution canon';
        END IF;
        RETURN NEW;
    END IF;
    IF user_canon IS NULL OR jsonb_typeof(user_canon)<>'object' OR
       NOT user_canon ?& ARRAY['facts','knowledge_character_ids'] OR
       (user_canon-ARRAY['facts','knowledge_character_ids'])<>'{}'::jsonb OR
       receipt_count<>1 OR receipt_facts<>user_canon->'facts' OR
       receipt_recipients<>user_canon->'knowledge_character_ids' OR
       roleplay_user_canon_materialization_exact(
           NEW.operation_id
       ) IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_user_canon_completions AS completion
           WHERE completion.operation_id=NEW.operation_id
             AND completion.preparation_id=stored_preparation_id
       ) THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle receipt differs from exact command';
    END IF;
    RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_lifecycle_requires_user_canon_receipt
AFTER INSERT ON job_lifecycle_operations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt();

CREATE TRIGGER roleplay_user_canon_completions_immutable
BEFORE UPDATE OR DELETE ON roleplay_user_canon_completions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_user_canon_completions_truncate_immutable
BEFORE TRUNCATE ON roleplay_user_canon_completions
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE INDEX idx_roleplay_user_canon_world_source
    ON roleplay_user_canon_completions(world_id,source_message_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_user_canon_completions'::regclass
          AND tgname='roleplay_user_canon_completions_authority'
          AND NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='job_lifecycle_operations'::regclass
          AND tgname='roleplay_lifecycle_requires_user_canon_receipt'
          AND tgdeferrable AND tginitdeferred AND NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_canon_events'::regclass
          AND tgname='roleplay_canon_event_source_authority'
          AND NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='job_lifecycle_operations'::regclass
          AND conname='job_lifecycle_operations_roleplay_payload_check'
          AND pg_get_constraintdef(oid) LIKE '%roleplay_user_canon%'
    ) OR to_regprocedure(
        current_schema()||'.roleplay_user_canon_materialization_exact(text)'
    ) IS NULL OR EXISTS (SELECT 1 FROM roleplay_user_canon_completions) THEN
        RAISE EXCEPTION 'roleplay user canon provenance postcondition failed';
    END IF;
END $$;

COMMIT;
