LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

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
        WHEN 'application_acceptance_grounding_review' THEN station='coding_workload_review'
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
        WHEN 'database_schema_selection' THEN station='database_schema_selection'
        WHEN 'database_query_intent' THEN station='database_query_intent'
        WHEN 'database_evidence_gap' THEN station='database_evidence_gap'
        WHEN 'database_join_path_selection' THEN station='database_join_path_selection'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'web_grounded_synthesis_correction' THEN
            station='web_grounded_synthesis_correction'
        WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'known_artifact_truth' THEN station='coding_known_artifact_truth'
        WHEN 'declaration_artifact_boundary' THEN
            station='coding_declaration_artifact_boundary'
        WHEN 'artifact_candidate_selection' THEN
            station='coding_artifact_candidate_selection'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'typescript_repair_guidance' THEN
            station='coding_fragment_repair_guidance'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(
            station,payload->'original'->>'kind',payload->'original'->'payload'
        ),FALSE)
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE TABLE database_evidence_receipts (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    data_source_id TEXT NOT NULL REFERENCES data_sources(id) ON DELETE RESTRICT,
    schema_fingerprint TEXT NOT NULL,
    intent_hash TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    result_hash TEXT NOT NULL,
    plan_total_cost DOUBLE PRECISION NOT NULL,
    plan_estimated_rows BIGINT NOT NULL,
    returned_rows INTEGER NOT NULL,
    result_bytes INTEGER NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT database_evidence_schema_hash_check CHECK (
        schema_fingerprint ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT database_evidence_intent_hash_check CHECK (
        intent_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT database_evidence_query_hash_check CHECK (
        query_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT database_evidence_result_hash_check CHECK (
        result_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT database_evidence_metrics_check CHECK (
        plan_total_cost >= 0 AND plan_total_cost <= 1000000000000 AND
        plan_estimated_rows >= 0 AND plan_estimated_rows <= 1000000000 AND
        returned_rows >= 0 AND returned_rows <= 500 AND
        result_bytes >= 0 AND result_bytes <= 4194304 AND
        isfinite(acquired_at)
    ),
    UNIQUE (job_id, schema_fingerprint, intent_hash, query_hash, result_hash)
);

CREATE INDEX idx_database_evidence_job
    ON database_evidence_receipts(job_id, id ASC);

CREATE FUNCTION validate_database_evidence_receipt_insert()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM 1
    FROM jobs AS job
    JOIN ai_channels AS channel
      ON channel.id=job.metadata->>'channel_id'
    JOIN data_sources AS source
      ON source.id=NEW.data_source_id
    WHERE job.id=NEW.job_id
      AND job.pipeline='chat'
      AND jsonb_typeof(job.metadata->'data_source_id')='string'
      AND job.metadata->>'data_source_id'=NEW.data_source_id
      AND channel.data_source_id=NEW.data_source_id
      AND source.schema_catalog->>'fingerprint'=NEW.schema_fingerprint
    FOR KEY SHARE OF job,channel,source;
    IF NOT FOUND THEN
        RAISE EXCEPTION
            'database evidence receipt does not match its exact channel and job source binding';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER database_evidence_receipts_validate_insert
BEFORE INSERT ON database_evidence_receipts
FOR EACH ROW EXECUTE FUNCTION validate_database_evidence_receipt_insert();

CREATE FUNCTION reject_database_evidence_receipt_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'database evidence receipts are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER database_evidence_receipts_immutable
BEFORE UPDATE OR DELETE ON database_evidence_receipts
FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_receipt_change();

CREATE TRIGGER database_evidence_receipts_truncate_immutable
BEFORE TRUNCATE ON database_evidence_receipts
FOR EACH STATEMENT EXECUTE FUNCTION reject_database_evidence_receipt_change();

CREATE FUNCTION reject_database_evidence_job_binding_change()
RETURNS TRIGGER AS $$
BEGIN
    IF (
        NEW.metadata->>'channel_id' IS DISTINCT FROM OLD.metadata->>'channel_id' OR
        NEW.metadata->>'data_source_id' IS DISTINCT FROM OLD.metadata->>'data_source_id'
    ) THEN
        RAISE EXCEPTION
            'job channel and data-source evidence binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_database_evidence_binding_immutable
BEFORE UPDATE OF metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_database_evidence_job_binding_change();

DO $$
BEGIN
    IF station_owns_portable_work('database_schema_selection','database_query_intent','{}'::jsonb) OR
       NOT station_owns_portable_work('database_schema_selection','database_schema_selection','{}'::jsonb) OR
       NOT station_owns_portable_work('database_query_intent','database_query_intent','{}'::jsonb) OR
       NOT station_owns_portable_work('database_evidence_gap','database_evidence_gap','{}'::jsonb) OR
       NOT station_owns_portable_work('database_join_path_selection','database_join_path_selection','{}'::jsonb) OR
       to_regclass(current_schema() || '.database_evidence_receipts') IS NULL OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='database_evidence_receipts'::regclass
             AND tgname IN (
                 'database_evidence_receipts_validate_insert',
                 'database_evidence_receipts_immutable',
                 'database_evidence_receipts_truncate_immutable'
             )
             AND NOT tgisinternal
           GROUP BY tgrelid
           HAVING COUNT(*)=3
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='jobs'::regclass
             AND tgname='jobs_database_evidence_binding_immutable'
             AND NOT tgisinternal
       ) THEN
        RAISE EXCEPTION 'database cognition authority postcondition failed';
    END IF;
END $$;
