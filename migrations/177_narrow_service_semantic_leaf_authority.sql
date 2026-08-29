BEGIN;

LOCK TABLE station_gap_openings, station_gap_outcomes, jobs
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    station_source TEXT;
    station_language TEXT;
    station_volatility "char";
    station_strict BOOLEAN;
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO station_source,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );

    IF station_source IS NULL OR station_language<>'sql' OR
       station_volatility<>'i' OR NOT station_strict OR
       encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex')<>
       '7697634f9396160a6cc0d6091ac6cbee9bd8a1e553be040420b28fd6138c8167' THEN
        RAISE EXCEPTION
            'narrow service semantic leaf authority requires the exact migration 176 station function';
    END IF;
END;
$precondition$;

DO $opening_precondition$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM station_gap_openings AS opening
        JOIN jobs AS job ON job.id=opening.job_id
        LEFT JOIN station_gap_outcomes AS outcome
          ON outcome.opening_id=opening.id
        WHERE outcome.id IS NULL
          AND job.status IN ('pending','running','waiting_input')
          AND station_owns_portable_work(
              opening.station,
              opening.work_kind,
              opening.portable_payload::jsonb
          ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'narrow service semantic leaf authority requires a fresh reset: active invalid station opening exists';
    END IF;
END;
$opening_precondition$;

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

DO $postcondition$
DECLARE
    station_source TEXT;
    station_language TEXT;
    station_volatility "char";
    station_strict BOOLEAN;
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO station_source,station_language,station_volatility,station_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );

    IF station_source IS NULL OR station_language<>'sql' OR
       station_volatility<>'i' OR NOT station_strict OR
       encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex')<>
       'e08e8fe89efa49d540d56769bde6a708cce6945c371236877bde3eab472aac24' OR EXISTS (
        SELECT 1
        FROM (VALUES
            ('coding_service_endpoint_exposure','application_service_endpoint_exposure'),
            ('coding_service_endpoint_method','application_service_endpoint_method'),
            ('coding_service_endpoint_route_template','application_service_endpoint_route_template'),
            ('coding_service_endpoint_request_media','application_service_endpoint_request_media'),
            ('coding_service_endpoint_response_media','application_service_endpoint_response_media'),
            ('coding_service_endpoint_success_status','application_service_endpoint_success_status'),
            ('coding_application_state_field_coverage','application_state_field_coverage'),
            ('coding_application_state_field_purpose','application_state_field_purpose'),
            ('coding_application_state_field_kind','application_state_field_kind'),
            ('coding_application_record_field_coverage','application_record_field_coverage'),
            ('coding_application_record_field_purpose','application_record_field_purpose'),
            ('coding_application_record_field_kind','application_record_field_kind')
        ) AS ownership(station,work_kind)
        WHERE station_owns_portable_work(
            ownership.station,ownership.work_kind,'{}'::jsonb
        ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
            'not_the_owner',ownership.work_kind,'{}'::jsonb
        ) IS DISTINCT FROM FALSE
    ) OR station_owns_portable_work(
        'coding_service_endpoint_exposure',
        'application_service_endpoint_contract','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
        'coding_application_state_field_coverage',
        'application_service_state_interface','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
        'coding_service_endpoint_exposure',
        'response_correction','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
        'coding_application_state_field_purpose',
        'application_state_field_name','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR EXISTS (
        SELECT 1
        FROM station_gap_openings AS opening
        JOIN jobs AS job ON job.id=opening.job_id
        LEFT JOIN station_gap_outcomes AS outcome
          ON outcome.opening_id=opening.id
        WHERE outcome.id IS NULL
          AND job.status IN ('pending','running','waiting_input')
          AND station_owns_portable_work(
              opening.station,
              opening.work_kind,
              opening.portable_payload::jsonb
          ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'narrow service semantic leaf authority postcondition failed';
    END IF;
END;
$postcondition$;

COMMIT;
