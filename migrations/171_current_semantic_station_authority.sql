BEGIN;

LOCK TABLE station_gap_openings, station_gap_outcomes, projects, jobs,
    generated_workload_deployments IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    station_source TEXT;
    guard_source TEXT;
    station_language TEXT;
    guard_language TEXT;
    station_volatility "char";
    guard_volatility "char";
    station_strict BOOLEAN;
    guard_strict BOOLEAN;
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO station_source,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO guard_source,guard_language,guard_volatility,guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    IF station_source IS NULL OR station_language<>'sql' OR
       station_volatility<>'i' OR NOT station_strict OR
       encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex')<>
       '1550583d1bcf608601ba4001c7bedbbaf507baf10c76e814fcc1521edebfd206' THEN
        RAISE EXCEPTION
            'current semantic station authority requires the exact prior station function';
    END IF;
    IF guard_source IS NULL OR guard_language<>'plpgsql' OR
       guard_volatility<>'v' OR guard_strict IS DISTINCT FROM FALSE OR
       encode(digest(convert_to(guard_source,'UTF8'),'sha256'),'hex')<>
       '5ee88ea6498bba2a89b1339a1f259d71dace780bcd9ae2ff89a66039900df1e7' THEN
        RAISE EXCEPTION
            'current semantic station authority requires the exact prior opening guard';
    END IF;
END $$;

