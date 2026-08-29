BEGIN;

LOCK TABLE roleplay_user_turns, roleplay_user_canon_completions,
    roleplay_simulation_turn_preparations, roleplay_simulation_preparation_jobs,
    roleplay_canon_events, roleplay_character_knowledge,
    roleplay_character_memories, ai_channel_messages,
    job_lifecycle_operations, jobs IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    completion_validator_source TEXT;
    lifecycle_validator_source TEXT;
BEGIN
    IF to_regprocedure(
        current_schema()||'.roleplay_user_turn_requires_canon(text,text,jsonb)'
    ) IS NOT NULL THEN
        RAISE EXCEPTION 'roleplay user canon modality authority already exists';
    END IF;

    SELECT procedure.prosrc INTO completion_validator_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema()||'.validate_roleplay_user_canon_completion()'
    );
    SELECT procedure.prosrc INTO lifecycle_validator_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema()||'.enforce_roleplay_user_canon_lifecycle_receipt()'
    );

    IF completion_validator_source IS NULL OR
       lifecycle_validator_source IS NULL OR
       position(
           'stored_contribution_kind IN (''command'',''legacy_untyped'')'
           IN completion_validator_source
       )=0 OR
       position(
           'stored_contribution_kind IN (''command'',''legacy_untyped'')'
           IN lifecycle_validator_source
       )=0 OR
       NOT EXISTS (
           SELECT 1
           FROM pg_trigger AS trigger
           JOIN pg_proc AS procedure ON procedure.oid=trigger.tgfoid
           WHERE trigger.tgrelid='roleplay_user_canon_completions'::regclass
             AND trigger.tgname='roleplay_user_canon_completions_authority'
             AND procedure.proname='validate_roleplay_user_canon_completion'
             AND NOT trigger.tgisinternal
       ) OR NOT EXISTS (
           SELECT 1
           FROM pg_trigger AS trigger
           JOIN pg_proc AS procedure ON procedure.oid=trigger.tgfoid
           WHERE trigger.tgrelid='job_lifecycle_operations'::regclass
             AND trigger.tgname='roleplay_lifecycle_requires_user_canon_receipt'
             AND procedure.proname='enforce_roleplay_user_canon_lifecycle_receipt'
             AND trigger.tgdeferrable AND trigger.tginitdeferred
             AND NOT trigger.tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_user_canon_completions'::regclass
             AND tgname='roleplay_user_canon_completions_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_user_canon_completions'::regclass
             AND tgname='roleplay_user_canon_completions_truncate_immutable'
             AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION 'inherited roleplay user canon modality authority differs';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jobs AS job
        LEFT JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.job_id=job.id
        WHERE job.status IN ('pending','running','waiting_input')
          AND (job.metadata->>'channel_mode'='roleplay' OR
               binding.job_id IS NOT NULL)
    ) THEN
        RAISE EXCEPTION
            'cannot change roleplay user canon modality while a roleplay turn is active';
    END IF;
END $$;

-- Receipts accepted before this cutover remain immutable history.  This
-- migration-only predicate proves that every such receipt is still bound to
-- the exact old lifecycle command, frozen turn, observers, and materialized
-- canon.  The function is dropped before the transaction ends and is not a
-- runtime path.
CREATE FUNCTION roleplay_historical_user_canon_receipt_exact(
    target_operation_id TEXT
)
RETURNS BOOLEAN AS $function$
    SELECT COUNT(*)=1
    FROM (
        SELECT completion.operation_id
        FROM roleplay_user_canon_completions AS completion
        JOIN roleplay_simulation_turn_preparations AS preparation
          ON preparation.operation_id=completion.preparation_id
         AND preparation.world_id=completion.world_id
         AND preparation.user_message_id=completion.source_message_id
        JOIN roleplay_user_turns AS user_turn
          ON user_turn.user_message_id=preparation.user_message_id
         AND user_turn.channel_id=preparation.channel_id
         AND user_turn.world_id=preparation.world_id
        JOIN ai_channel_messages AS message
          ON message.id=user_turn.user_message_id
         AND message.channel_id=user_turn.channel_id
         AND message.role='user' AND message.content=user_turn.exact_text
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=completion.operation_id
         AND operation.kind='complete_step'
         AND operation.command_payload ? 'roleplay_responses'
        JOIN roleplay_simulation_preparation_jobs AS binding
          ON binding.preparation_id=preparation.operation_id
         AND binding.job_id=operation.job_id
        WHERE completion.operation_id=target_operation_id
          AND user_turn.contribution_kind NOT IN ('command','legacy_untyped')
          AND completion.persona_kind=user_turn.persona_kind
          AND completion.actor_character_id IS NOT DISTINCT FROM
              user_turn.persona_character_id
          AND roleplay_transition_observers_are_exact(
              preparation.result->'participant_character_ids'
          )
          AND (user_turn.persona_kind<>'character' OR
               preparation.result->'participant_character_ids' ?
                   user_turn.persona_character_id)
          AND completion.knowledge_character_ids=CASE
              WHEN jsonb_array_length(completion.facts)=0 THEN '[]'::jsonb
              WHEN user_turn.persona_kind='character' THEN
                  jsonb_build_array(user_turn.persona_character_id)
              ELSE preparation.result->'participant_character_ids'
          END
          AND operation.command_payload->'roleplay_user_canon'=
              jsonb_build_object(
                  'facts',completion.facts,
                  'knowledge_character_ids',completion.knowledge_character_ids
              )
          AND roleplay_user_canon_materialization_exact(
              completion.operation_id
          ) IS TRUE
    ) AS exact_receipt;
