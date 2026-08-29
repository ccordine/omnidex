BEGIN;

LOCK TABLE jobs, station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    station_hash TEXT;
    renderer_hash TEXT;
    projection_hash TEXT;
    uncertainty_shape_hash TEXT;
    uncertainty_digest_hash TEXT;
    renderer_guard_hash TEXT;
    renderer_guard_language TEXT;
    renderer_guard_volatility "char";
    renderer_guard_strict BOOLEAN;
    history_hash TEXT;
    history_language TEXT;
    history_volatility "char";
    history_strict BOOLEAN;
    lineage_hash TEXT;
    lineage_language TEXT;
    lineage_volatility "char";
    lineage_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')
      INTO station_hash
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO renderer_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_renderer_version_check'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO projection_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_prompt_projection'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_shape_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_shape_v2'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_digest_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_digest'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO renderer_guard_hash,renderer_guard_language,
           renderer_guard_volatility,renderer_guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO history_hash,history_language,history_volatility,history_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('prevent_station_gap_history_mutation()');
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO lineage_hash,lineage_language,lineage_volatility,lineage_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'fragment_generation_replacement_authority_is_exact()'
    );

    IF station_hash IS DISTINCT FROM
       'c5e5e23eaee5d23a13fed44245ffa25b826c3733161caec00dde146fa6691572' OR
       renderer_hash IS DISTINCT FROM
       'd48c2ba5f9bd4882b37dc43f552c1842a7150f424d10243da7f891e36f8e09a6' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       uncertainty_shape_hash IS DISTINCT FROM
       '234054ff407103fda7735d1db700893455aed354c43b9cac008c07ebc72e863d' OR
       uncertainty_digest_hash IS DISTINCT FROM
       'e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea' OR
       renderer_guard_hash IS DISTINCT FROM
       '8e28871bb3c57e6e3597dd16010979f36ab9b92fa2f6602526dde7b7f0c008ff' OR
       renderer_guard_language IS DISTINCT FROM 'plpgsql' OR
       renderer_guard_volatility IS DISTINCT FROM 'v' OR
       renderer_guard_strict IS DISTINCT FROM FALSE OR
       history_hash IS DISTINCT FROM
       '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58' OR
       history_language IS DISTINCT FROM 'plpgsql' OR
       history_volatility IS DISTINCT FROM 'v' OR
       history_strict IS DISTINCT FROM FALSE OR
       lineage_hash IS DISTINCT FROM
       '43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae' OR
       lineage_language IS DISTINCT FROM 'sql' OR
       lineage_volatility IS DISTINCT FROM 's' OR
       lineage_strict IS DISTINCT FROM FALSE OR
       fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM (VALUES
               ('station_gap_openings','station_gap_openings_immutable',27),
               ('station_gap_openings','station_gap_openings_truncate_immutable',34),
               ('station_gap_outcomes','station_gap_outcomes_immutable',27),
               ('station_gap_outcomes','station_gap_outcomes_truncate_immutable',34)
           ) AS expected(table_name,trigger_name,trigger_type)
           LEFT JOIN pg_trigger AS actual
             ON actual.tgrelid=to_regclass(expected.table_name)
            AND actual.tgname=expected.trigger_name
            AND actual.tgfoid=to_regprocedure('prevent_station_gap_history_mutation()')
            AND actual.tgtype=expected.trigger_type
            AND actual.tgenabled='O' AND NOT actual.tgisinternal
           WHERE actual.oid IS NULL
       ) OR EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version NOT IN (
               'omnidex.render-portable-job.v5',
               'omnidex.render-portable-job.v6',
               'omnidex.render-portable-job.v7'
           ) OR work_kind IN (
               'application_requirement_candidate_cardinality',
               'application_requirement_candidate_split',
               'application_requirement_candidate_split_correction'
           ) OR semantic_uncertainty_contract->>'id' IS DISTINCT FROM
               'omnidex.semantic-uncertainty.'||work_kind||
               CASE
                   WHEN renderer_version='omnidex.render-portable-job.v7' AND
                        work_kind IN (
                            'application_product_context',
                            'application_requirement_coverage',
                            'application_requirement',
                            'application_project_stack_constraint'
                        ) THEN '.v2'
                   ELSE '.v1'
               END
       ) OR EXISTS (
           SELECT 1 FROM jobs
           WHERE pipeline IN ('coding','scrum')
             AND status IN ('pending','running','waiting_input')
       ) OR EXISTS (
           SELECT 1
           FROM station_gap_openings AS opening
           LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
           WHERE outcome.id IS NULL
       ) THEN
        RAISE EXCEPTION
            'portable renderer V8 requires exact terminal V5/V6/V7 history and migration 187 authority';
    END IF;
