LOCK TABLE roleplay_interaction_commands, roleplay_characters,
    roleplay_simulation_turn_preparations, roleplay_simulation_preparation_jobs,
    roleplay_simulation_turn_advances, roleplay_turn_completions,
    roleplay_canon_events, evidence, step_completion_evidence_sets,
    job_lifecycle_operations, station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM roleplay_interaction_commands WHERE command_key='research'
    ) THEN
        RAISE EXCEPTION
            'cannot reserve /research while a fictional interaction command uses research';
    END IF;
END $$;

ALTER TABLE roleplay_interaction_commands
    DROP CONSTRAINT roleplay_interaction_commands_command_key_check,
    ADD CONSTRAINT roleplay_interaction_commands_command_key_check CHECK (
        command_key ~ '^[a-z][a-z0-9-]{0,31}$' AND
        command_key NOT IN ('give','take','research')
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
        WHEN 'roleplay_grounded_response' THEN station='conversation_response'
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

CREATE TABLE roleplay_character_capability_grants (
    grant_id TEXT PRIMARY KEY CHECK (grant_id ~ '^rpg_[0-9a-f]{32}$'),
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    character_id TEXT NOT NULL,
    capability TEXT NOT NULL CHECK (capability='web_research'),
    authority_namespace TEXT NOT NULL DEFAULT 'CODE_ISSUED_CAPABILITY'
        CHECK (authority_namespace='CODE_ISSUED_CAPABILITY'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_capability_grants_character_fkey
        FOREIGN KEY (world_id,character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    UNIQUE (grant_id,world_id,character_id,capability)
);

CREATE TABLE roleplay_character_capabilities (
    world_id TEXT NOT NULL,
    character_id TEXT NOT NULL,
    capability TEXT NOT NULL CHECK (capability='web_research'),
    grant_id TEXT NOT NULL UNIQUE,
    authority_namespace TEXT NOT NULL DEFAULT 'CODE_ISSUED_CAPABILITY'
        CHECK (authority_namespace='CODE_ISSUED_CAPABILITY'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (world_id,character_id,capability),
    CONSTRAINT roleplay_capabilities_character_fkey
        FOREIGN KEY (world_id,character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_capabilities_grant_fkey
        FOREIGN KEY (grant_id,world_id,character_id,capability)
        REFERENCES roleplay_character_capability_grants(
            grant_id,world_id,character_id,capability
        ) ON DELETE RESTRICT
);

CREATE TABLE roleplay_research_turns (
    preparation_id TEXT PRIMARY KEY
        REFERENCES roleplay_simulation_turn_preparations(operation_id) ON DELETE RESTRICT,
    channel_id TEXT NOT NULL REFERENCES ai_channels(id) ON DELETE RESTRICT,
    user_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    world_id TEXT NOT NULL REFERENCES roleplay_worlds(id) ON DELETE RESTRICT,
    scene_id TEXT NOT NULL,
    scene_revision BIGINT NOT NULL CHECK (scene_revision>=1),
    character_id TEXT NOT NULL,
    capability TEXT NOT NULL CHECK (capability='web_research'),
    capability_grant_id TEXT NOT NULL,
    question TEXT NOT NULL CHECK (
        octet_length(question) BETWEEN 1 AND 1024 AND question=btrim(question) AND
        position(E'\n' in question)=0 AND position(E'\r' in question)=0
    ),
    question_sha256 TEXT NOT NULL CHECK (question_sha256 ~ '^[0-9a-f]{64}$'),
    narrative_fingerprint TEXT NOT NULL CHECK (narrative_fingerprint ~ '^[0-9a-f]{64}$'),
    authority_namespace TEXT NOT NULL DEFAULT 'REAL_WORLD'
        CHECK (authority_namespace='REAL_WORLD'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_research_turns_scene_fkey
        FOREIGN KEY (world_id,scene_id)
        REFERENCES roleplay_current_scenes(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_research_turns_character_fkey
        FOREIGN KEY (world_id,character_id)
        REFERENCES roleplay_characters(world_id,id) ON DELETE RESTRICT,
    CONSTRAINT roleplay_research_turns_grant_fkey
        FOREIGN KEY (capability_grant_id,world_id,character_id,capability)
        REFERENCES roleplay_character_capability_grants(
            grant_id,world_id,character_id,capability
        ) ON DELETE RESTRICT
);

CREATE TABLE roleplay_research_preparation_jobs (
    preparation_id TEXT PRIMARY KEY
        REFERENCES roleplay_research_turns(preparation_id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (preparation_id,job_id),
    CONSTRAINT roleplay_research_simulation_job_fkey
        FOREIGN KEY (preparation_id,job_id)
        REFERENCES roleplay_simulation_preparation_jobs(preparation_id,job_id)
        ON DELETE RESTRICT
);

CREATE TABLE roleplay_research_completions (
    operation_id TEXT PRIMARY KEY
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    preparation_id TEXT NOT NULL UNIQUE,
    job_id BIGINT NOT NULL UNIQUE,
    source_message_id BIGINT NOT NULL UNIQUE
        REFERENCES ai_channel_messages(id) ON DELETE RESTRICT,
    rendered_sha256 TEXT NOT NULL CHECK (rendered_sha256 ~ '^[0-9a-f]{64}$'),
    authority_namespace TEXT NOT NULL DEFAULT 'REAL_WORLD'
        CHECK (authority_namespace='REAL_WORLD'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_research_completion_binding_fkey
        FOREIGN KEY (preparation_id,job_id)
        REFERENCES roleplay_research_preparation_jobs(preparation_id,job_id)
        ON DELETE RESTRICT
);

CREATE TABLE roleplay_research_completion_citations (
    operation_id TEXT NOT NULL
        REFERENCES roleplay_research_completions(operation_id) ON DELETE RESTRICT,
    completion_index INTEGER NOT NULL CHECK (completion_index BETWEEN 0 AND 3),
    evidence_id BIGINT NOT NULL UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
    capsule_id TEXT NOT NULL CHECK (
        octet_length(capsule_id) BETWEEN 1 AND 128 AND capsule_id=btrim(capsule_id)
    ),
    source_ref TEXT NOT NULL CHECK (
        octet_length(source_ref) BETWEEN 1 AND 2048 AND source_ref=btrim(source_ref)
    ),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    observed_at TIMESTAMPTZ NOT NULL,
    truncated BOOLEAN NOT NULL,
    paragraph_indexes JSONB NOT NULL CHECK (
        jsonb_typeof(paragraph_indexes)='array' AND
        jsonb_array_length(paragraph_indexes) BETWEEN 1 AND 4
    ),
    authority_namespace TEXT NOT NULL DEFAULT 'REAL_WORLD'
        CHECK (authority_namespace='REAL_WORLD'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (operation_id,completion_index)
);

CREATE INDEX idx_roleplay_capabilities_character
    ON roleplay_character_capabilities(world_id,character_id,capability);
CREATE INDEX idx_roleplay_research_turns_character
    ON roleplay_research_turns(world_id,character_id,created_at DESC);

CREATE FUNCTION validate_roleplay_research_turn()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.question_sha256<>
       encode(public.digest(convert_to(NEW.question,'UTF8'),'sha256'),'hex') OR
       NOT EXISTS (
           SELECT 1
           FROM roleplay_simulation_turn_preparations AS preparation
           JOIN ai_channel_messages AS message
             ON message.id=preparation.user_message_id
            AND message.channel_id=preparation.channel_id
            AND message.role='user'
           JOIN roleplay_worlds AS world
             ON world.id=preparation.world_id AND world.channel_id=preparation.channel_id
           JOIN ai_channels AS channel
             ON channel.id=world.channel_id AND channel.mode='roleplay'
           JOIN roleplay_character_capabilities AS capability
             ON capability.world_id=preparation.world_id
            AND capability.character_id=preparation.active_character_id
            AND capability.capability='web_research'
            AND capability.grant_id=NEW.capability_grant_id
           WHERE preparation.operation_id=NEW.preparation_id
             AND preparation.channel_id=NEW.channel_id
             AND preparation.user_message_id=NEW.user_message_id
             AND preparation.world_id=NEW.world_id
             AND preparation.scene_id=NEW.scene_id
             AND preparation.scene_revision=NEW.scene_revision
             AND preparation.active_character_id=NEW.character_id
             AND preparation.input_kind='external_command'
             AND NOT preparation.explicit_action
			 AND preparation.result->>'narrative_fingerprint'=NEW.narrative_fingerprint
             AND message.content='/research '||to_json(NEW.question)::text
       ) THEN
        RAISE EXCEPTION
            'research turn lacks exact roleplay channel, command, active-character capability, or preparation authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_research_turns_authority
BEFORE INSERT ON roleplay_research_turns
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_turn();

CREATE FUNCTION validate_roleplay_research_preparation_job()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_turns AS research
        JOIN roleplay_simulation_preparation_jobs AS simulation
          ON simulation.preparation_id=research.preparation_id
        JOIN jobs AS job ON job.id=simulation.job_id
        JOIN ai_channel_messages AS message ON message.id=research.user_message_id
        WHERE research.preparation_id=NEW.preparation_id
          AND simulation.job_id=NEW.job_id
          AND job.pipeline='chat' AND job.instruction=message.content
          AND job.metadata->>'channel_mode'='roleplay'
          AND job.metadata->>'roleplay_input_kind'='external_command'
          AND job.metadata->>'roleplay_viewpoint_character_id'=research.character_id
    ) THEN
        RAISE EXCEPTION 'research job differs from its exact command and simulation preparation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_research_preparation_jobs_authority
BEFORE INSERT ON roleplay_research_preparation_jobs
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_preparation_job();

CREATE FUNCTION validate_roleplay_research_paragraph_indexes(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='array' AND jsonb_array_length(value) BETWEEN 1 AND 4 AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_array_elements(value) AS item
               WHERE jsonb_typeof(item)<>'number' OR (item #>> '{}') !~ '^[0-3]$'
           ) AND (
               SELECT COUNT(*)=COUNT(DISTINCT item #>> '{}')
               FROM jsonb_array_elements(value) AS item
           );
$$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE roleplay_research_completion_citations
    ADD CONSTRAINT roleplay_research_citation_paragraphs_check
    CHECK (validate_roleplay_research_paragraph_indexes(paragraph_indexes));

CREATE FUNCTION validate_roleplay_research_citation()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_completions AS completion
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=completion.preparation_id
        JOIN evidence AS item ON item.id=NEW.evidence_id
        WHERE completion.operation_id=NEW.operation_id
          AND item.completion_operation_id=completion.operation_id
          AND item.completion_evidence_index=NEW.completion_index
          AND item.job_id=completion.job_id
          AND item.kind='objective_citation'
          AND item.source_type='web_document'
          AND item.source_ref=NEW.source_ref
          AND item.payload_json->>'hash'=NEW.source_sha256
          AND item.payload_json->>'source_ref'=NEW.source_ref
          AND item.payload_json#>>'{metadata,capsule_id}'=NEW.capsule_id
          AND item.payload_json#>>'{metadata,source_sha256}'=NEW.source_sha256
          AND item.payload_json#>>'{metadata,authority_namespace}'='REAL_WORLD'
          AND item.payload_json#>>'{metadata,roleplay_research_preparation_id}'=research.preparation_id
          AND item.payload_json#>>'{metadata,roleplay_research_world_id}'=research.world_id
          AND item.payload_json#>>'{metadata,roleplay_research_character_id}'=research.character_id
          AND item.payload_json#>>'{metadata,roleplay_research_question_sha256}'=research.question_sha256
          AND item.payload_json#>>'{metadata,roleplay_research_capability_grant_id}'=research.capability_grant_id
          AND item.payload_json#>'{metadata,paragraph_indexes}'=NEW.paragraph_indexes
          AND (item.payload_json#>>'{metadata,source_observed_at}')::timestamptz=NEW.observed_at
          AND (item.payload_json#>>'{metadata,source_truncated}')::boolean=NEW.truncated
    ) THEN
        RAISE EXCEPTION 'research citation differs from exact REAL_WORLD completion evidence';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_research_completion_citations_authority
BEFORE INSERT ON roleplay_research_completion_citations
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_citation();

CREATE FUNCTION validate_roleplay_research_completion()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roleplay_research_preparation_jobs AS binding
        JOIN roleplay_research_turns AS research
          ON research.preparation_id=binding.preparation_id
        JOIN roleplay_character_capabilities AS capability
          ON capability.grant_id=research.capability_grant_id
         AND capability.world_id=research.world_id
         AND capability.character_id=research.character_id
         AND capability.capability=research.capability
        JOIN ai_channel_messages AS message
          ON message.id=NEW.source_message_id AND message.channel_id=research.channel_id
         AND message.role='assistant'
        JOIN roleplay_simulation_turn_advances AS advance
          ON advance.preparation_id=research.preparation_id AND advance.job_id=binding.job_id
        JOIN step_completion_evidence_sets AS evidence_set
          ON evidence_set.operation_id=NEW.operation_id AND evidence_set.job_id=binding.job_id
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=evidence_set.operation_id
         AND operation.kind='complete_step'
         AND operation.command_payload->>'context_key'='objective_result'
        WHERE binding.preparation_id=NEW.preparation_id AND binding.job_id=NEW.job_id
          AND encode(public.digest(convert_to(message.content,'UTF8'),'sha256'),'hex')=NEW.rendered_sha256
          AND evidence_set.evidence_count BETWEEN 1 AND 4
          AND objective_completion_evidence_set_is_valid(NEW.operation_id)
    ) OR EXISTS (
        SELECT 1 FROM roleplay_turn_completions AS fictional
        WHERE fictional.operation_id=NEW.operation_id OR fictional.source_message_id=NEW.source_message_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_canon_events AS event
        WHERE event.source_message_id=NEW.source_message_id
    ) OR (
        SELECT COUNT(*) FROM roleplay_research_completion_citations AS citation
        WHERE citation.operation_id=NEW.operation_id
    )<>(
        SELECT evidence_count FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'research completion lacks exact message, citations, active capability, turn advance, or REAL_WORLD isolation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER roleplay_research_completions_authority
AFTER INSERT ON roleplay_research_completions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_research_completion();

CREATE FUNCTION reject_fictional_completion_for_research()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS evidence_set
        JOIN roleplay_research_preparation_jobs AS research
          ON research.job_id=evidence_set.job_id
        WHERE evidence_set.operation_id=NEW.operation_id
    ) OR EXISTS (
        SELECT 1 FROM roleplay_research_completions
        WHERE operation_id=NEW.operation_id OR source_message_id=NEW.source_message_id
    ) THEN
        RAISE EXCEPTION 'REAL_WORLD research completion cannot be materialized as fictional canon';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_turn_completions_research_isolation
BEFORE INSERT ON roleplay_turn_completions
FOR EACH ROW EXECUTE FUNCTION reject_fictional_completion_for_research();

CREATE FUNCTION reject_research_authority_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% research authority is immutable',TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roleplay_capability_grants_immutable
BEFORE UPDATE OR DELETE ON roleplay_character_capability_grants
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_capability_grants_no_truncate
BEFORE TRUNCATE ON roleplay_character_capability_grants
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_capabilities_update_immutable
BEFORE UPDATE ON roleplay_character_capabilities
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_capabilities_no_truncate
BEFORE TRUNCATE ON roleplay_character_capabilities
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_turns_immutable
BEFORE UPDATE OR DELETE ON roleplay_research_turns
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_turns_no_truncate
BEFORE TRUNCATE ON roleplay_research_turns
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_preparation_jobs_immutable
BEFORE UPDATE OR DELETE ON roleplay_research_preparation_jobs
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_preparation_jobs_no_truncate
BEFORE TRUNCATE ON roleplay_research_preparation_jobs
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_completions_immutable
BEFORE UPDATE OR DELETE ON roleplay_research_completions
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_completions_no_truncate
BEFORE TRUNCATE ON roleplay_research_completions
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_citations_immutable
BEFORE UPDATE OR DELETE ON roleplay_research_completion_citations
FOR EACH ROW EXECUTE FUNCTION reject_research_authority_mutation();
CREATE TRIGGER roleplay_research_citations_no_truncate
BEFORE TRUNCATE ON roleplay_research_completion_citations
FOR EACH STATEMENT EXECUTE FUNCTION reject_research_authority_mutation();

DO $$
BEGIN
    IF NOT station_owns_portable_work(
           'conversation_response','roleplay_grounded_response','{}'::jsonb
       ) OR station_owns_portable_work(
           'web_grounded_synthesis','roleplay_grounded_response','{}'::jsonb
       ) OR to_regclass(current_schema()||'.roleplay_character_capabilities') IS NULL OR
       to_regclass(current_schema()||'.roleplay_research_turns') IS NULL OR
       to_regclass(current_schema()||'.roleplay_research_completions') IS NULL OR
       EXISTS (SELECT 1 FROM roleplay_interaction_commands WHERE command_key='research') OR
       NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='roleplay_interaction_commands'::regclass
             AND conname='roleplay_interaction_commands_command_key_check'
             AND pg_get_constraintdef(oid) LIKE '%research%'
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_research_completions'::regclass
             AND tgname='roleplay_research_completions_authority' AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='roleplay_turn_completions'::regclass
             AND tgname='roleplay_turn_completions_research_isolation' AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION 'roleplay research authority postcondition failed';
    END IF;
END $$;
