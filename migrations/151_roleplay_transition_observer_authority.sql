BEGIN;

LOCK TABLE roleplay_simulation_transitions,
    roleplay_simulation_turn_preparations,
    roleplay_current_scenes,
    roleplay_scene_participants
    IN SHARE ROW EXCLUSIVE MODE;

DROP TRIGGER roleplay_simulation_transitions_immutable
    ON roleplay_simulation_transitions;

ALTER TABLE roleplay_simulation_transitions
    ADD COLUMN observer_character_ids JSONB;

CREATE FUNCTION roleplay_transition_observers_are_exact(candidate JSONB)
RETURNS BOOLEAN AS $function$
    SELECT jsonb_typeof(candidate)='array'
       AND jsonb_array_length(candidate) BETWEEN 1 AND 16
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
$function$ LANGUAGE sql IMMUTABLE STRICT;

ALTER TABLE roleplay_simulation_transitions
    ADD CONSTRAINT roleplay_simulation_transition_observers_check CHECK (
        observer_character_ids IS NULL OR
        roleplay_transition_observers_are_exact(observer_character_ids)
    );

-- Only an immutable preparation can prove who observed a historical
-- transition. Direct legacy transitions intentionally remain NULL and are
-- therefore excluded from every presence-scoped projection.
UPDATE roleplay_simulation_transitions AS transition
SET observer_character_ids=preparation.result->'participant_character_ids'
FROM roleplay_simulation_turn_preparations AS preparation
WHERE preparation.operation_id=transition.operation_id
  AND preparation.pending_transition_id=transition.operation_id
  AND preparation.world_id=transition.world_id
  AND preparation.scene_id=transition.scene_id
  AND preparation.active_character_id=transition.actor_character_id
  AND preparation.base_scene_revision=transition.before_revision
  AND preparation.scene_revision=transition.after_revision
  AND preparation.result->'pending_transition'=transition.result
  AND roleplay_transition_observers_are_exact(
      preparation.result->'participant_character_ids'
  )
  AND preparation.result->'participant_character_ids' ?
      transition.actor_character_id;

DROP TRIGGER roleplay_simulation_transitions_authority
    ON roleplay_simulation_transitions;

CREATE OR REPLACE FUNCTION validate_roleplay_simulation_transition()
RETURNS TRIGGER AS $function$
DECLARE
    current_revision BIGINT;
    current_actor TEXT;
    current_observers JSONB;
BEGIN
    SELECT scene.revision,scene.current_character_id
      INTO current_revision,current_actor
    FROM roleplay_current_scenes AS scene
    WHERE scene.world_id=NEW.world_id AND scene.id=NEW.scene_id
    FOR UPDATE;

    SELECT COALESCE(
               jsonb_agg(participant.character_id ORDER BY participant.turn_position),
               '[]'::jsonb
           )
      INTO current_observers
    FROM (
        SELECT character_id,turn_position
        FROM roleplay_scene_participants
        WHERE world_id=NEW.world_id AND scene_id=NEW.scene_id
        ORDER BY turn_position,character_id
        FOR SHARE
    ) AS participant;

    IF current_revision IS DISTINCT FROM NEW.after_revision OR
       current_actor IS DISTINCT FROM NEW.actor_character_id OR
       NEW.observer_character_ids IS NULL OR
       NOT roleplay_transition_observers_are_exact(NEW.observer_character_ids) OR
       current_observers IS DISTINCT FROM NEW.observer_character_ids OR
       NEW.result->>'schema'<>'omnidex.roleplay-simulation-transition.v1' OR
       NEW.result->>'operation_id'<>NEW.operation_id OR NEW.result->>'world_id'<>NEW.world_id OR
       NEW.result->>'scene_id'<>NEW.scene_id OR NEW.result->>'actor_character_id'<>NEW.actor_character_id OR
       (NEW.result->>'before_revision')::bigint<>NEW.before_revision OR
       (NEW.result->>'after_revision')::bigint<>NEW.after_revision OR
       NEW.result->'action'->>'kind'<>NEW.action_kind OR
       COALESCE(NEW.result->'action'->>'command_key','')<>NEW.command_key OR
       jsonb_typeof(NEW.result->'effects')<>'array' OR
       jsonb_array_length(NEW.result->'effects') NOT BETWEEN 1 AND 32 OR
       jsonb_typeof(NEW.result->'narrative_events')<>'array' OR
       jsonb_array_length(NEW.result->'narrative_events') NOT BETWEEN 1 AND 2 THEN
        RAISE EXCEPTION 'simulation transition does not match exact scene, observer, or result authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
    ) AND NOT EXISTS (
        SELECT 1
        FROM roleplay_simulation_turn_preparations AS preparation
        WHERE preparation.operation_id=NEW.operation_id
          AND preparation.pending_transition_id=NEW.operation_id
          AND preparation.world_id=NEW.world_id
          AND preparation.scene_id=NEW.scene_id
          AND preparation.active_character_id=NEW.actor_character_id
          AND preparation.base_scene_revision=NEW.before_revision
          AND preparation.scene_revision=NEW.after_revision
          AND preparation.result->'pending_transition'=NEW.result
          AND preparation.result->'participant_character_ids'=
              NEW.observer_character_ids
    ) THEN
        RAISE EXCEPTION 'simulation transition differs from its frozen preparation observer authority';
    END IF;
    RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_simulation_transitions_authority
BEFORE INSERT ON roleplay_simulation_transitions
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_simulation_transition();

CREATE INDEX idx_roleplay_transitions_observers
    ON roleplay_simulation_transitions USING GIN (observer_character_ids)
    WHERE observer_character_ids IS NOT NULL;

CREATE TRIGGER roleplay_simulation_transitions_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_transitions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema=current_schema()
          AND table_name='roleplay_simulation_transitions'
          AND column_name='observer_character_ids'
          AND is_nullable='YES'
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='roleplay_simulation_transitions'::regclass
          AND conname='roleplay_simulation_transition_observers_check'
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_simulation_transitions'::regclass
          AND tgname='roleplay_simulation_transitions_authority'
          AND NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_simulation_transitions'::regclass
          AND tgname='roleplay_simulation_transitions_immutable'
          AND NOT tgisinternal
    ) OR to_regclass(current_schema()||'.idx_roleplay_transitions_observers') IS NULL THEN
        RAISE EXCEPTION 'roleplay transition observer authority postcondition failed';
    END IF;
END $$;

COMMIT;
