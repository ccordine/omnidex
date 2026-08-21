BEGIN;

LOCK TABLE roleplay_user_turns, roleplay_simulation_turn_preparations,
    roleplay_simulation_preparation_jobs, jobs, roleplay_characters,
    roleplay_character_profiles, roleplay_current_scenes
    IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND metadata->>'channel_mode'='roleplay'
    ) THEN
        RAISE EXCEPTION 'cannot change roleplay user-turn authority while a roleplay turn is active';
    END IF;
END $$;

DROP TRIGGER roleplay_user_turns_immutable ON roleplay_user_turns;
DROP TRIGGER roleplay_user_turns_validate_insert ON roleplay_user_turns;
DROP TRIGGER roleplay_simulation_preparations_immutable
    ON roleplay_simulation_turn_preparations;
DROP TRIGGER jobs_chat_turn_binding_immutable ON jobs;

ALTER TABLE roleplay_user_turns
    DROP CONSTRAINT roleplay_user_turns_persona_contribution_check,
    DROP CONSTRAINT roleplay_user_turns_authority_check,
    DROP COLUMN authority,
    ADD COLUMN parts JSONB NOT NULL DEFAULT '[]'::jsonb;

DROP FUNCTION roleplay_user_turn_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT);

CREATE FUNCTION roleplay_user_turn_parts_valid(
    parts_value JSONB,
    persona_kind_value TEXT,
    contribution_kind_value TEXT,
    exact_text_value TEXT
)
RETURNS BOOLEAN AS $$
DECLARE
    part JSONB;
    rendered TEXT;
    message_count INTEGER := 0;
    action_count INTEGER := 0;
    event_count INTEGER := 0;
    part_count INTEGER;
BEGIN
    IF jsonb_typeof(parts_value)<>'array' THEN
        RETURN FALSE;
    END IF;
    part_count := jsonb_array_length(parts_value);
    IF part_count=0 THEN
        RETURN contribution_kind_value IN (
            'dialogue','action','action_dialogue','narration','direction',
            'command','legacy_untyped'
        );
    END IF;
    IF part_count>16 OR contribution_kind_value IN ('command','legacy_untyped') THEN
        RETURN FALSE;
    END IF;
    FOR part IN
        SELECT item.value FROM jsonb_array_elements(parts_value) AS item(value)
    LOOP
        IF jsonb_typeof(part)<>'object' OR
           NOT part ?& ARRAY['kind','text'] OR
           (part - ARRAY['kind','text'])<>'{}'::jsonb OR
           jsonb_typeof(part->'kind')<>'string' OR
           jsonb_typeof(part->'text')<>'string' OR
           part->>'kind' NOT IN ('message','action','event') OR
           octet_length(part->>'text') NOT BETWEEN 1 AND 4096 OR
           btrim(part->>'text')='' THEN
            RETURN FALSE;
        END IF;
        message_count := message_count + (part->>'kind'='message')::integer;
        action_count := action_count + (part->>'kind'='action')::integer;
        event_count := event_count + (part->>'kind'='event')::integer;
    END LOOP;
    SELECT string_agg(
        CASE item.value->>'kind'
            WHEN 'message' THEN '[Message]' || E'\n' || (item.value->>'text')
            WHEN 'action' THEN '[Action]' || E'\n' || (item.value->>'text')
            ELSE '[Event]' || E'\n' || (item.value->>'text')
        END,
        E'\n\n' ORDER BY item.ordinal
    ) INTO rendered
    FROM jsonb_array_elements(parts_value) WITH ORDINALITY AS item(value,ordinal);
    IF rendered<>exact_text_value OR octet_length(rendered)>4096 THEN
        RETURN FALSE;
    END IF;
    IF persona_kind_value='character' THEN
        RETURN contribution_kind_value=CASE
            WHEN event_count>0 THEN 'structured_turn'
            WHEN message_count>0 AND action_count>0 THEN 'action_dialogue'
            WHEN message_count>0 THEN 'dialogue'
            ELSE 'action'
        END;
    END IF;
    IF persona_kind_value='narrator' THEN
        RETURN contribution_kind_value=CASE
            WHEN message_count>0 AND action_count+event_count>0 THEN 'narration_direction'
            WHEN message_count>0 THEN 'direction'
            ELSE 'narration'
        END;
    END IF;
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION roleplay_user_turn_authority(
    persona_kind_value TEXT,
    persona_character_id_value TEXT,
    persona_name_value TEXT,
    persona_summary_value TEXT,
    contribution_kind_value TEXT,
    exact_text_value TEXT,
    parts_value JSONB
)
RETURNS JSONB AS $$
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'persona_kind',persona_kind_value,
        'character_id',persona_character_id_value,
        'persona_name',persona_name_value,
        'persona_summary',CASE WHEN persona_kind_value='character'
            THEN persona_summary_value ELSE NULL END,
        'contribution_kind',contribution_kind_value,
        'parts',parts_value,
        'exact_text',exact_text_value
    ));
$$ LANGUAGE SQL IMMUTABLE;