$function$ LANGUAGE SQL STABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM roleplay_user_canon_completions AS completion
        WHERE roleplay_historical_user_canon_receipt_exact(
            completion.operation_id
        ) IS DISTINCT FROM TRUE
    ) OR EXISTS (
        SELECT 1
        FROM job_lifecycle_operations AS operation
        WHERE operation.command_payload ? 'roleplay_user_canon'
          AND NOT EXISTS (
              SELECT 1
              FROM roleplay_user_canon_completions AS completion
              WHERE completion.operation_id=operation.operation_id
          )
    ) THEN
        RAISE EXCEPTION
            'historical roleplay user canon receipt authority differs';
    END IF;
END $$;

CREATE FUNCTION roleplay_user_turn_requires_canon(
    persona_kind_value TEXT,
    contribution_kind_value TEXT,
    parts_value JSONB
)
RETURNS BOOLEAN AS $function$
    SELECT CASE
        WHEN jsonb_typeof(parts_value)<>'array' THEN FALSE
        WHEN persona_kind_value='character' THEN
            contribution_kind_value IN (
                'dialogue','action','action_dialogue','structured_turn'
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value='narration' AND
             jsonb_array_length(parts_value)=0 THEN TRUE
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('narration','narration_direction') THEN
            EXISTS (
                SELECT 1
                FROM jsonb_array_elements(parts_value) AS part(value)
                WHERE part.value->>'kind' IN ('action','event')
            )
        WHEN persona_kind_value='narrator' AND
             contribution_kind_value IN ('direction','command') THEN FALSE
        WHEN persona_kind_value='legacy_untyped' AND
             contribution_kind_value='legacy_untyped' THEN FALSE
        ELSE FALSE
    END;
$function$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION validate_roleplay_user_canon_completion()
RETURNS TRIGGER AS $function$
DECLARE
    frozen_participants JSONB;
    stored_persona_kind TEXT;
    stored_actor_character_id TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    expected_recipients JSONB;
BEGIN
    SELECT preparation.result->'participant_character_ids',
           user_turn.persona_kind,user_turn.persona_character_id,
           user_turn.contribution_kind,user_turn.parts
      INTO frozen_participants,stored_persona_kind,
           stored_actor_character_id,stored_contribution_kind,stored_parts
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

    IF NOT FOUND OR NOT roleplay_user_turn_requires_canon(
           stored_persona_kind,stored_contribution_kind,stored_parts
       ) OR stored_persona_kind IS DISTINCT FROM NEW.persona_kind OR
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

CREATE OR REPLACE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt()
RETURNS TRIGGER AS $function$
DECLARE
    user_canon JSONB := NEW.command_payload->'roleplay_user_canon';
    stored_persona_kind TEXT;
    stored_contribution_kind TEXT;
    stored_parts JSONB;
    stored_preparation_id TEXT;
    requires_canon BOOLEAN;
    receipt_facts JSONB;
    receipt_recipients JSONB;
    receipt_count INTEGER;
BEGIN
    IF NEW.kind<>'complete_step' THEN
        RETURN NEW;
    END IF;

    SELECT preparation.operation_id,user_turn.persona_kind,
           user_turn.contribution_kind,user_turn.parts
      INTO stored_preparation_id,stored_persona_kind,
           stored_contribution_kind,stored_parts
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
    IF stored_preparation_id IS NULL OR stored_persona_kind IS NULL OR
       stored_contribution_kind IS NULL OR stored_parts IS NULL THEN
        RAISE EXCEPTION
            'roleplay user canon lifecycle lacks frozen preparation authority';
    END IF;

    requires_canon := roleplay_user_turn_requires_canon(
        stored_persona_kind,stored_contribution_kind,stored_parts
    );
    IF NOT requires_canon THEN
        IF user_canon IS NOT NULL OR receipt_count<>0 THEN
            RAISE EXCEPTION
                'roleplay user turn without canon authority cannot carry a canon receipt';
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

DO $$
DECLARE
    helper_language TEXT;
    helper_volatility "char";
    helper_strict BOOLEAN;
    completion_validator_source TEXT;
    lifecycle_validator_source TEXT;
BEGIN
    SELECT language.lanname,procedure.provolatile,procedure.proisstrict
      INTO helper_language,helper_volatility,helper_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        current_schema()||'.roleplay_user_turn_requires_canon(text,text,jsonb)'
    );
    SELECT procedure.prosrc INTO completion_validator_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema()||'.validate_roleplay_user_canon_completion()'
    );
    SELECT procedure.prosrc INTO lifecycle_validator_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema()||'.enforce_roleplay_user_canon_lifecycle_receipt()'
    );

    IF helper_language IS DISTINCT FROM 'sql' OR
       helper_volatility IS DISTINCT FROM 'i' OR
       helper_strict IS DISTINCT FROM TRUE OR
       roleplay_user_turn_requires_canon(
           'character','dialogue','[{"kind":"message","text":"I speak."}]'::jsonb
       ) IS DISTINCT FROM TRUE OR
       roleplay_user_turn_requires_canon(
           'narrator','narration','[]'::jsonb
       ) IS DISTINCT FROM TRUE OR
       roleplay_user_turn_requires_canon(
           'narrator','narration','[{"kind":"event","text":"Rain begins."}]'::jsonb
       ) IS DISTINCT FROM TRUE OR
       roleplay_user_turn_requires_canon(
           'narrator','narration_direction',
           '[{"kind":"message","text":"Continue."},{"kind":"action","text":"The door opens."}]'::jsonb
       ) IS DISTINCT FROM TRUE OR
       roleplay_user_turn_requires_canon(
           'narrator','direction','[{"kind":"message","text":"Continue."}]'::jsonb
       ) IS DISTINCT FROM FALSE OR
       roleplay_user_turn_requires_canon(
           'narrator','command','[]'::jsonb
       ) IS DISTINCT FROM FALSE OR
       roleplay_user_turn_requires_canon(
           'legacy_untyped','legacy_untyped','[]'::jsonb
       ) IS DISTINCT FROM FALSE OR
       roleplay_user_turn_requires_canon(
           'narrator','narration_direction',
           '[{"kind":"message","text":"Only direction."}]'::jsonb
       ) IS DISTINCT FROM FALSE OR
       position('roleplay_user_turn_requires_canon' IN completion_validator_source)=0 OR
       position('roleplay_user_turn_requires_canon' IN lifecycle_validator_source)=0 OR
       position(
           'stored_contribution_kind IN (''command'',''legacy_untyped'')'
           IN completion_validator_source
       )<>0 OR
       position(
           'stored_contribution_kind IN (''command'',''legacy_untyped'')'
           IN lifecycle_validator_source
       )<>0 OR NOT EXISTS (
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
           WHERE tgrelid='roleplay_user_canon_completions'::regclass
             AND tgname='roleplay_user_canon_completions_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_user_canon_completions'::regclass
             AND tgname='roleplay_user_canon_completions_truncate_immutable'
             AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1
           FROM roleplay_user_canon_completions AS completion
           WHERE roleplay_historical_user_canon_receipt_exact(
               completion.operation_id
           ) IS DISTINCT FROM TRUE
       ) THEN
        RAISE EXCEPTION 'roleplay user canon modality authority postcondition failed';
    END IF;
END $$;

DROP FUNCTION roleplay_historical_user_canon_receipt_exact(TEXT);

COMMIT;
