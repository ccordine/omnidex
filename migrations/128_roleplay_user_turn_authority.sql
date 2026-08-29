BEGIN;

LOCK TABLE ai_channels, ai_channel_messages, jobs, roleplay_worlds,
    roleplay_characters, roleplay_character_profiles, roleplay_current_scenes,
    roleplay_scene_participants, roleplay_simulation_turn_preparations,
    roleplay_simulation_preparation_jobs IN SHARE ROW EXCLUSIVE MODE;

CREATE FUNCTION roleplay_user_turn_authority(
    persona_kind_value TEXT,
    persona_character_id_value TEXT,
    persona_name_value TEXT,
    persona_summary_value TEXT,
    contribution_kind_value TEXT,
    exact_text_value TEXT
)
RETURNS JSONB AS $$
    SELECT CASE persona_kind_value
        WHEN 'character' THEN jsonb_build_object(
            'persona_kind',persona_kind_value,
            'character_id',persona_character_id_value,
            'persona_name',persona_name_value,
            'persona_summary',persona_summary_value,
            'contribution_kind',contribution_kind_value,
            'exact_text',exact_text_value
        )
        ELSE jsonb_build_object(
            'persona_kind',persona_kind_value,
            'persona_name',persona_name_value,
            'contribution_kind',contribution_kind_value,
            'exact_text',exact_text_value
        )
    END;
$$ LANGUAGE SQL IMMUTABLE;

CREATE TABLE roleplay_user_turns (
    user_message_id BIGINT PRIMARY KEY
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    channel_id TEXT NOT NULL REFERENCES ai_channels(id) ON DELETE RESTRICT,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    persona_kind TEXT NOT NULL CHECK (
        persona_kind IN ('character','narrator','legacy_untyped')
    ),
    persona_character_id TEXT,
    persona_name TEXT NOT NULL CHECK (
        octet_length(persona_name) BETWEEN 1 AND 256 AND
        persona_name=btrim(persona_name)
    ),
    persona_summary TEXT NOT NULL DEFAULT '' CHECK (
        octet_length(persona_summary) <= 1024 AND
        persona_summary=btrim(persona_summary)
    ),
    contribution_kind TEXT NOT NULL CHECK (
        contribution_kind IN (
            'dialogue','action','action_dialogue','narration','direction',
            'command','legacy_untyped'
        )
    ),
    exact_text TEXT NOT NULL CHECK (
        octet_length(exact_text) BETWEEN 1 AND 4096 AND btrim(exact_text)<>''
    ),
    authority JSONB GENERATED ALWAYS AS (
        roleplay_user_turn_authority(
            persona_kind,persona_character_id,persona_name,persona_summary,
            contribution_kind,exact_text
        )
    ) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_user_turns_character_fkey
        FOREIGN KEY (world_id,persona_character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_user_turns_persona_contribution_check CHECK (
        (
            persona_kind='character' AND persona_character_id IS NOT NULL AND
            persona_name<>'Narrator' AND persona_name<>'Unattributed user' AND
            persona_summary<>'' AND
            contribution_kind IN ('dialogue','action','action_dialogue')
        ) OR (
            persona_kind='narrator' AND persona_character_id IS NULL AND
            persona_name='Narrator' AND persona_summary='' AND
            contribution_kind IN ('narration','direction','command')
        ) OR (
            persona_kind='legacy_untyped' AND persona_character_id IS NULL AND
            persona_name='Unattributed user' AND persona_summary='' AND
            contribution_kind='legacy_untyped'
        )
    ),
    CONSTRAINT roleplay_user_turns_command_text_check CHECK (
        (contribution_kind='command' AND left(exact_text,1)='/') OR
        (contribution_kind<>'command' AND
            (contribution_kind='legacy_untyped' OR left(exact_text,1)<>'/'))
    ),
    CONSTRAINT roleplay_user_turns_authority_check CHECK (
        jsonb_typeof(authority)='object' AND octet_length(authority::text) <= 8192 AND
        authority ?& ARRAY[
            'persona_kind','persona_name','contribution_kind','exact_text'
        ]
    )
);

CREATE INDEX idx_roleplay_user_turns_channel_message
    ON roleplay_user_turns(channel_id,user_message_id DESC);
CREATE INDEX idx_roleplay_user_turns_world_character
    ON roleplay_user_turns(world_id,persona_character_id,user_message_id DESC);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM ai_channel_messages AS message
        JOIN ai_channels AS channel ON channel.id=message.channel_id
        LEFT JOIN roleplay_worlds AS world ON world.channel_id=channel.id
        WHERE channel.mode='roleplay' AND message.role='user' AND world.id IS NULL
    ) THEN
        RAISE EXCEPTION 'roleplay user message has no world authority';
    END IF;
