LOCK TABLE roleplay_character_library, roleplay_characters,
    roleplay_simulation_turn_preparations, roleplay_simulation_preparation_jobs,
    jobs IN SHARE ROW EXCLUSIVE MODE;

LOCK TABLE station_call_openings IN ACCESS EXCLUSIVE MODE;

ALTER TABLE station_call_openings
    DROP CONSTRAINT station_call_openings_tokenizer_profile_check;

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_tokenizer_profile_check CHECK (
        tokenizer_profile IN (
            'ollama-0.24.0-qwen35-gpt2-boundary-v1',
            'ollama-0.24.0-qwen3-qwen2-boundary-v1',
            'ollama-0.24.0-qwen2-qwen2-bos-boundary-v1',
            'ollama-0.24.0-mistral3-gpt2-bos-boundary-v1',
            'ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1',
            'ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1',
            'ollama-0.24.0-gemma3-llama-default-boundary-v1',
            'ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1',
            'ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1',
            'ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1',
            'ollama-0.24.0-qwen2-llama-default-code-boundary-v1',
            'ollama-0.24.0-gemma-llama-default-fim-boundary-v1',
            'ollama-0.24.0-gemma-llama-default-chat-boundary-v1',
            'ollama-0.24.0-llama-llama-default-code-boundary-v1',
            'ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1',
            'ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1',
            'ollama-0.24.0-roleplay-raw-completion-v1'
        )
    );

CREATE TABLE roleplay_character_generation_configs (
    library_character_id TEXT PRIMARY KEY
        REFERENCES roleplay_character_library(id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    narrative_model TEXT NOT NULL DEFAULT '',
    voice_rewrite_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    voice_rewrite_model TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roleplay_character_generation_model_check CHECK (
        octet_length(narrative_model) <= 256 AND narrative_model=btrim(narrative_model) AND
        (narrative_model='' OR narrative_model ~ '^[A-Za-z0-9._:/@-]+$') AND
        octet_length(voice_rewrite_model) <= 256 AND voice_rewrite_model=btrim(voice_rewrite_model) AND
        (voice_rewrite_model='' OR voice_rewrite_model ~ '^[A-Za-z0-9._:/@-]+$')
    ),
    CONSTRAINT roleplay_character_generation_voice_check CHECK (
        (voice_rewrite_enabled AND voice_rewrite_model<>'') OR
        (NOT voice_rewrite_enabled AND voice_rewrite_model='')
    )
);

INSERT INTO roleplay_character_generation_configs (library_character_id)
SELECT id FROM roleplay_character_library ORDER BY id;

CREATE FUNCTION initialize_roleplay_character_generation_config()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO roleplay_character_generation_configs (library_character_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_character_library_generation_config
AFTER INSERT ON roleplay_character_library
FOR EACH ROW EXECUTE FUNCTION initialize_roleplay_character_generation_config();

CREATE FUNCTION validate_roleplay_character_generation_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.library_character_id IS DISTINCT FROM OLD.library_character_id OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.revision<>OLD.revision+1 OR NEW.updated_at<OLD.updated_at THEN
        RAISE EXCEPTION 'roleplay character generation identity is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER roleplay_character_generation_update_guard
BEFORE UPDATE ON roleplay_character_generation_configs
FOR EACH ROW EXECUTE FUNCTION validate_roleplay_character_generation_update();
CREATE TRIGGER roleplay_character_generation_delete_rejected
BEFORE DELETE ON roleplay_character_generation_configs
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_simulation_state_delete();
CREATE TRIGGER roleplay_character_generation_truncate_rejected
BEFORE TRUNCATE ON roleplay_character_generation_configs
FOR EACH STATEMENT EXECUTE FUNCTION reject_roleplay_simulation_state_delete();

DROP TRIGGER roleplay_simulation_preparations_immutable
    ON roleplay_simulation_turn_preparations;
UPDATE roleplay_simulation_turn_preparations AS preparation
SET result=jsonb_set(
    preparation.result,'{generation_config}',jsonb_build_object(
        'schema','omnidex.roleplay-character-generation.v1',
        'library_character_id',config.library_character_id,
        'revision',config.revision,
        'narrative_model',config.narrative_model,
        'voice_rewrite_enabled',config.voice_rewrite_enabled,
        'voice_rewrite_model',config.voice_rewrite_model
    ),TRUE
)
FROM roleplay_characters AS character
JOIN roleplay_character_generation_configs AS config
  ON config.library_character_id=character.library_character_id
WHERE preparation.world_id=character.world_id
  AND preparation.active_character_id=character.id;
CREATE TRIGGER roleplay_simulation_preparations_immutable
BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation();

ALTER TABLE roleplay_simulation_turn_preparations
    DROP CONSTRAINT roleplay_simulation_turn_preparations_result_check,
    ADD CONSTRAINT roleplay_simulation_turn_preparations_result_check CHECK (
        jsonb_typeof(result)='object' AND octet_length(result::text) <= 131072 AND
        result ?& ARRAY['preparation_id','channel_id','user_message_id','world_id','scene_id',
            'base_scene_revision','scene_revision','active_character_id','input_kind','explicit_action',
            'participant_character_ids','generation_config','narrative_projection','narrative_authority',
            'narrative_fingerprint','created_at'] AND
        jsonb_typeof(result->'participant_character_ids')='array' AND
        jsonb_typeof(result->'generation_config')='object' AND
        jsonb_typeof(result->'narrative_projection')='object' AND
        jsonb_typeof(result->'narrative_authority')='object'
    );

UPDATE jobs AS job
SET metadata=jsonb_set(
    job.metadata,'{roleplay_generation_config}',preparation.result->'generation_config',TRUE
)
FROM roleplay_simulation_preparation_jobs AS binding
JOIN roleplay_simulation_turn_preparations AS preparation
  ON preparation.operation_id=binding.preparation_id
WHERE job.id=binding.job_id;

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
            'roleplay_generation_config','roleplay_simulation_preparation_id','roleplay_world_id',
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
        WHEN 'roleplay_voice_rewrite' THEN station='roleplay_voice_rewrite'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'database_schema_selection' THEN station='database_schema_selection'
        WHEN 'database_query_intent' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'
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

DO $$
BEGIN
    IF NOT station_owns_portable_work(
           'roleplay_voice_rewrite','roleplay_voice_rewrite','{}'::jsonb
       ) OR station_owns_portable_work(
           'conversation_response','roleplay_voice_rewrite','{}'::jsonb
       ) OR EXISTS (
           SELECT 1 FROM roleplay_character_library AS library
           LEFT JOIN roleplay_character_generation_configs AS config
             ON config.library_character_id=library.id
           WHERE config.library_character_id IS NULL
       ) OR EXISTS (
           SELECT 1 FROM roleplay_simulation_turn_preparations
           WHERE NOT result ? 'generation_config'
       ) THEN
        RAISE EXCEPTION 'roleplay character generation authority postcondition failed';
    END IF;
END $$;
