BEGIN;

LOCK TABLE roleplay_user_turns, roleplay_current_scenes,
    roleplay_scene_participants, roleplay_characters,
    roleplay_character_profiles, roleplay_simulation_turn_preparations
    IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM roleplay_user_turns AS user_turn
        WHERE user_turn.persona_kind='character' AND NOT EXISTS (
            SELECT 1
            FROM roleplay_simulation_turn_preparations AS preparation
            WHERE preparation.channel_id=user_turn.channel_id
              AND preparation.user_message_id=user_turn.user_message_id
              AND preparation.world_id=user_turn.world_id
              AND preparation.result->'user_turn'=user_turn.authority
              AND preparation.result->'participant_character_ids' ?
                  user_turn.persona_character_id
        )
    ) THEN
        RAISE EXCEPTION 'cannot install user-persona scene authority over a retained character turn without exact prepared scene authority';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION validate_roleplay_user_turn_insert()
RETURNS TRIGGER AS $function$
DECLARE
    current_scene_id TEXT;
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

    SELECT scene.id
      INTO current_scene_id
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id
    FOR SHARE;

    IF current_scene_id IS NULL THEN
        RAISE EXCEPTION 'roleplay user turn requires a current scene';
    END IF;
    IF NEW.persona_kind='legacy_untyped' THEN
        RAISE EXCEPTION 'new roleplay turns require explicit persona and contribution authority';
    ELSIF NEW.persona_kind='character' AND NOT EXISTS (
            SELECT 1
            FROM roleplay_scene_participants AS participant
            JOIN roleplay_characters AS character
              ON character.world_id=participant.world_id
             AND character.id=participant.character_id
            JOIN roleplay_character_profiles AS profile
              ON profile.library_character_id=character.library_character_id
            WHERE participant.world_id=NEW.world_id
              AND participant.scene_id=current_scene_id
              AND participant.character_id=NEW.persona_character_id
              AND character.name=NEW.persona_name
              AND profile.summary=NEW.persona_summary
        ) THEN
        RAISE EXCEPTION 'selected user persona must be a current scene participant';
    END IF;
    RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

COMMIT;
