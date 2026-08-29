BEGIN;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_strict BOOLEAN;
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT :=
        'efbe7a813ef0a0ad9df888bdca40f571fff58e5db6d261c4dcf9d0ee7229f5e1';
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO observed_source,observed_language,observed_volatility,observed_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_source IS NULL OR observed_language <> 'sql' OR
       observed_volatility <> 'i' OR NOT observed_strict OR
       observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot install service state lifetime station: prior station authority differs';
    END IF;
END $$;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT :=
        '33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1';
BEGIN
    SELECT procedure.prosrc,language.lanname,procedure.provolatile
    INTO observed_source,observed_language,observed_volatility
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    observed_sha256 := encode(
        digest(convert_to(observed_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_source IS NULL OR observed_language <> 'plpgsql' OR
       observed_volatility <> 'v' OR observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot install service state lifetime station: prior opening guard differs';
    END IF;
END $$;

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
        WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
        -- Retained only for immutable historical opening rows. The insert
        -- guard rejects new bundled work and its corrections.
        WHEN 'application_service_endpoint_contract' THEN station='coding_service_endpoint_contract'
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        -- These mappings are retained only so immutable historical opening rows
        -- remain valid. Current runtime code does not dispatch them, and the
        -- insert guard rejects every new opening and correction for them.
        WHEN 'application_requirements' THEN station='coding_requirements'
        WHEN 'application_file_content' THEN station='coding_workload'
        WHEN 'application_job_specification_repair' THEN station='coding_workload'
        WHEN 'application_job_specification_review' THEN
            station IN ('coding_workload','coding_workload_review')
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'memory_context_selection' THEN station='memory_context_selection'
        WHEN 'roleplay_narrative_continuity' THEN station='roleplay_narrative_continuity'
        WHEN 'context_search_terms' THEN station='context_search_terms'
        WHEN 'context_relevance' THEN station='context_relevance'
        WHEN 'context_minification' THEN station='context_minification'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'roleplay_grounded_response' THEN station='conversation_response'
        WHEN 'roleplay_canon_extraction' THEN station='roleplay_canon_extraction'
        WHEN 'roleplay_voice_rewrite' THEN station='roleplay_voice_rewrite'
        WHEN 'roleplay_voice_preservation' THEN station='roleplay_voice_preservation'
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
DECLARE
    observed_station_source TEXT;
    observed_guard_source TEXT;
    observed_station_sha256 TEXT;
    observed_guard_sha256 TEXT;
    expected_station_sha256 CONSTANT TEXT :=
        '9dbad6b51b0c9efc761a864a8050e9437b61c23845b3ab642a50ce4c3af6614f';
    expected_guard_sha256 CONSTANT TEXT :=
        '33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1';
BEGIN
    SELECT procedure.prosrc INTO observed_station_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT procedure.prosrc INTO observed_guard_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    observed_station_sha256 := encode(
        digest(convert_to(observed_station_source,'UTF8'),'sha256'),'hex'
    );
    observed_guard_sha256 := encode(
        digest(convert_to(observed_guard_source,'UTF8'),'sha256'),'hex'
    );
    IF observed_station_sha256 <> expected_station_sha256 OR
       observed_guard_sha256 <> expected_guard_sha256 OR EXISTS (
        SELECT 1 FROM station_gap_openings AS opening
        WHERE station_owns_portable_work(
            opening.station,opening.work_kind,opening.portable_payload::jsonb
        ) IS DISTINCT FROM TRUE
    ) OR station_owns_portable_work(
        'coding_service_state_lifetime','application_service_state_lifetime','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_workload','application_service_state_lifetime','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
        'coding_service_state_lifetime','application_service_endpoint_requirement','{}'::jsonb
    ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
        'coding_service_state_lifetime','response_correction',
        '{"original":{"kind":"application_service_state_lifetime","payload":{}}}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_state_lifetime','response_correction','{}'::jsonb
    ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'service state lifetime station postcondition differs from exact authority';
    END IF;
END $$;

COMMIT;
