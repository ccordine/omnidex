LOCK TABLE ai_channels, ai_channel_messages, jobs IN ACCESS EXCLUSIVE MODE;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid='job_lifecycle_operations'::regclass
      AND contype='c'
      AND pg_get_constraintdef(oid) LIKE '%command_payload%context_key%context_value%'
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format(
            'ALTER TABLE job_lifecycle_operations DROP CONSTRAINT %I',
            constraint_name
        );
    END IF;
END $$;

ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_roleplay_payload_check CHECK (
        (kind = 'complete_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'output', 'context_key', 'context_value'] AND
            command_payload - ARRAY[
                'operation_id', 'step_id', 'output', 'context_key', 'context_value',
                'roleplay_facts', 'roleplay_knowledge_character_ids'
            ] = '{}'::jsonb AND
            jsonb_typeof(COALESCE(command_payload->'roleplay_facts','[]'::jsonb))='array' AND
            jsonb_array_length(COALESCE(command_payload->'roleplay_facts','[]'::jsonb)) <= 8 AND
            jsonb_typeof(COALESCE(
                command_payload->'roleplay_knowledge_character_ids','[]'::jsonb
            ))='array' AND
            jsonb_array_length(COALESCE(
                command_payload->'roleplay_knowledge_character_ids','[]'::jsonb
            )) <= 16 AND
            (
                jsonb_array_length(COALESCE(
                    command_payload->'roleplay_facts','[]'::jsonb
                )) > 0 OR
                jsonb_array_length(COALESCE(
                    command_payload->'roleplay_knowledge_character_ids','[]'::jsonb
                )) = 0
            )) OR
        (kind = 'fail_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'error'] AND
            command_payload - ARRAY['operation_id', 'step_id', 'error'] = '{}'::jsonb) OR
        (kind IN ('submit_feedback', 'replan_job') AND
            command_payload ?& ARRAY['operation_id', 'job_id', 'feedback'] AND
            command_payload - ARRAY['operation_id', 'job_id', 'feedback'] = '{}'::jsonb) OR
        (kind = 'cancel_job' AND
            command_payload ?& ARRAY['operation_id', 'job_id', 'reason'] AND
            command_payload - ARRAY['operation_id', 'job_id', 'reason'] = '{}'::jsonb)
    );

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
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'memory_context_selection' THEN station='memory_context_selection'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_canon_extraction' THEN station='roleplay_canon_extraction'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'database_schema_selection' THEN station='database_schema_selection'
        WHEN 'database_query_intent' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN
            station='web_grounded_synthesis_correction'
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

CREATE TABLE roleplay_worlds (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL UNIQUE REFERENCES ai_channels(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'FICTIONAL_CANON',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_worlds_identity_check CHECK (
        id ~ '^rpw_[0-9a-f]{32}$'
    ),
    CONSTRAINT roleplay_worlds_name_check CHECK (
        octet_length(name) BETWEEN 1 AND 256 AND name=btrim(name)
    ),
    CONSTRAINT roleplay_worlds_authority_check CHECK (
        authority_namespace='FICTIONAL_CANON'
    )
);

CREATE TABLE roleplay_characters (
    id TEXT PRIMARY KEY,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'FICTIONAL_CANON',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_characters_identity_check CHECK (
        id ~ '^rpc_[0-9a-f]{32}$'
    ),
    CONSTRAINT roleplay_characters_name_check CHECK (
        octet_length(name) BETWEEN 1 AND 256 AND name=btrim(name)
    ),
    CONSTRAINT roleplay_characters_authority_check CHECK (
        authority_namespace='FICTIONAL_CANON'
    ),
    UNIQUE (world_id, id)
);

CREATE TABLE roleplay_canon_events (
    id TEXT PRIMARY KEY,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    source_message_id BIGINT NOT NULL REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    ordinal BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    content TEXT NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'FICTIONAL_CANON',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_canon_events_identity_check CHECK (
        id ~ '^rpe_[0-9a-f]{32}$'
    ),
    CONSTRAINT roleplay_canon_events_content_check CHECK (
        octet_length(content) BETWEEN 1 AND 512 AND btrim(content)<>''
    ),
    CONSTRAINT roleplay_canon_events_authority_check CHECK (
        authority_namespace='FICTIONAL_CANON'
    ),
    UNIQUE (world_id, content),
    UNIQUE (world_id, id)
);

CREATE TABLE roleplay_character_knowledge (
    id TEXT PRIMARY KEY,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    character_id TEXT NOT NULL,
    canon_event_id TEXT NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'CHARACTER_KNOWLEDGE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_knowledge_identity_check CHECK (
        id ~ '^rpk_[0-9a-f]{32}$'
    ),
    CONSTRAINT roleplay_character_knowledge_authority_check CHECK (
        authority_namespace='CHARACTER_KNOWLEDGE'
    ),
    CONSTRAINT roleplay_character_knowledge_character_fkey
        FOREIGN KEY (world_id, character_id)
        REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_character_knowledge_event_fkey
        FOREIGN KEY (world_id, canon_event_id)
        REFERENCES roleplay_canon_events(world_id, id) ON DELETE RESTRICT,
    UNIQUE (character_id, canon_event_id)
);