END $$;

INSERT INTO roleplay_user_turns (
    user_message_id,channel_id,world_id,persona_kind,persona_name,
    contribution_kind,exact_text,created_at
)
SELECT message.id,message.channel_id,world.id,'legacy_untyped',
       'Unattributed user','legacy_untyped',message.content,message.created_at
FROM ai_channel_messages AS message
JOIN ai_channels AS channel ON channel.id=message.channel_id
JOIN roleplay_worlds AS world ON world.channel_id=channel.id
WHERE channel.mode='roleplay' AND message.role='user'
ORDER BY message.id;

CREATE FUNCTION validate_roleplay_user_turn_insert()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM ai_channel_messages AS message
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
    ELSIF NEW.persona_kind='narrator' THEN
        IF NOT EXISTS (
            SELECT 1 FROM roleplay_current_scenes
            WHERE world_id=NEW.world_id
        ) THEN
            RAISE EXCEPTION 'narrator turn requires a current scene';
        END IF;
    ELSIF NEW.persona_kind='character' THEN
        IF NOT EXISTS (
            SELECT 1
            FROM roleplay_current_scenes AS scene
            JOIN roleplay_scene_participants AS participant
              ON participant.world_id=scene.world_id AND participant.scene_id=scene.id
             AND participant.character_id=NEW.persona_character_id
            JOIN roleplay_characters AS character
              ON character.world_id=scene.world_id AND character.id=participant.character_id
            JOIN roleplay_character_profiles AS profile
              ON profile.library_character_id=character.library_character_id
            WHERE scene.world_id=NEW.world_id
              AND character.id<>scene.current_character_id
              AND character.name=NEW.persona_name
              AND profile.summary=NEW.persona_summary
        ) THEN
            RAISE EXCEPTION 'selected user persona must be a current participant distinct from the responding character';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_user_turns_validate_insert
BEFORE INSERT ON roleplay_user_turns
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_user_turn_insert();

CREATE FUNCTION require_roleplay_user_turn()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.role='user' AND EXISTS (
        SELECT 1 FROM ai_channels WHERE id=NEW.channel_id AND mode='roleplay'
    ) AND NOT EXISTS (
        SELECT 1 FROM roleplay_user_turns
        WHERE user_message_id=NEW.id AND channel_id=NEW.channel_id
          AND exact_text=NEW.content
    ) THEN
        RAISE EXCEPTION 'roleplay user message requires explicit turn authority in the same transaction';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER ai_channel_messages_require_roleplay_user_turn
AFTER INSERT ON ai_channel_messages DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_roleplay_user_turn();

CREATE TRIGGER roleplay_user_turns_immutable
BEFORE UPDATE OR DELETE ON roleplay_user_turns
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();
CREATE TRIGGER roleplay_user_turns_truncate_immutable
BEFORE TRUNCATE ON roleplay_user_turns
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

DROP TRIGGER roleplay_simulation_preparations_immutable
    ON roleplay_simulation_turn_preparations;
UPDATE roleplay_simulation_turn_preparations AS preparation
SET result=jsonb_set(preparation.result,'{user_turn}',turn_authority.authority,TRUE)
FROM roleplay_user_turns AS turn_authority
WHERE turn_authority.user_message_id=preparation.user_message_id
  AND turn_authority.channel_id=preparation.channel_id
  AND turn_authority.world_id=preparation.world_id;
CREATE TRIGGER roleplay_simulation_preparations_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

