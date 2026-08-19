LOCK TABLE roleplay_simulation_turn_preparations,
    roleplay_simulation_preparation_jobs,
    roleplay_simulation_transitions,
    roleplay_simulation_turn_advances,
    roleplay_turn_completions,
    roleplay_research_completions,
    ai_channel_messages,
    job_lifecycle_operations,
    jobs
    IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE roleplay_simulation_turn_preparations
    ADD CONSTRAINT roleplay_simulation_pending_transition_identity_check
    CHECK (pending_transition_id IS NULL OR pending_transition_id=operation_id);

CREATE UNIQUE INDEX idx_roleplay_simulation_pending_transition
    ON roleplay_simulation_turn_preparations(pending_transition_id)
    WHERE pending_transition_id IS NOT NULL;

CREATE FUNCTION roleplay_terminal_simulation_publication_valid(
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
          AND advance.participant_character_ids=preparation.result->'participant_character_ids'
          AND job.pipeline='chat'
          AND job.status='completed'
          AND job.result=operation.command_payload->>'output'
          AND operation.command_payload->>'context_key'='objective_result'
          AND (
              (preparation.pending_transition_id IS NULL AND NOT EXISTS (
                  SELECT 1
                  FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.operation_id
              )) OR
              (preparation.pending_transition_id=preparation.operation_id AND EXISTS (
                  SELECT 1
                  FROM roleplay_simulation_transitions AS transition
                  WHERE transition.operation_id=preparation.pending_transition_id
                    AND transition.world_id=preparation.world_id
                    AND transition.scene_id=preparation.scene_id
                    AND transition.actor_character_id=preparation.active_character_id
                    AND transition.before_revision=preparation.base_scene_revision
                    AND transition.after_revision=preparation.scene_revision
                    AND transition.result=preparation.result->'pending_transition'
              ))
          )
          AND num_nonnulls(
              (
                  SELECT fictional.operation_id
                  FROM roleplay_turn_completions AS fictional
                  JOIN ai_channel_messages AS message
                    ON message.id=fictional.source_message_id
                  WHERE fictional.operation_id=operation.operation_id
                    AND fictional.world_id=preparation.world_id
                    AND fictional.viewpoint_character_id=preparation.active_character_id
                    AND fictional.authority_namespace='FICTIONAL_CANON'
                    AND message.channel_id=preparation.channel_id
                    AND message.role='assistant'
                    AND message.content=operation.command_payload->>'output'
              ),
              (
                  SELECT real_world.operation_id
                  FROM roleplay_research_completions AS real_world
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
          )=1
    ) AS exact_terminal_publication;
$$ LANGUAGE SQL STABLE;

CREATE FUNCTION require_terminal_roleplay_prepared_transition()
RETURNS TRIGGER AS $$
DECLARE
    preparation_count INTEGER;
    target_preparation_id TEXT;
BEGIN
    SELECT COUNT(*),MIN(preparation.operation_id)
    INTO preparation_count,target_preparation_id
    FROM roleplay_simulation_turn_preparations AS preparation
    WHERE preparation.operation_id=NEW.operation_id OR
          preparation.pending_transition_id=NEW.operation_id;

    IF preparation_count=0 THEN
        RETURN NEW;
    END IF;
    IF preparation_count<>1 OR target_preparation_id<>NEW.operation_id OR
       NOT roleplay_terminal_simulation_publication_valid(target_preparation_id,NULL) THEN
        RAISE EXCEPTION
            'prepared simulation transition requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_prepared_transitions_require_terminal_completion
AFTER INSERT ON roleplay_simulation_transitions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_prepared_transition();

CREATE FUNCTION require_terminal_roleplay_turn_advance()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT roleplay_terminal_simulation_publication_valid(
        NEW.preparation_id,
        NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'simulation turn advance requires one exact terminal roleplay completion';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_turn_advances_require_terminal_completion
AFTER INSERT ON roleplay_simulation_turn_advances
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_terminal_roleplay_turn_advance();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='roleplay_simulation_turn_preparations'::regclass
          AND conname='roleplay_simulation_pending_transition_identity_check'
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_simulation_transitions'::regclass
          AND tgname='roleplay_prepared_transitions_require_terminal_completion'
          AND NOT tgisinternal
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid='roleplay_simulation_turn_advances'::regclass
          AND tgname='roleplay_turn_advances_require_terminal_completion'
          AND NOT tgisinternal
    ) THEN
        RAISE EXCEPTION 'terminal roleplay simulation publication postcondition failed';
    END IF;
END $$;