CREATE FUNCTION current_model_config_is_valid(config JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(config)='object' AND NOT EXISTS (
        SELECT 1
        FROM jsonb_each(CASE
            WHEN jsonb_typeof(config)='object' THEN config
            ELSE '{}'::jsonb
        END) AS field(key,value)
        WHERE field.key NOT IN (
            'context_relevance_model',
            'context_minification_model',
            'conversation_objective_kind_model',
            'conversation_response_model',
            'roleplay_semantic_model',
            'grounded_answer_model',
            'database_schema_selection_model',
            'database_query_intent_model',
            'database_evidence_gap_model',
            'database_join_path_selection_model',
            'repository_evidence_relevance_model',
            'web_relevance_model',
            'web_grounded_synthesis_model',
            'coding_surface_model',
            'coding_requirements_model',
            'coding_service_deployment_intent_model',
            'coding_workload_model',
            'coding_artifact_handling_model',
            'coding_capability_relation_model',
            'coding_skill_selection_model',
            'coding_fragment_model',
            'coding_fragment_repair_guidance_model',
            'coding_fragment_correction_model',
            'coding_repository_change_surface_model'
        ) OR jsonb_typeof(field.value)<>'string' OR
             field.value #>> '{}'='' OR
             btrim(field.value #>> '{}')<>field.value #>> '{}'
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM projects
        WHERE settings ? 'model_config' AND
              current_model_config_is_valid(settings->'model_config')
                  IS DISTINCT FROM TRUE
    ) OR EXISTS (
        SELECT 1 FROM jobs
        WHERE metadata ? 'model_config' AND
              current_model_config_is_valid(metadata->'model_config')
                  IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'semantic station retirement requires a fresh reset: non-current model routing exists';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE pipeline IN ('coding','scrum') AND
              status IN ('pending','running','waiting_input')
    ) THEN
        RAISE EXCEPTION
            'cannot retire semantic stations while a coding job is active';
    END IF;
    IF EXISTS (
        SELECT 1 FROM generated_workload_deployments
        WHERE status IN ('prepared','applying','indeterminate')
    ) THEN
        RAISE EXCEPTION
            'cannot retire semantic stations while a deployment is nonterminal';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION station_owns_portable_work(
    station TEXT, work_kind TEXT, payload JSONB
)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_context_need_coverage' THEN station='coding_requirements'
        WHEN 'application_context_need_question' THEN station='coding_requirements'
        WHEN 'application_product_context' THEN station='coding_requirements'
        WHEN 'application_requirement_coverage' THEN station='coding_requirements'
        WHEN 'application_requirement' THEN station='coding_requirements'
        WHEN 'repository_requirement_coverage' THEN station='coding_requirements'
        WHEN 'repository_requirement' THEN station='coding_requirements'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'
        WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'repository_change_owner' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance_leaf' THEN station='repository_evidence_relevance'
        WHEN 'context_relevance_selection' THEN station='context_relevance'
        WHEN 'context_minification' THEN station='context_minification'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response_text' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response_evidence_relation' THEN station='conversation_response'
        WHEN 'roleplay_canon_fact_coverage' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_canon_fact' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_ongoing_action' THEN station='roleplay_ongoing_action'
        WHEN 'grounded_answer_text' THEN station='grounded_answer'
        WHEN 'grounded_answer_evidence_relation' THEN station='grounded_answer'
        WHEN 'database_schema_selection_coverage' THEN station='database_schema_selection'
        WHEN 'database_schema_relation_selection' THEN station='database_schema_selection'
        WHEN 'database_query_from_relation' THEN station='database_query_intent'
        WHEN 'database_query_shape' THEN station='database_query_intent'
        WHEN 'database_query_projection_coverage' THEN station='database_query_intent'
        WHEN 'database_query_projection_aggregate' THEN station='database_query_intent'
        WHEN 'database_query_projection_field' THEN station='database_query_intent'
        WHEN 'database_query_projection_time_bucket' THEN station='database_query_intent'
        WHEN 'database_query_filter_coverage' THEN station='database_query_intent'
        WHEN 'database_query_filter_field' THEN station='database_query_intent'
        WHEN 'database_query_filter_operator' THEN station='database_query_intent'
        WHEN 'database_query_filter_value_coverage' THEN station='database_query_intent'
        WHEN 'database_query_filter_value' THEN station='database_query_intent'
        WHEN 'database_query_window_coverage' THEN station='database_query_intent'
        WHEN 'database_query_window_field' THEN station='database_query_intent'
        WHEN 'database_query_window_unit' THEN station='database_query_intent'
        WHEN 'database_query_window_amount' THEN station='database_query_intent'
        WHEN 'database_query_existence_coverage' THEN station='database_query_intent'
        WHEN 'database_query_existence_relation' THEN station='database_query_intent'
        WHEN 'database_query_existence_negated' THEN station='database_query_intent'
        WHEN 'database_query_having_coverage' THEN station='database_query_intent'
        WHEN 'database_query_having_aggregate' THEN station='database_query_intent'
        WHEN 'database_query_having_field' THEN station='database_query_intent'
        WHEN 'database_query_having_operator' THEN station='database_query_intent'
        WHEN 'database_query_having_value' THEN station='database_query_intent'
        WHEN 'database_query_order_coverage' THEN station='database_query_intent'
        WHEN 'database_query_order_projection' THEN station='database_query_intent'
        WHEN 'database_query_order_direction' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_relevance_relation' THEN station='web_relevance'
        WHEN 'web_synthesis_paragraph_coverage' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_paragraph' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_evidence_relation' THEN station='web_grounded_synthesis'
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
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM station_gap_openings
        WHERE station_owns_portable_work(
            station,work_kind,portable_payload::jsonb
        ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'semantic station retirement requires a fresh reset: retired opening state exists';
    END IF;
END $$;

ALTER TABLE projects
    ADD CONSTRAINT projects_current_model_config CHECK (
        NOT (settings ? 'model_config') OR
        current_model_config_is_valid(settings->'model_config')
    );

ALTER TABLE jobs
    ADD CONSTRAINT jobs_current_model_config CHECK (
        NOT (metadata ? 'model_config') OR
        current_model_config_is_valid(metadata->'model_config')
    );

DROP TRIGGER station_gap_openings_enforce_context_sieve_insert
    ON station_gap_openings;
DROP FUNCTION enforce_context_sieve_station_opening_insert();

DO $$
DECLARE
    station_source TEXT;
    model_config_source TEXT;
BEGIN
    SELECT procedure.prosrc INTO station_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT procedure.prosrc INTO model_config_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'current_model_config_is_valid(jsonb)'
    );
    IF encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex')<>
       '7697634f9396160a6cc0d6091ac6cbee9bd8a1e553be040420b28fd6138c8167' OR
       encode(digest(convert_to(model_config_source,'UTF8'),'sha256'),'hex')<>
       '68f9663f6a6765335c458dfef4d53ea3ad9b4875d751101428f7a61dccd58198' OR
       to_regprocedure('enforce_context_sieve_station_opening_insert()') IS NOT NULL OR
       EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass AND
                 tgname='station_gap_openings_enforce_context_sieve_insert' AND
                 NOT tgisinternal
       ) OR station_owns_portable_work(
           'coding_requirements','application_requirement','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_requirements','response_correction','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'web_claim_evidence_review','web_review_claim','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_service_state_interface','application_state_field_name','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR current_model_config_is_valid(
           '{"conversation_response_model":"qwen"}'::jsonb
       ) IS DISTINCT FROM TRUE OR current_model_config_is_valid(
           '{"web_claim_evidence_review_model":"retired"}'::jsonb
       ) IS DISTINCT FROM FALSE OR (
           SELECT count(*) FROM pg_constraint
           WHERE connamespace=current_schema()::regnamespace AND convalidated AND
                 conname IN (
                     'projects_current_model_config',
                     'jobs_current_model_config'
                 )
       )<>2 THEN
        RAISE EXCEPTION
            'current semantic station authority postcondition failed';
    END IF;
END $$;

COMMIT;