ALTER TABLE roleplay_simulation_turn_preparations
    DROP CONSTRAINT roleplay_simulation_turn_preparations_result_check,
    ADD CONSTRAINT roleplay_simulation_turn_preparations_result_check CHECK (
        jsonb_typeof(result)='object' AND octet_length(result::text) <= 131072 AND
        result ?& ARRAY['preparation_id','channel_id','user_message_id','world_id','scene_id',
            'base_scene_revision','scene_revision','active_character_id','user_turn','input_kind',
            'explicit_action','participant_character_ids','generation_config','narrative_projection',
            'narrative_authority','narrative_fingerprint','created_at'] AND
        jsonb_typeof(result->'user_turn')='object' AND
        jsonb_typeof(result->'participant_character_ids')='array' AND
        jsonb_typeof(result->'generation_config')='object' AND
        jsonb_typeof(result->'narrative_projection')='object' AND
        jsonb_typeof(result->'narrative_authority')='object'
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
        JOIN roleplay_scene_participants AS participant
          ON participant.world_id=world.id AND participant.scene_id=NEW.scene_id
         AND participant.character_id=NEW.active_character_id
        JOIN roleplay_current_scenes AS scene
          ON scene.world_id=world.id AND scene.id=NEW.scene_id
         AND scene.revision=NEW.base_scene_revision
         AND scene.current_character_id=NEW.active_character_id
        WHERE world.id=NEW.world_id AND channel.id=NEW.channel_id AND channel.mode='roleplay'
          AND message.id=NEW.user_message_id AND message.role='user'
          AND message.content=user_turn.exact_text
          AND NEW.result->'user_turn'=user_turn.authority
          AND (
              (NEW.input_kind='prose' AND user_turn.contribution_kind<>'command') OR
              (NEW.input_kind<>'prose' AND user_turn.contribution_kind='command')
          )
          AND (
              user_turn.persona_character_id IS NULL OR
              user_turn.persona_character_id<>NEW.active_character_id
          )
    ) OR NEW.result->>'preparation_id'<>NEW.operation_id OR
       NEW.result->>'channel_id'<>NEW.channel_id OR
       (NEW.result->>'user_message_id')::bigint<>NEW.user_message_id OR
       NEW.result->>'world_id'<>NEW.world_id OR NEW.result->>'scene_id'<>NEW.scene_id OR
       (NEW.result->>'base_scene_revision')::bigint<>NEW.base_scene_revision OR
       (NEW.result->>'scene_revision')::bigint<>NEW.scene_revision OR
       NEW.result->>'active_character_id'<>NEW.active_character_id OR
       NEW.result->>'input_kind'<>NEW.input_kind OR
       (NEW.result->>'explicit_action')::boolean<>NEW.explicit_action OR
       COALESCE(NEW.result->'pending_transition'->>'operation_id','')<>COALESCE(NEW.pending_transition_id,'') OR
       (NEW.pending_transition_id IS NOT NULL AND (
          NEW.result->'pending_transition'->>'world_id'<>NEW.world_id OR
          NEW.result->'pending_transition'->>'scene_id'<>NEW.scene_id OR
          NEW.result->'pending_transition'->>'actor_character_id'<>NEW.active_character_id OR
          (NEW.result->'pending_transition'->>'before_revision')::bigint<>NEW.base_scene_revision OR
          (NEW.result->'pending_transition'->>'after_revision')::bigint<>NEW.scene_revision
       )) THEN
        RAISE EXCEPTION 'simulation preparation does not match exact user turn, channel, message, scene, or result authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

UPDATE jobs AS job
SET metadata=jsonb_set(
    job.metadata,'{roleplay_user_turn}',preparation.result->'user_turn',TRUE
)
FROM roleplay_simulation_preparation_jobs AS binding
JOIN roleplay_simulation_turn_preparations AS preparation
  ON preparation.operation_id=binding.preparation_id
WHERE job.id=binding.job_id;

