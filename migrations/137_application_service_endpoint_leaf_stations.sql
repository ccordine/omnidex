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
        '65ffb44f3ba4aa764d10fadafb0928c30e02fa6abdfc192f45c2044eb79e65e3';
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
            'cannot install service endpoint leaf stations: prior station authority differs';
    END IF;
END $$;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT :=
        '617105467a6ac384cb40cb8ef08503056e21507c974f29cfa02ec95215865310';
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
            'cannot install service endpoint leaf stations: prior opening guard differs';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        WITH RECURSIVE unresolved_chain AS (
            SELECT opening.id,opening.work_kind AS current_kind,
                   opening.portable_payload::jsonb AS current_payload
            FROM station_gap_openings AS opening
            LEFT JOIN station_gap_outcomes AS outcome
              ON outcome.opening_id=opening.id
            JOIN jobs AS job ON job.id=opening.job_id
            WHERE outcome.id IS NULL
              AND job.status IN ('pending','running','waiting_input')
            UNION ALL
            SELECT chain.id,
                   chain.current_payload->'original'->>'kind',
                   chain.current_payload->'original'->'payload'
            FROM unresolved_chain AS chain
            WHERE chain.current_kind='response_correction'
              AND chain.current_payload->'original'->>'kind' IS NOT NULL
        )
        SELECT 1 FROM unresolved_chain
        WHERE current_kind='application_service_endpoint_contract'
    ) THEN
        RAISE EXCEPTION
            'cannot retire bundled service endpoint contract while an active opening is unresolved';
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
        WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'
        WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'
        WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'
        WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'
        WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'
        WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'
        WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'
        -- Retained only for immutable historical opening rows. The insert
        -- guard below rejects new bundled work and its corrections.
        WHEN 'application_service_endpoint_contract' THEN station='coding_service_endpoint_contract'
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        -- These mappings are retained only so immutable historical opening rows
        -- remain valid. Current runtime code does not dispatch them, and the
        -- insert guard below rejects every new opening and correction for them.
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

CREATE OR REPLACE FUNCTION enforce_context_sieve_station_opening_insert()
RETURNS TRIGGER AS $$
DECLARE
    correction_payload JSONB;
    original_kind TEXT;
BEGIN
    IF NEW.work_kind IN (
        'application_requirements',
        'application_file_content',
        'application_job_specification_repair',
        'application_job_specification_review',
        'application_acceptance_grounding_review',
        'application_service_endpoint_contract',
        'conversation_context_selection',
        'memory_context_selection',
        'roleplay_narrative_continuity'
    ) THEN
        RAISE EXCEPTION
            'retired station work kind % cannot create a new opening',
            NEW.work_kind;
    END IF;
    IF NEW.work_kind <> 'response_correction' THEN
        RETURN NEW;
    END IF;

    correction_payload := NEW.portable_payload::jsonb;
    original_kind := correction_payload->'original'->>'kind';
    IF original_kind='response_correction' THEN
        RAISE EXCEPTION
            'nested response correction cannot create a new station opening';
    END IF;
    IF original_kind IN (
        'application_requirements',
        'application_file_content',
        'application_job_specification_repair',
        'application_job_specification_review',
        'application_acceptance_grounding_review',
        'application_service_endpoint_contract',
        'conversation_context_selection',
        'memory_context_selection',
        'roleplay_narrative_continuity'
    ) THEN
        RAISE EXCEPTION
            'retired station work kind % cannot create a correction opening',
            original_kind;
    END IF;
    IF original_kind IS DISTINCT FROM 'application_job_specification' AND (
        NOT correction_payload ? 'retained_candidate' OR
        jsonb_typeof(correction_payload->'retained_candidate') IS DISTINCT FROM 'string' OR
        btrim(correction_payload->>'retained_candidate')=''
    ) THEN
        RAISE EXCEPTION
            'response correction for % requires one non-blank retained_candidate',
            original_kind;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql VOLATILE;

DO $$
DECLARE
    observed_station_source TEXT;
    observed_guard_source TEXT;
    observed_station_sha256 TEXT;
    observed_guard_sha256 TEXT;
    expected_station_sha256 CONSTANT TEXT :=
        'efbe7a813ef0a0ad9df888bdca40f571fff58e5db6d261c4dcf9d0ee7229f5e1';
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
        'coding_service_endpoint_exposure','application_service_endpoint_exposure','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_method','application_service_endpoint_method','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_route_template','application_service_endpoint_route_template','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_request_media','application_service_endpoint_request_media','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_response_media','application_service_endpoint_response_media','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_success_status','application_service_endpoint_success_status','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_contract','application_service_endpoint_contract','{}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_exposure','response_correction',
        '{"original":{"kind":"application_service_endpoint_exposure","payload":{}}}'::jsonb
    ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
        'coding_service_endpoint_method','application_service_endpoint_exposure','{}'::jsonb
    ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'service endpoint leaf station postcondition differs from exact authority';
    END IF;
END $$;

COMMIT;
