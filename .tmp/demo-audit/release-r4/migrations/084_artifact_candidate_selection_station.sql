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
        'b10183c38f3a39706076e2fb244605f28fd3ec117852003d707d833f8a36e8c9';
BEGIN
    SELECT p.prosrc, language.lanname, p.provolatile, p.proisstrict
    INTO observed_source, observed_language, observed_volatility, observed_strict
    FROM pg_proc AS p
    JOIN pg_language AS language ON language.oid=p.prolang
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');

    IF observed_source IS NULL OR observed_language <> 'sql' OR
       observed_volatility <> 'i' OR NOT observed_strict THEN
        RAISE EXCEPTION
            'cannot install artifact candidate selection station: exact prior station function is missing or has different authority';
    END IF;
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> expected_pre_sha256 THEN
        RAISE EXCEPTION
            'cannot install artifact candidate selection station: prior station function hash % differs from %',
            observed_sha256, expected_pre_sha256;
    END IF;
END $$;

CREATE OR REPLACE FUNCTION station_owns_portable_work(
    station TEXT, work_kind TEXT, payload JSONB
)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_identity' THEN station='coding_product_identity'
        WHEN 'requirement_partition' THEN station='coding_requirement_partition'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'
        WHEN 'repository_grounded_review' THEN station='repository_grounded_review'
        WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'
        WHEN 'conversation_context_selection' THEN station='conversation_context_selection'
        WHEN 'memory_context_selection' THEN station='memory_context_selection'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN
            station='web_grounded_synthesis_correction'
        WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'declaration_artifact_boundary' THEN
            station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN
            station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
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
    IF EXISTS (
        SELECT 1 FROM station_gap_openings AS opening
        WHERE station_owns_portable_work(
            opening.station,opening.work_kind,opening.portable_payload::jsonb
        ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'historical opening violates artifact candidate selection station authority';
    END IF;
END $$;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected_post_sha256 CONSTANT TEXT :=
        '12e011154fafc79b346688262f9d4aaaa11c4ef9ff4559fa7353ee47a82a8565';
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> expected_post_sha256 OR
       station_owns_portable_work(
           'coding_artifact_candidate_selection',
           'artifact_candidate_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_artifact_handling',
           'artifact_candidate_selection','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_artifact_candidate_selection','artifact_handling','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_artifact_candidate_selection','response_correction',
           '{"original":{"kind":"artifact_candidate_selection","payload":{}}}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_artifact_candidate_selection','response_correction',
           '{"original":{"kind":"response_correction","payload":{"original":{"kind":"artifact_candidate_selection","payload":{}}}}}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_declaration_artifact_boundary','response_correction',
           '{"original":{"kind":"artifact_candidate_selection","payload":{}}}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_artifact_candidate_selection','response_correction','{}'::jsonb
       ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'artifact candidate selection station postcondition failed: function hash % differs from % or routing is invalid',
            observed_sha256, expected_post_sha256;
    END IF;
END $$;

COMMIT;
