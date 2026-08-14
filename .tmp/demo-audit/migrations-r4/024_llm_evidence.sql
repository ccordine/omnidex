CREATE TABLE IF NOT EXISTS llm_call_evidence (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    step_id BIGINT NOT NULL REFERENCES job_steps(id) ON DELETE RESTRICT,
    scope TEXT NOT NULL CHECK (BTRIM(scope) <> ''),
    work_id TEXT CHECK (work_id IS NULL OR work_id ~ '^[0-9a-f]{64}$'),
    work_kind TEXT CHECK (work_kind IS NULL OR BTRIM(work_kind) <> ''),
    requested_model TEXT NOT NULL CHECK (BTRIM(requested_model) <> ''),
    model TEXT NOT NULL CHECK (BTRIM(model) <> ''),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    system_prompt TEXT NOT NULL CHECK (BTRIM(system_prompt) <> ''),
    user_prompt TEXT NOT NULL CHECK (BTRIM(user_prompt) <> ''),
    request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    response_format TEXT NOT NULL CHECK (response_format IN ('text', 'json')),
    response_schema JSONB,
    context_tokens INTEGER NOT NULL CHECK (context_tokens > 0),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens > 0),
    response TEXT,
    response_sha256 TEXT CHECK (response_sha256 IS NULL OR response_sha256 ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('preparation_failed', 'generation_failed', 'empty_response', 'succeeded')),
    error TEXT,
    latency_ms BIGINT NOT NULL CHECK (latency_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((work_id IS NULL) = (work_kind IS NULL)),
    CHECK (response_schema IS NULL OR response_format = 'json'),
    CHECK ((response IS NULL) = (response_sha256 IS NULL)),
    CHECK ((status = 'succeeded') = (error IS NULL)),
    CHECK (status <> 'succeeded' OR (response IS NOT NULL AND BTRIM(response) <> '')),
    CHECK (status <> 'preparation_failed' OR response IS NULL),
    CHECK (status <> 'empty_response' OR response IS NULL OR BTRIM(response) = '')
);

CREATE INDEX IF NOT EXISTS idx_llm_call_evidence_job
    ON llm_call_evidence (job_id, id ASC);

CREATE INDEX IF NOT EXISTS idx_llm_call_evidence_step
    ON llm_call_evidence (step_id, id ASC);

CREATE INDEX IF NOT EXISTS idx_llm_call_evidence_work
    ON llm_call_evidence (work_id, id ASC)
    WHERE work_id IS NOT NULL;

CREATE OR REPLACE FUNCTION prevent_llm_call_evidence_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'LLM call evidence is immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS llm_call_evidence_immutable ON llm_call_evidence;
CREATE TRIGGER llm_call_evidence_immutable
BEFORE UPDATE OR DELETE ON llm_call_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_llm_call_evidence_mutation();