CREATE TABLE roleplay_turn_completions (
    operation_id TEXT PRIMARY KEY
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    viewpoint_character_id TEXT NOT NULL,
    source_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    facts JSONB NOT NULL,
    knowledge_character_ids JSONB NOT NULL,
    authority_namespace TEXT NOT NULL DEFAULT 'FICTIONAL_CANON',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_turn_completions_operation_check CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    CONSTRAINT roleplay_turn_completions_facts_check CHECK (
        jsonb_typeof(facts)='array' AND jsonb_array_length(facts) <= 8
    ),
    CONSTRAINT roleplay_turn_completions_knowledge_check CHECK (
        jsonb_typeof(knowledge_character_ids)='array' AND
        jsonb_array_length(knowledge_character_ids) <= 16 AND
        (jsonb_array_length(facts) > 0 OR jsonb_array_length(knowledge_character_ids)=0)
    ),
    CONSTRAINT roleplay_turn_completions_authority_check CHECK (
        authority_namespace='FICTIONAL_CANON'
    ),
    CONSTRAINT roleplay_turn_completions_viewpoint_fkey
        FOREIGN KEY (world_id, viewpoint_character_id)
        REFERENCES roleplay_characters(world_id, id) ON DELETE RESTRICT
);

ALTER TABLE ai_channels
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'assistant',
    ADD COLUMN roleplay_viewpoint_character_id TEXT,
    ADD CONSTRAINT ai_channels_mode_check CHECK (
        mode IN ('assistant','roleplay')
    ),
    ADD CONSTRAINT ai_channels_roleplay_binding_check CHECK (
        (mode='assistant' AND roleplay_viewpoint_character_id IS NULL) OR
        (mode='roleplay' AND roleplay_viewpoint_character_id IS NOT NULL)
    ),
    ADD CONSTRAINT ai_channels_roleplay_data_source_isolation_check CHECK (
        mode='assistant' OR data_source_id IS NULL
    ),
    ADD CONSTRAINT ai_channels_roleplay_viewpoint_fkey
        FOREIGN KEY (roleplay_viewpoint_character_id)
        REFERENCES roleplay_characters(id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED;

UPDATE jobs
SET metadata=jsonb_set(
    metadata,
    '{channel_mode}',
    to_jsonb('assistant'::text),
    true
)
WHERE pipeline='chat'
  AND metadata ? 'channel_id'
  AND NOT metadata ? 'channel_mode';

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
            'data_source_id','channel_mode','roleplay_viewpoint_character_id','model_config'
        ] LOOP
            IF NEW.metadata->binding_key IS DISTINCT FROM OLD.metadata->binding_key THEN
                RAISE EXCEPTION 'chat turn binding authority % is immutable', binding_key;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_chat_turn_binding_immutable
