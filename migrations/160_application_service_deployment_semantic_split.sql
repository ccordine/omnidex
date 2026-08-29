BEGIN;

LOCK TABLE station_gap_openings, station_gap_outcomes, jobs,
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
       'fc906af27b6e8243dac60467c8408bf583fb36d0bfd9ccfb0366315c4fcac59c' THEN
        RAISE EXCEPTION
            'cannot split service deployment semantics: prior station authority differs';
    END IF;
    IF guard_source IS NULL OR guard_language<>'plpgsql' OR
       guard_volatility<>'v' OR guard_strict IS DISTINCT FROM FALSE OR
       encode(digest(convert_to(guard_source,'UTF8'),'sha256'),'hex')<>
       '33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1' THEN
        RAISE EXCEPTION
            'cannot split service deployment semantics: prior opening guard differs';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM jobs
        WHERE pipeline IN ('coding','scrum')
          AND status IN ('pending','running','waiting_input')
    ) THEN
        RAISE EXCEPTION
            'cannot split service deployment semantics while a coding job is active';
    END IF;
    IF EXISTS (
        SELECT 1 FROM generated_workload_deployments
        WHERE status IN ('prepared','applying','indeterminate')
    ) THEN
        RAISE EXCEPTION
            'cannot split service deployment semantics while a deployment is nonterminal';
    END IF;
    IF EXISTS (
        WITH RECURSIVE unresolved_chain AS (
            SELECT opening.id,opening.work_kind AS current_kind,
                   opening.portable_payload::jsonb AS current_payload
            FROM station_gap_openings AS opening
            LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
            WHERE outcome.id IS NULL
            UNION ALL
            SELECT chain.id,
                   chain.current_payload->'original'->>'kind',
                   chain.current_payload->'original'->'payload'
            FROM unresolved_chain AS chain
            WHERE chain.current_kind='response_correction'
              AND chain.current_payload->'original'->>'kind' IS NOT NULL
        )
        SELECT 1 FROM unresolved_chain
        WHERE current_kind='application_service_deployment_intent'
    ) THEN
        RAISE EXCEPTION
            'cannot split service deployment semantics while retired station work is unresolved';
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
        WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'
        WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'
        -- Retained only for immutable historical opening rows. The insert
        -- guard rejects new direct work and corrections for this retired kind.
        WHEN 'application_service_deployment_intent' THEN station='coding_service_deployment_intent'
        WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'
        WHEN 'application_service_state_interface' THEN station='coding_service_state_interface'
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
        WHEN 'roleplay_ongoing_action' THEN station='roleplay_ongoing_action'
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
        'application_service_deployment_intent',
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
        'application_service_deployment_intent',
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
    station_source TEXT;
    guard_source TEXT;
BEGIN
    SELECT procedure.prosrc INTO station_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'station_owns_portable_work(text,text,jsonb)'
    );
    SELECT procedure.prosrc INTO guard_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        'enforce_context_sieve_station_opening_insert()'
    );
    IF encode(digest(convert_to(station_source,'UTF8'),'sha256'),'hex')<>
       'd373f163c91642a2d99917e9f82d54575fa3770618ac3d24a54a1d97abf91c8d' OR
       encode(digest(convert_to(guard_source,'UTF8'),'sha256'),'hex')<>
       '5ee88ea6498bba2a89b1339a1f259d71dace780bcd9ae2ff89a66039900df1e7' OR
       EXISTS (
           SELECT 1 FROM station_gap_openings AS opening
           WHERE station_owns_portable_work(
               opening.station,opening.work_kind,opening.portable_payload::jsonb
           ) IS DISTINCT FROM TRUE
       ) OR station_owns_portable_work(
           'coding_service_continued_availability',
           'application_service_continued_availability','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_service_persistence_destination',
           'application_service_persistence_destination','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_service_persistence_destination',
           'application_service_continued_availability','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_service_continued_availability','response_correction',
           '{"original":{"kind":"application_service_continued_availability","payload":{}}}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_service_persistence_destination','response_correction',
           '{"original":{"kind":"application_service_persistence_destination","payload":{}}}'::jsonb
       ) IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'service deployment semantic split postcondition differs from exact authority';
    END IF;
END $$;

COMMIT;
