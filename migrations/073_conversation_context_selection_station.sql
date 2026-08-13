LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_strict BOOLEAN;
    observed_sha256 TEXT;
    expected_pre_sha256 CONSTANT TEXT := '8f55bfb4027623220c443013e3f14acb8ec8b01d56f108746fa0326600cf265d';
BEGIN
    SELECT p.prosrc, language.lanname, p.provolatile, p.proisstrict
    INTO observed_source, observed_language, observed_volatility, observed_strict
    FROM pg_proc AS p
    JOIN pg_language AS language ON language.oid=p.prolang
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');

    IF observed_source IS NULL OR observed_language <> 'sql' OR
       observed_volatility <> 'i' OR NOT observed_strict THEN
        RAISE EXCEPTION
            'cannot install conversation context-selection station: exact prior station function is missing or has different authority';
    END IF;
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot install conversation context-selection station: prior station function hash % differs from %',
            observed_sha256, expected_pre_sha256;
    END IF;
END $$;

CREATE OR REPLACE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_identity' THEN station='coding_product_identity'
        WHEN 'requirement_partition' THEN station='coding_requirement_partition'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'
        WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'skill_procedure' THEN station='coding_skill_procedure'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        WHEN 'response_correction' THEN station_owns_portable_work(
            station,payload->'original'->>'kind',payload->'original'->'payload'
        )
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected_post_sha256 CONSTANT TEXT := '3470c9b282bef241bf523a63ea665a38afaf64d7bf144934bc4eba8b0be4a2f8';
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> expected_post_sha256 OR
       NOT station_owns_portable_work(
           'conversation_context_selection','conversation_context_selection','{}'::jsonb
       ) OR station_owns_portable_work(
           'conversation_objective_kind','conversation_context_selection','{}'::jsonb
       ) OR station_owns_portable_work(
           'conversation_context_selection','conversation_objective_kind','{}'::jsonb
       ) OR NOT station_owns_portable_work(
           'web_claim_evidence_review','web_claim_evidence_review','{}'::jsonb
       ) OR station_owns_portable_work(
           'conversation_context_selection','unsupported_work','{}'::jsonb
       ) THEN
        RAISE EXCEPTION
            'conversation context-selection station postcondition failed: function hash % differs from % or routing is invalid',
            observed_sha256, expected_post_sha256;
    END IF;
END $$;
