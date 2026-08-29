LOCK TABLE station_gap_openings, station_gap_outcomes,
    worker_skills, worker_skill_embeddings, worker_skill_checks,
    worker_skill_dependencies, worker_skill_promotion_receipts
    IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_language TEXT;
    observed_volatility "char";
    observed_strict BOOLEAN;
    observed_sha256 TEXT;
    expected_station_sha256 CONSTANT TEXT :=
        '06b8361909b8592e5991e4cd211162543db3697288958d3454cac937fb8fcae9';
BEGIN
    SELECT p.prosrc, language.lanname, p.provolatile, p.proisstrict
    INTO observed_source, observed_language, observed_volatility, observed_strict
    FROM pg_proc AS p
    JOIN pg_language AS language ON language.oid=p.prolang
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');

    IF observed_source IS NULL OR observed_language<>'sql' OR
       observed_volatility<>'i' OR NOT observed_strict OR
       encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex')<>
           expected_station_sha256 THEN
        observed_sha256 := encode(
            digest(convert_to(COALESCE(observed_source,''),'UTF8'),'sha256'),'hex'
        );
        RAISE EXCEPTION
            'cannot install retrieval-only worker skills: station authority hash % is not frozen',
            observed_sha256;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM worker_skills) OR
       EXISTS (SELECT 1 FROM worker_skill_embeddings) OR
       EXISTS (SELECT 1 FROM worker_skill_checks) OR
       EXISTS (SELECT 1 FROM worker_skill_dependencies) OR
       EXISTS (SELECT 1 FROM worker_skill_promotion_receipts) THEN
        RAISE EXCEPTION
            'cannot install retrieval-only worker skills while unauthenticated skill authority exists';
    END IF;
END $$;

DROP TRIGGER worker_skill_promotion_receipts_validate_insert
    ON worker_skill_promotion_receipts;
DROP TRIGGER worker_skill_promotion_receipts_immutable_update
    ON worker_skill_promotion_receipts;
DROP TRIGGER worker_skill_promotion_receipts_immutable_delete
    ON worker_skill_promotion_receipts;
DROP TRIGGER worker_skill_promotion_receipts_immutable_truncate
    ON worker_skill_promotion_receipts;
DROP TABLE worker_skill_promotion_receipts;
DROP FUNCTION validate_worker_skill_promotion_receipt_insert();
DROP FUNCTION prevent_worker_skill_promotion_receipt_change();

DROP TRIGGER worker_skills_validate_status_transition ON worker_skills;
DROP FUNCTION validate_worker_skill_status_transition();
DROP TRIGGER worker_skills_validate_insert ON worker_skills;
DROP FUNCTION validate_worker_skill_insert();
DROP TRIGGER worker_skills_immutable_content ON worker_skills;
DROP FUNCTION prevent_worker_skill_content_update();
DROP TRIGGER worker_skill_embeddings_immutable ON worker_skill_embeddings;
DROP FUNCTION prevent_worker_skill_embedding_update();

DROP TABLE worker_skill_checks;
DROP TABLE worker_skill_dependencies;
DROP INDEX idx_worker_skills_one_pending_version;

ALTER TABLE worker_skills
    DROP CONSTRAINT worker_skills_status_check,
    DROP COLUMN activated_at,
    DROP COLUMN rejected_at,
    DROP COLUMN retired_at,
    ADD CONSTRAINT worker_skills_active_only CHECK (status='active');

CREATE UNIQUE INDEX worker_skill_embeddings_one_frozen_identity
    ON worker_skill_embeddings(skill_id,skill_version);

CREATE FUNCTION reject_unavailable_worker_skill_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'learned skill mutation is unavailable until code-owned recurring-gap and held-out replay authority exists';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skills_reject_unavailable_statements
BEFORE INSERT OR UPDATE OR DELETE ON worker_skills
FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();
CREATE TRIGGER worker_skills_reject_unavailable_truncate
BEFORE TRUNCATE ON worker_skills
FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();
CREATE TRIGGER worker_skill_embeddings_reject_unavailable_statements
BEFORE INSERT OR UPDATE OR DELETE ON worker_skill_embeddings
FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();
CREATE TRIGGER worker_skill_embeddings_reject_unavailable_truncate
BEFORE TRUNCATE ON worker_skill_embeddings
FOR EACH STATEMENT EXECUTE FUNCTION reject_unavailable_worker_skill_mutation();

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
            'historical opening violates retrieval-only station authority';
    END IF;
END $$;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected_post_sha256 CONSTANT TEXT :=
        '40544be99da5f06f232982d49697763eb9ba4dcb5f7c4bffb38f4a18efd46eb4';
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256<>expected_post_sha256 OR
       station_owns_portable_work(
           'coding_skill_procedure','skill_procedure','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_skill_selection','skill_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_skill_procedure','response_correction',
           '{"original":{"kind":"skill_procedure","payload":{}}}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'coding_skill_procedure','response_correction','{}'::jsonb
       ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'retrieval-only worker skill station postcondition failed: function hash %',
            observed_sha256;
    END IF;
END $$;