END;
$precondition$;

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
        WHEN 'application_requirement_candidate_cardinality' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_split' THEN station='coding_requirements'
        WHEN 'application_requirement_candidate_split_correction' THEN station='coding_requirements'
        WHEN 'repository_requirement_coverage' THEN station='coding_requirements'
        WHEN 'repository_requirement' THEN station='coding_requirements'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'
        WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_state_field_coverage' THEN station='coding_application_state_field_coverage'
        WHEN 'application_state_field_purpose' THEN station='coding_application_state_field_purpose'
        WHEN 'application_state_field_kind' THEN station='coding_application_state_field_kind'
        WHEN 'application_record_field_coverage' THEN station='coding_application_record_field_coverage'
        WHEN 'application_record_field_purpose' THEN station='coding_application_record_field_purpose'
        WHEN 'application_record_field_kind' THEN station='coding_application_record_field_kind'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
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
        WHEN 'repository_artifact_absence' THEN station='coding_repository_artifact_absence'
        WHEN 'plain_text_artifact_creation' THEN station='coding_plain_text_artifact_creation'
        WHEN 'declaration_artifact_boundary' THEN station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'runtime_capability_selection' THEN station='coding_runtime_capability_selection'
        WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_generation_replacement' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_renderer_version_check,
    DROP CONSTRAINT station_gap_openings_semantic_uncertainty_shape_v2,
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version IN (
            'omnidex.render-portable-job.v5',
            'omnidex.render-portable-job.v6',
            'omnidex.render-portable-job.v7',
            'omnidex.render-portable-job.v8'
        )
    ),
    ADD CONSTRAINT station_gap_openings_semantic_uncertainty_shape_v3 CHECK (
        jsonb_typeof(semantic_uncertainty_contract)='object' AND
        semantic_uncertainty_contract ?& ARRAY[
            'id','work_kind','exact_question','deterministic_limitation',
            'required_information','single_result','deterministic_consumer'
        ] AND
        semantic_uncertainty_contract-
            'id'-'work_kind'-'exact_question'-'deterministic_limitation'-
            'required_information'-'single_result'-'deterministic_consumer'='{}'::jsonb AND
        jsonb_typeof(semantic_uncertainty_contract->'id')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'work_kind')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'exact_question')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_limitation')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'required_information')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'single_result')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_consumer')='string' AND
        semantic_uncertainty_contract->>'work_kind'=work_kind AND
        (
            work_kind NOT IN (
                'application_requirement_candidate_cardinality',
                'application_requirement_candidate_split',
                'application_requirement_candidate_split_correction'
            ) OR renderer_version='omnidex.render-portable-job.v8'
        ) AND
        semantic_uncertainty_contract->>'id'=
            'omnidex.semantic-uncertainty.'||work_kind||
            CASE
                WHEN renderer_version='omnidex.render-portable-job.v8' AND
                     work_kind='application_requirement' THEN '.v3'
                WHEN renderer_version IN (
                         'omnidex.render-portable-job.v7',
                         'omnidex.render-portable-job.v8'
                     ) AND work_kind IN (
                         'application_product_context',
                         'application_requirement_coverage',
                         'application_requirement',
                         'application_project_stack_constraint'
                     ) THEN '.v2'
                ELSE '.v1'
            END AND
        octet_length(semantic_uncertainty_contract->>'id') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'id'=btrim(semantic_uncertainty_contract->>'id') AND
        octet_length(semantic_uncertainty_contract->>'exact_question') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'exact_question'=
            btrim(semantic_uncertainty_contract->>'exact_question') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_limitation') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_limitation'=
            btrim(semantic_uncertainty_contract->>'deterministic_limitation') AND
        octet_length(semantic_uncertainty_contract->>'required_information') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'required_information'=
            btrim(semantic_uncertainty_contract->>'required_information') AND
        octet_length(semantic_uncertainty_contract->>'single_result') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'single_result'=
            btrim(semantic_uncertainty_contract->>'single_result') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_consumer') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_consumer'=
            btrim(semantic_uncertainty_contract->>'deterministic_consumer') AND
        length(semantic_uncertainty_contract->>'exact_question')-
            length(replace(semantic_uncertainty_contract->>'exact_question','?',''))=1 AND
        right(semantic_uncertainty_contract->>'exact_question',1)='?' AND
        left(semantic_uncertainty_contract->>'single_result',4)='One ' AND
        (
            (semantic_uncertainty_contract->>'id')||
            (semantic_uncertainty_contract->>'work_kind')||
            (semantic_uncertainty_contract->>'exact_question')||
            (semantic_uncertainty_contract->>'deterministic_limitation')||
            (semantic_uncertainty_contract->>'required_information')||
            (semantic_uncertainty_contract->>'single_result')||
            (semantic_uncertainty_contract->>'deterministic_consumer')
        ) !~ E'[\\r\\n]'
    );