ALTER TABLE roleplay_user_turns
    ADD COLUMN authority JSONB GENERATED ALWAYS AS (
        roleplay_user_turn_authority(
            persona_kind,persona_character_id,persona_name,persona_summary,
            contribution_kind,exact_text,parts
        )
    ) STORED,
    ADD CONSTRAINT roleplay_user_turns_persona_contribution_check CHECK (
        (persona_kind='character' AND persona_character_id IS NOT NULL AND
         persona_name NOT IN ('Narrator','Unattributed user') AND
         contribution_kind IN (
             'dialogue','action','action_dialogue','structured_turn'
         )) OR
        (persona_kind='narrator' AND persona_character_id IS NULL AND
         persona_name='Narrator' AND persona_summary='' AND
         contribution_kind IN (
             'narration','direction','narration_direction','command'
         )) OR
        (persona_kind='legacy_untyped' AND persona_character_id IS NULL AND
         persona_name='Unattributed user' AND persona_summary='' AND
         contribution_kind='legacy_untyped')
    ),
    ADD CONSTRAINT roleplay_user_turns_parts_check CHECK (
        roleplay_user_turn_parts_valid(
            parts,persona_kind,contribution_kind,exact_text
        )
    ),
    ADD CONSTRAINT roleplay_user_turns_authority_check CHECK (
        jsonb_typeof(authority)='object' AND octet_length(authority::text)<=16384 AND
        authority ?& ARRAY[
            'persona_kind','persona_name','contribution_kind','parts','exact_text'
        ]
    );

CREATE OR REPLACE FUNCTION validate_roleplay_user_turn_insert()
RETURNS TRIGGER AS $$
BEGIN
    IF jsonb_typeof(NEW.parts)<>'array' THEN
        RAISE EXCEPTION 'roleplay user-turn parts must be one ordered JSON array';
    END IF;
    IF jsonb_array_length(NEW.parts)=0 AND NEW.contribution_kind<>'command' THEN
        RAISE EXCEPTION 'new roleplay prose turns require ordered message, action, or event parts';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM ai_channel_messages AS message
        JOIN ai_channels AS channel ON channel.id=message.channel_id
        JOIN roleplay_worlds AS world ON world.channel_id=channel.id
        WHERE message.id=NEW.user_message_id AND message.role='user'
          AND message.channel_id=NEW.channel_id AND message.content=NEW.exact_text
          AND channel.mode='roleplay' AND world.id=NEW.world_id
    ) THEN
        RAISE EXCEPTION 'roleplay user turn does not match its exact message, channel, or world';
    END IF;
    IF NEW.persona_kind='legacy_untyped' THEN
        RAISE EXCEPTION 'new roleplay turns require explicit persona and contribution authority';
    ELSIF NEW.persona_kind='narrator' AND NOT EXISTS (
        SELECT 1 FROM roleplay_current_scenes WHERE world_id=NEW.world_id
    ) THEN
        RAISE EXCEPTION 'narrator turn requires a current scene';
    ELSIF NEW.persona_kind='character' AND NOT EXISTS (
        SELECT 1 FROM roleplay_characters AS character
        LEFT JOIN roleplay_character_profiles AS profile
          ON profile.library_character_id=character.library_character_id
        WHERE character.world_id=NEW.world_id AND character.id=NEW.persona_character_id
          AND character.name=NEW.persona_name
          AND COALESCE(profile.summary,'')=NEW.persona_summary
    ) THEN
        RAISE EXCEPTION 'selected user persona must be an exact character in the current world';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_user_turns_validate_insert
BEFORE INSERT ON roleplay_user_turns
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_turn_insert();

UPDATE roleplay_simulation_turn_preparations AS preparation
SET result=jsonb_set(preparation.result,'{user_turn}',turn_authority.authority,TRUE)
FROM roleplay_user_turns AS turn_authority
WHERE turn_authority.user_message_id=preparation.user_message_id
  AND turn_authority.channel_id=preparation.channel_id;

UPDATE jobs AS job
SET metadata=jsonb_set(job.metadata,'{roleplay_user_turn}',preparation.result->'user_turn',TRUE)
FROM roleplay_simulation_preparation_jobs AS binding
JOIN roleplay_simulation_turn_preparations AS preparation
  ON preparation.operation_id=binding.preparation_id
WHERE job.id=binding.job_id;

CREATE TRIGGER roleplay_user_turns_immutable
BEFORE UPDATE OR DELETE ON roleplay_user_turns
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_simulation_preparations_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER jobs_chat_turn_binding_immutable
BEFORE UPDATE OF pipeline,metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN roleplay_user_turns AS turn_authority
          ON turn_authority.user_message_id=preparation.user_message_id
        WHERE preparation.result->'user_turn' IS DISTINCT FROM turn_authority.authority
    ) OR EXISTS (
        SELECT 1 FROM roleplay_user_turns
        WHERE jsonb_typeof(parts)<>'array' OR NOT authority ? 'parts'
    ) THEN
        RAISE EXCEPTION 'structured roleplay user-turn authority postcondition failed';
    END IF;
END $$;

COMMIT;
