BEGIN;

LOCK TABLE roleplay_turn_completions, roleplay_research_completions,
    roleplay_simulation_turn_preparations, roleplay_simulation_turn_advances,
    roleplay_simulation_transitions, ai_channel_messages,
    job_lifecycle_operations, jobs
    IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE roleplay_turn_completions
    ADD COLUMN response_position INTEGER NOT NULL DEFAULT 0,
    DROP CONSTRAINT roleplay_turn_completions_pkey,
    ADD CONSTRAINT roleplay_turn_completions_pkey
        PRIMARY KEY (operation_id,response_position),
    ADD CONSTRAINT roleplay_turn_completions_position_check
        CHECK (response_position BETWEEN 0 AND 15);

CREATE OR REPLACE FUNCTION validate_roleplay_turn_completion()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM roleplay_worlds AS world
        JOIN roleplay_characters AS character
          ON character.world_id=world.id AND character.id=NEW.viewpoint_character_id
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id AND message.id=NEW.source_message_id
        WHERE world.id=NEW.world_id AND message.role='assistant'
    ) THEN
        RAISE EXCEPTION 'roleplay response completion requires its exact world, character, and assistant source';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.facts) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              octet_length(item #>> '{}') NOT BETWEEN 1 AND 512 OR
              btrim(item #>> '{}')=''
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.facts) AS item
    ) THEN
        RAISE EXCEPTION 'roleplay response facts are invalid or duplicated';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              NOT ((item #>> '{}') ~ '^rpc_[0-9a-f]{32}$') OR
              NOT EXISTS (
                  SELECT 1 FROM roleplay_characters AS character
                  WHERE character.world_id=NEW.world_id
                    AND character.id=(item #>> '{}')
              )
    ) OR (
        SELECT COUNT(*)<>COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
    ) OR (
        jsonb_array_length(NEW.facts)>0 AND
        NEW.knowledge_character_ids<>jsonb_build_array(NEW.viewpoint_character_id)
    ) THEN
        RAISE EXCEPTION 'roleplay response knowledge must bind exactly to its responding character';
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
          AND advance.active_character_id=
              preparation.result->'responder_routes'->0->>'character_id'
          AND advance.participant_character_ids=
              preparation.result->'participant_character_ids'
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

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='roleplay_turn_completions'::regclass
          AND conname='roleplay_turn_completions_position_check'
    ) OR EXISTS (
        SELECT 1 FROM roleplay_turn_completions
        GROUP BY operation_id,response_position HAVING COUNT(*)>1
    ) THEN
        RAISE EXCEPTION 'ordered roleplay response publication postcondition failed';
    END IF;
END $$;

COMMIT;