BEFORE UPDATE OF pipeline,metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();

CREATE INDEX idx_roleplay_characters_world
    ON roleplay_characters(world_id, id);

CREATE INDEX idx_roleplay_canon_events_world_ordinal
    ON roleplay_canon_events(world_id, ordinal DESC, id DESC);

CREATE INDEX idx_roleplay_character_knowledge_projection
    ON roleplay_character_knowledge(character_id, canon_event_id);

CREATE OR REPLACE FUNCTION roleplay_world_requires_roleplay_channel()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM ai_channels
        WHERE id=NEW.channel_id AND mode='roleplay'
    ) THEN
        RAISE EXCEPTION 'roleplay world requires an explicitly typed roleplay channel';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_world_channel_authority
BEFORE INSERT ON roleplay_worlds
FOR EACH ROW EXECUTE FUNCTION roleplay_world_requires_roleplay_channel();

CREATE OR REPLACE FUNCTION enforce_roleplay_channel_viewpoint()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.mode='roleplay' AND NOT EXISTS (
        SELECT 1
        FROM roleplay_characters AS character
        JOIN roleplay_worlds AS world ON world.id=character.world_id
        WHERE character.id=NEW.roleplay_viewpoint_character_id
          AND world.channel_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'roleplay viewpoint must belong to the channel fictional world';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER ai_channels_roleplay_viewpoint_authority
AFTER INSERT OR UPDATE ON ai_channels
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION enforce_roleplay_channel_viewpoint();

CREATE OR REPLACE FUNCTION reject_roleplay_channel_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.mode IS DISTINCT FROM OLD.mode OR
       NEW.roleplay_viewpoint_character_id IS DISTINCT FROM OLD.roleplay_viewpoint_character_id THEN
        RAISE EXCEPTION 'channel mode and roleplay viewpoint binding are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ai_channels_roleplay_binding_immutable
BEFORE UPDATE ON ai_channels
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_channel_binding_update();

CREATE OR REPLACE FUNCTION reject_roleplay_identity_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME='roleplay_worlds' AND (
        NEW.id IS DISTINCT FROM OLD.id OR NEW.channel_id IS DISTINCT FROM OLD.channel_id OR
        NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace
    ) THEN
        RAISE EXCEPTION 'roleplay world identity binding is immutable';
    END IF;
    IF TG_TABLE_NAME='roleplay_characters' AND (
        NEW.id IS DISTINCT FROM OLD.id OR NEW.world_id IS DISTINCT FROM OLD.world_id OR
        NEW.authority_namespace IS DISTINCT FROM OLD.authority_namespace
    ) THEN
        RAISE EXCEPTION 'roleplay character identity binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_worlds_binding_immutable
BEFORE UPDATE ON roleplay_worlds
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_identity_binding_update();

CREATE TRIGGER roleplay_characters_binding_immutable
BEFORE UPDATE ON roleplay_characters
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_identity_binding_update();

CREATE OR REPLACE FUNCTION roleplay_event_source_matches_world()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_worlds AS world
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id
        WHERE world.id=NEW.world_id AND message.id=NEW.source_message_id
          AND message.role='assistant'
    ) THEN
        RAISE EXCEPTION
            'roleplay canon event source must be an assistant message in the world channel';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_canon_event_source_authority
BEFORE INSERT ON roleplay_canon_events
FOR EACH ROW EXECUTE FUNCTION roleplay_event_source_matches_world();

