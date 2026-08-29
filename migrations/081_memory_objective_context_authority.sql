BEGIN;

LOCK TABLE memory_chunks, memory_candidates, ai_channels, projects,
    job_generations, station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    IF encode(digest(convert_to(COALESCE(observed_source,''),'UTF8'),'sha256'),'hex')<>
       '40544be99da5f06f232982d49697763eb9ba4dcb5f7c4bffb38f4a18efd46eb4' THEN
        RAISE EXCEPTION
            'cannot install memory objective context authority: station contract is not frozen';
    END IF;
    IF EXISTS (SELECT 1 FROM memory_chunks) OR
       EXISTS (SELECT 1 FROM memory_candidates) THEN
        RAISE EXCEPTION
            'cannot install exact memory scope while unscoped memory authority exists';
    END IF;
    IF EXISTS (
        SELECT 1 FROM job_generations
        WHERE boundary_action='objective_resolve' AND
              octet_length(feedback)>2048
    ) THEN
        RAISE EXCEPTION
            'cannot install bounded objective replan authority while oversized feedback exists';
    END IF;
END $$;

ALTER TABLE ai_channels
    ADD CONSTRAINT ai_channels_id_project_id_key UNIQUE (id,project_id);

ALTER TABLE memory_chunks
    ADD COLUMN project_id BIGINT NOT NULL,
    ADD COLUMN channel_id TEXT NOT NULL,
    ADD CONSTRAINT memory_chunks_scope_fkey
        FOREIGN KEY (channel_id,project_id)
        REFERENCES ai_channels(id,project_id) ON DELETE RESTRICT,
    ADD CONSTRAINT memory_chunks_id_scope_key
        UNIQUE (id,project_id,channel_id);

ALTER TABLE memory_candidates
    ADD COLUMN project_id BIGINT NOT NULL,
    ADD COLUMN channel_id TEXT NOT NULL,
    ADD CONSTRAINT memory_candidates_scope_fkey
        FOREIGN KEY (channel_id,project_id)
        REFERENCES ai_channels(id,project_id) ON DELETE RESTRICT,
    DROP CONSTRAINT memory_candidates_source_memory_id_fkey,
    ADD CONSTRAINT memory_candidates_source_memory_scope_fkey
        FOREIGN KEY (source_memory_id,project_id,channel_id)
        REFERENCES memory_chunks(id,project_id,channel_id) ON DELETE RESTRICT,
    DROP CONSTRAINT memory_candidates_promoted_memory_id_fkey,
    ADD CONSTRAINT memory_candidates_promoted_memory_scope_fkey
        FOREIGN KEY (promoted_memory_id,project_id,channel_id)
        REFERENCES memory_chunks(id,project_id,channel_id) ON DELETE RESTRICT;

CREATE INDEX idx_memory_chunks_exact_scope
    ON memory_chunks(project_id,channel_id,id);
CREATE INDEX idx_memory_candidates_exact_scope
    ON memory_candidates(project_id,channel_id,id);

CREATE FUNCTION reject_memory_capsule_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'durable memory capsules are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER memory_chunks_immutable
BEFORE UPDATE ON memory_chunks
FOR EACH ROW EXECUTE FUNCTION reject_memory_capsule_mutation();
CREATE TRIGGER memory_chunks_no_truncate
BEFORE TRUNCATE ON memory_chunks
FOR EACH STATEMENT EXECUTE FUNCTION reject_memory_capsule_mutation();

CREATE FUNCTION preserve_memory_candidate_scope()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.channel_id IS DISTINCT FROM OLD.channel_id THEN
        RAISE EXCEPTION 'memory candidate scope is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER memory_candidates_scope_immutable
BEFORE UPDATE ON memory_candidates
FOR EACH ROW EXECUTE FUNCTION preserve_memory_candidate_scope();

ALTER TABLE job_generations
    ADD CONSTRAINT job_generations_objective_feedback_bounded CHECK (
        boundary_action<>'objective_resolve' OR octet_length(feedback)<=2048
    );

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
            'historical opening violates memory objective context station authority';
    END IF;
END $$;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256<>'c0fd460253bc36461089b326b45cf1ef2828c0a3c7063b106c231d5f0145196d' OR
       station_owns_portable_work(
           'memory_context_selection','memory_context_selection','{}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'coding_fragment','memory_context_selection','{}'::jsonb
       ) IS DISTINCT FROM FALSE OR station_owns_portable_work(
           'memory_context_selection','response_correction',
           '{"original":{"kind":"memory_context_selection","payload":{}}}'::jsonb
       ) IS DISTINCT FROM TRUE OR station_owns_portable_work(
           'memory_context_selection','response_correction','{}'::jsonb
       ) IS DISTINCT FROM FALSE THEN
        RAISE EXCEPTION
            'memory objective context authority postcondition failed: function hash %',
            observed_sha256;
    END IF;
END $$;

COMMIT;