CREATE OR REPLACE FUNCTION require_current_station_gap_renderer()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.renderer_version IS DISTINCT FROM
       'omnidex.render-portable-job.v8' THEN
        RAISE EXCEPTION
            'new station gap opening requires portable renderer V8';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $postcondition$
DECLARE
    station_hash TEXT;
    station_language TEXT;
    station_volatility "char";
    station_strict BOOLEAN;
    renderer_hash TEXT;
    projection_hash TEXT;
    uncertainty_shape_hash TEXT;
    uncertainty_digest_hash TEXT;
    guard_hash TEXT;
    guard_language TEXT;
    guard_volatility "char";
    guard_strict BOOLEAN;
BEGIN
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO station_hash,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO renderer_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_renderer_version_check'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO projection_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_prompt_projection'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_shape_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_shape_v3'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
      INTO uncertainty_digest_hash
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_semantic_uncertainty_digest'
      AND contype='c' AND convalidated;
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
           language.lanname,procedure.provolatile,procedure.proisstrict
      INTO guard_hash,guard_language,guard_volatility,guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_current_station_gap_renderer()');

    IF station_hash IS DISTINCT FROM
       '9519578164ba4cff0a0b5f194a848e2e2ac3736b7d107f0e11a561aa642ef312' OR
       station_language IS DISTINCT FROM 'sql' OR
       station_volatility IS DISTINCT FROM 'i' OR
       station_strict IS DISTINCT FROM TRUE OR
       renderer_hash IS DISTINCT FROM
       'e24ead43cb1f974b9e8ece01904d72e16a6d11dcd60ed45c0a10900360c626e7' OR
       projection_hash IS DISTINCT FROM
       '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad' OR
       uncertainty_shape_hash IS DISTINCT FROM
       'e0fc0a371298cef3dde3b40902e4846d390a4bd182b56b7d9e0e9264aca4939a' OR
       uncertainty_digest_hash IS DISTINCT FROM
       'e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea' OR
       guard_hash IS DISTINCT FROM
       '9be8623acc55e88cda131dc0654f05da5d96b9f9506b1382d35f32fc17f724a8' OR
       guard_language IS DISTINCT FROM 'plpgsql' OR
       guard_volatility IS DISTINCT FROM 'v' OR
       guard_strict IS DISTINCT FROM FALSE OR
       station_owns_portable_work(
           'coding_requirements',
           'application_requirement_candidate_cardinality','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR
       station_owns_portable_work(
           'coding_requirements',
           'application_requirement_candidate_split','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR
       station_owns_portable_work(
           'coding_requirements',
           'application_requirement_candidate_split_correction','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR
       station_owns_portable_work(
           'coding_project_stack_constraint',
           'application_requirement_candidate_split','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_require_current_renderer'
             AND tgfoid=to_regprocedure('require_current_station_gap_renderer()')
             AND tgtype=7 AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='station_gap_openings'::regclass
             AND conname='station_gap_openings_semantic_uncertainty_shape_v2'
       ) OR EXISTS (
           SELECT 1 FROM station_gap_openings
           WHERE renderer_version NOT IN (
               'omnidex.render-portable-job.v5',
               'omnidex.render-portable-job.v6',
               'omnidex.render-portable-job.v7'
           ) OR work_kind IN (
               'application_requirement_candidate_cardinality',
               'application_requirement_candidate_split',
               'application_requirement_candidate_split_correction'
           ) OR semantic_uncertainty_contract->>'id' IS DISTINCT FROM
               'omnidex.semantic-uncertainty.'||work_kind||
               CASE
                   WHEN renderer_version='omnidex.render-portable-job.v7' AND
                        work_kind IN (
                            'application_product_context',
                            'application_requirement_coverage',
                            'application_requirement',
                            'application_project_stack_constraint'
                        ) THEN '.v2'
                   ELSE '.v1'
               END
       ) OR EXISTS (
           SELECT 1
           FROM station_gap_openings AS opening
           LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
           WHERE outcome.id IS NULL
       ) THEN
        RAISE EXCEPTION
            'portable renderer V8 postcondition failed: station %, renderer %, projection %, uncertainty %, digest %, guard %',
            station_hash,renderer_hash,projection_hash,uncertainty_shape_hash,
            uncertainty_digest_hash,guard_hash;
    END IF;
END;
$postcondition$;

COMMIT;