CREATE OR REPLACE FUNCTION validate_roleplay_turn_completion()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_worlds AS world
        JOIN roleplay_characters AS character
          ON character.world_id=world.id
         AND character.id=NEW.viewpoint_character_id
        JOIN ai_channel_messages AS message
          ON message.channel_id=world.channel_id
         AND message.id=NEW.source_message_id
        WHERE world.id=NEW.world_id AND message.role='assistant'
    ) THEN
        RAISE EXCEPTION
            'roleplay turn completion requires its exact world viewpoint and assistant source';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.facts) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              octet_length(item #>> '{}') NOT BETWEEN 1 AND 512 OR
              btrim(item #>> '{}')=''
    ) OR (
        SELECT COUNT(*) <> COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.facts) AS item
    ) THEN
        RAISE EXCEPTION 'roleplay turn completion facts are invalid or duplicated';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
        WHERE jsonb_typeof(item)<>'string' OR
              NOT ((item #>> '{}') ~ '^rpc_[0-9a-f]{32}$') OR
              NOT EXISTS (
                  SELECT 1 FROM roleplay_characters AS character
                  WHERE character.world_id=NEW.world_id
                    AND character.id=(item #>> '{}')
              )
    ) OR (
        SELECT COUNT(*) <> COUNT(DISTINCT item #>> '{}')
        FROM jsonb_array_elements(NEW.knowledge_character_ids) AS item
    ) THEN
        RAISE EXCEPTION 'roleplay turn completion knowledge recipients are invalid or duplicated';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_turn_completions_authority
BEFORE INSERT ON roleplay_turn_completions
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_turn_completion();

CREATE OR REPLACE FUNCTION reject_roleplay_append_authority_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% authority is immutable and append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_canon_events_immutable
BEFORE UPDATE OR DELETE ON roleplay_canon_events
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_canon_events_truncate_immutable
BEFORE TRUNCATE ON roleplay_canon_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_character_knowledge_immutable
BEFORE UPDATE OR DELETE ON roleplay_character_knowledge
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_character_knowledge_truncate_immutable
BEFORE TRUNCATE ON roleplay_character_knowledge
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_turn_completions_immutable
BEFORE UPDATE OR DELETE ON roleplay_turn_completions
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

CREATE TRIGGER roleplay_turn_completions_truncate_immutable
BEFORE TRUNCATE ON roleplay_turn_completions
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

DO $$
BEGIN
    IF NOT station_owns_portable_work(
           'roleplay_canon_extraction','roleplay_canon_extraction','{}'::jsonb
       ) OR station_owns_portable_work(
           'conversation_response','roleplay_canon_extraction','{}'::jsonb
       ) OR to_regclass(current_schema() || '.roleplay_worlds') IS NULL OR
       to_regclass(current_schema() || '.roleplay_characters') IS NULL OR
       to_regclass(current_schema() || '.roleplay_canon_events') IS NULL OR
       to_regclass(current_schema() || '.roleplay_character_knowledge') IS NULL OR
       to_regclass(current_schema() || '.roleplay_turn_completions') IS NULL OR
       EXISTS (
           SELECT 1 FROM ai_channels
           WHERE mode NOT IN ('assistant','roleplay') OR
                 mode='assistant' AND roleplay_viewpoint_character_id IS NOT NULL OR
                 mode='roleplay' AND (
                     roleplay_viewpoint_character_id IS NULL OR data_source_id IS NOT NULL
                 )
       ) OR EXISTS (
           SELECT 1 FROM jobs
           WHERE pipeline='chat' AND metadata ? 'channel_id' AND NOT metadata ? 'channel_mode'
       ) OR
       NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='job_lifecycle_operations'::regclass
             AND conname='job_lifecycle_operations_roleplay_payload_check'
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='jobs'::regclass
             AND tgname='jobs_chat_turn_binding_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_canon_events'::regclass
             AND tgname='roleplay_canon_events_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_turn_completions'::regclass
             AND tgname='roleplay_turn_completions_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_turn_completions'::regclass
             AND tgname='roleplay_turn_completions_truncate_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_character_knowledge'::regclass
             AND tgname='roleplay_character_knowledge_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='ai_channels'::regclass
             AND tgname='ai_channels_roleplay_viewpoint_authority'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_canon_events'::regclass
             AND tgname='roleplay_canon_events_truncate_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_character_knowledge'::regclass
             AND tgname='roleplay_character_knowledge_truncate_immutable'
             AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION 'roleplay canon authority postcondition failed';
    END IF;
END $$;