CREATE OR REPLACE FUNCTION validate_roleplay_preparation_job()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
        JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
        JOIN jobs AS job ON job.id=NEW.job_id
        WHERE preparation.operation_id=NEW.preparation_id AND job.pipeline='chat'
          AND job.instruction=message.content AND job.metadata->>'channel_id'=preparation.channel_id
          AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
          AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
          AND job.metadata->>'roleplay_world_id'=preparation.world_id
          AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
          AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
          AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
          AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
          AND job.metadata->>'roleplay_viewpoint_character_id'=preparation.active_character_id
          AND job.metadata->'roleplay_participant_character_ids'=
              preparation.result->'participant_character_ids'
          AND job.metadata->'roleplay_generation_config'=preparation.result->'generation_config'
          AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
    ) THEN
        RAISE EXCEPTION 'simulation job does not match its exact preparation, user turn, and message';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_chat_turn_binding_update()
RETURNS TRIGGER AS $$
DECLARE
    binding_key TEXT;
BEGIN
    IF OLD.pipeline='chat' OR NEW.pipeline='chat' THEN
        IF NEW.pipeline IS DISTINCT FROM OLD.pipeline THEN
            RAISE EXCEPTION 'chat turn pipeline authority is immutable';
        END IF;
        FOREACH binding_key IN ARRAY ARRAY[
            'channel_id','channel_user_message_id','project_id','client_cwd',
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config',
            'roleplay_generation_config','roleplay_user_turn',
            'roleplay_simulation_preparation_id','roleplay_world_id',
            'roleplay_scene_id','roleplay_scene_revision','roleplay_input_kind',
            'roleplay_participant_character_ids','roleplay_narrative_fingerprint'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable', binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    roleplay_user_messages BIGINT;
    roleplay_user_turn_count BIGINT;
    user_turn_trigger_count INTEGER;
    message_requirement_trigger_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO roleplay_user_messages
    FROM ai_channel_messages AS message
    JOIN ai_channels AS channel ON channel.id=message.channel_id
    WHERE channel.mode='roleplay' AND message.role='user';

    SELECT COUNT(*) INTO roleplay_user_turn_count FROM roleplay_user_turns;

    SELECT COUNT(*) INTO user_turn_trigger_count
    FROM pg_trigger
    WHERE tgrelid='roleplay_user_turns'::regclass AND NOT tgisinternal
      AND tgname IN (
          'roleplay_user_turns_validate_insert','roleplay_user_turns_immutable',
          'roleplay_user_turns_truncate_immutable'
      ) AND tgenabled='O';

    SELECT COUNT(*) INTO message_requirement_trigger_count
    FROM pg_trigger
    WHERE tgrelid='ai_channel_messages'::regclass AND NOT tgisinternal
      AND tgname='ai_channel_messages_require_roleplay_user_turn'
      AND tgenabled='O';

    IF roleplay_user_messages<>roleplay_user_turn_count OR
       user_turn_trigger_count<>3 OR message_requirement_trigger_count<>1 OR
       EXISTS (
           SELECT 1 FROM roleplay_user_turns AS turn_authority
           JOIN ai_channel_messages AS message ON message.id=turn_authority.user_message_id
           JOIN ai_channels AS channel ON channel.id=message.channel_id
           WHERE channel.mode<>'roleplay' OR message.role<>'user' OR
                 message.channel_id<>turn_authority.channel_id OR
                 message.content<>turn_authority.exact_text
       ) OR EXISTS (
           SELECT 1 FROM roleplay_simulation_turn_preparations AS preparation
           LEFT JOIN roleplay_user_turns AS turn_authority
             ON turn_authority.user_message_id=preparation.user_message_id
            AND turn_authority.channel_id=preparation.channel_id
            AND turn_authority.world_id=preparation.world_id
           WHERE turn_authority.user_message_id IS NULL OR
                 preparation.result->'user_turn' IS DISTINCT FROM turn_authority.authority
       ) OR EXISTS (
           SELECT 1 FROM roleplay_simulation_preparation_jobs AS binding
           JOIN roleplay_simulation_turn_preparations AS preparation
             ON preparation.operation_id=binding.preparation_id
           JOIN jobs AS job ON job.id=binding.job_id
           WHERE job.metadata->'roleplay_user_turn' IS DISTINCT FROM preparation.result->'user_turn'
       ) THEN
        RAISE EXCEPTION 'roleplay user turn authority postcondition failed';
    END IF;
END $$;

COMMIT;
