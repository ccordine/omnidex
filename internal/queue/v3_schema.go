package queue

const v3SchemaSQL = `
CREATE TABLE IF NOT EXISTS artifacts (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    step_id BIGINT REFERENCES job_steps(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    version TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_artifacts_job_step_kind ON artifacts(job_id, step_id, kind, id DESC);
CREATE INDEX IF NOT EXISTS idx_artifacts_kind_created ON artifacts(kind, created_at DESC);

CREATE TABLE IF NOT EXISTS evidence (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    step_id BIGINT REFERENCES job_steps(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    source_type TEXT,
    source_ref TEXT,
    payload_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evidence_job_step_kind ON evidence(job_id, step_id, kind, id DESC);
CREATE INDEX IF NOT EXISTS idx_evidence_kind_created ON evidence(kind, created_at DESC);

CREATE TABLE IF NOT EXISTS memory_candidates (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES jobs(id) ON DELETE SET NULL,
    source_memory_id BIGINT REFERENCES memory_chunks(id) ON DELETE SET NULL,
    candidate_kind TEXT NOT NULL,
    content TEXT NOT NULL,
    provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'candidate',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_candidates_status_created ON memory_candidates(status, created_at DESC);


CREATE TABLE IF NOT EXISTS claims (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    step_id BIGINT REFERENCES job_steps(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    normalized_text TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'unsupported',
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_claims_job_step ON claims(job_id, step_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_claims_status_created ON claims(status, created_at DESC);

CREATE TABLE IF NOT EXISTS claim_support (
    id BIGSERIAL PRIMARY KEY,
    claim_id BIGINT NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
    evidence_id BIGINT NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,
    support_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    rationale TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (claim_id, evidence_id)
);

CREATE INDEX IF NOT EXISTS idx_claim_support_claim ON claim_support(claim_id, support_score DESC);
`
