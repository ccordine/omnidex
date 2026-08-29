BEGIN;

LOCK TABLE station_gap_openings, station_call_openings,
    llm_call_evidence IN ACCESS EXCLUSIVE MODE;

DO $$
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
       'd373f163c91642a2d99917e9f82d54575fa3770618ac3d24a54a1d97abf91c8d' THEN
        RAISE EXCEPTION
            'raw portable transport requires the exact prior station authority';
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
        WHEN 'application_job_objective' THEN station='coding_workload'
        WHEN 'application_behavior_coverage' THEN station='coding_workload'
        WHEN 'application_behavior' THEN station='coding_workload'
        WHEN 'application_criterion_coverage' THEN station='coding_workload'
        WHEN 'application_criterion' THEN station='coding_workload'
        WHEN 'application_target_tree' THEN station='coding_target_tree'
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'
        WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_state_field_coverage' THEN station='coding_service_state_interface'
        WHEN 'application_state_field_name' THEN station='coding_service_state_interface'
        WHEN 'application_state_field_kind' THEN station='coding_service_state_interface'
        WHEN 'application_record_field_coverage' THEN station='coding_service_state_interface'
        WHEN 'application_record_field_name' THEN station='coding_service_state_interface'
        WHEN 'application_record_field_kind' THEN station='coding_service_state_interface'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
        WHEN 'repository_search_anchor_coverage' THEN station='coding_repository_search_term'
        WHEN 'repository_search_anchor' THEN station='coding_repository_search_term'
        WHEN 'repository_change_owner' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance_leaf' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_issue_detail' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_issue_kind' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        WHEN 'context_search_term_coverage' THEN station='context_search_terms'
        WHEN 'context_search_term' THEN station='context_search_terms'
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
        WHEN 'web_search_term_coverage' THEN station='web_search_terms'
        WHEN 'web_search_term' THEN station='web_search_terms'
        WHEN 'web_relevance_relation' THEN station='web_relevance'
        WHEN 'web_synthesis_paragraph_coverage' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_paragraph' THEN station='web_grounded_synthesis'
        WHEN 'web_synthesis_evidence_relation' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'
        WHEN 'web_review_claim_coverage' THEN station='web_claim_evidence_review'
        WHEN 'web_review_claim' THEN station='web_claim_evidence_review'
        WHEN 'web_review_claim_verdict' THEN station='web_claim_evidence_review'
        WHEN 'web_review_issue_evidence_relation' THEN station='web_claim_evidence_review'
        WHEN 'web_review_issue_detail' THEN station='web_claim_evidence_review'
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
        WHEN 'response_correction' THEN
            payload->'original'->>'kind'<>'response_correction' AND
            COALESCE(station_owns_portable_work(
                station,payload->'original'->>'kind',payload->'original'->'payload'
            ),FALSE)
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM station_gap_openings
        WHERE portable_schema IS DISTINCT FROM 'omnidex.portable-job.v2' OR
              renderer_version IS DISTINCT FROM 'omnidex.render-portable-job.v4' OR
              response_schema IS DISTINCT FROM 'null' OR
              scope IS DISTINCT FROM CASE
                  WHEN work_kind='application_target_tree' THEN
                      'portable_structural_worker'
                  WHEN work_kind IN (
                      'fragment_generation',
                      'fragment_modification',
                      'fragment_correction'
                  ) THEN 'portable_fragment_worker'
                  ELSE 'portable_semantic_worker'
              END OR
              station_owns_portable_work(
                  station,work_kind,portable_payload::jsonb
              ) IS DISTINCT FROM TRUE
    ) OR EXISTS (
        SELECT 1 FROM station_call_openings
        WHERE protocol IS DISTINCT FROM
              'omnidex.ollama-raw-text-generate-request.v1'
    ) OR EXISTS (
        SELECT 1 FROM llm_call_evidence
        WHERE response_format IS DISTINCT FROM 'text' OR
              response_schema IS NOT NULL
    ) THEN
        RAISE EXCEPTION
            'raw portable transport requires a fresh reset: legacy portable transport state exists';
    END IF;
END $$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_scope_check,
    DROP CONSTRAINT station_gap_openings_portable_schema_check,
    DROP CONSTRAINT station_gap_openings_renderer_version_check;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_scope_check CHECK (
        scope IN (
            'portable_semantic_worker',
            'portable_structural_worker',
            'portable_fragment_worker'
        )
    ),
    ADD CONSTRAINT station_gap_openings_portable_schema_check CHECK (
        portable_schema='omnidex.portable-job.v2'
    ),
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version='omnidex.render-portable-job.v4'
    ),
    ADD CONSTRAINT station_gap_openings_current_raw_transport CHECK (
        response_schema='null' AND
        CASE
            WHEN work_kind='application_target_tree' THEN
                scope='portable_structural_worker'
            WHEN work_kind IN (
                'fragment_generation',
                'fragment_modification',
                'fragment_correction'
            ) THEN scope='portable_fragment_worker'
            ELSE scope='portable_semantic_worker'
        END
    );

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_current_raw_transport CHECK (
        protocol='omnidex.ollama-raw-text-generate-request.v1'
    );

ALTER TABLE llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_current_raw_transport CHECK (
        response_format='text' AND response_schema IS NULL
    );

COMMIT;
