CREATE TABLE IF NOT EXISTS worker_skills (
    skill_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('candidate', 'validating', 'active', 'rejected', 'retired')),
    origin TEXT NOT NULL CHECK (origin IN ('bootstrap', 'learned')),
    skill_kind TEXT NOT NULL CHECK (skill_kind IN ('bootstrap_specialist', 'code_procedure')),
    purpose TEXT NOT NULL CHECK (BTRIM(purpose) <> ''),
    instructions TEXT NOT NULL CHECK (BTRIM(instructions) <> ''),
    preferred_models TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    allowed_tools TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    forbidden_tools TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    context_budget INTEGER NOT NULL CHECK (context_budget >= 0),
    input_schema JSONB,
    output_schema JSONB,
    stop_conditions TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    retry_policy TEXT NOT NULL DEFAULT '',
    require_evidence BOOLEAN NOT NULL DEFAULT FALSE,
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    created_by_job_id BIGINT REFERENCES jobs(id) ON DELETE RESTRICT,
    validation JSONB NOT NULL DEFAULT '[]'::JSONB,
    activated_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    retired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_id, version),
    CHECK (origin <> 'learned' OR created_by_job_id IS NOT NULL),
    CHECK (origin <> 'bootstrap' OR created_by_job_id IS NULL),
    CHECK (status NOT IN ('active', 'rejected') OR JSONB_TYPEOF(validation) = 'array'),
    CHECK (status <> 'active' OR JSONB_ARRAY_LENGTH(validation) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_skills_one_active_version
    ON worker_skills (skill_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_worker_skills_active_created
    ON worker_skills (created_at DESC, skill_id ASC)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS worker_skill_embeddings (
    skill_id TEXT NOT NULL,
    skill_version INTEGER NOT NULL,
    embedding_provider TEXT NOT NULL CHECK (BTRIM(embedding_provider) <> ''),
    embedding_model TEXT NOT NULL CHECK (BTRIM(embedding_model) <> ''),
    embedding vector NOT NULL CHECK (vector_dims(embedding) > 0),
    embedding_sha256 TEXT NOT NULL CHECK (embedding_sha256 ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_id, skill_version, embedding_provider, embedding_model),
    FOREIGN KEY (skill_id, skill_version)
        REFERENCES worker_skills(skill_id, version) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_worker_skill_embeddings_model
    ON worker_skill_embeddings (embedding_provider, embedding_model, skill_id, skill_version);

CREATE TABLE IF NOT EXISTS worker_skill_dependencies (
    skill_id TEXT NOT NULL,
    skill_version INTEGER NOT NULL,
    dependency_skill_id TEXT NOT NULL,
    dependency_version INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_id, skill_version, dependency_skill_id, dependency_version),
    FOREIGN KEY (skill_id, skill_version)
        REFERENCES worker_skills(skill_id, version) ON DELETE CASCADE,
    FOREIGN KEY (dependency_skill_id, dependency_version)
        REFERENCES worker_skills(skill_id, version) ON DELETE RESTRICT,
    CHECK (skill_id <> dependency_skill_id OR skill_version <> dependency_version)
);

CREATE TABLE IF NOT EXISTS worker_skill_checks (
    id BIGSERIAL PRIMARY KEY,
    skill_id TEXT NOT NULL,
    skill_version INTEGER NOT NULL,
    check_name TEXT NOT NULL CHECK (BTRIM(check_name) <> ''),
    status TEXT NOT NULL CHECK (status IN ('passed', 'failed')),
    detail TEXT NOT NULL CHECK (BTRIM(detail) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (skill_id, skill_version)
        REFERENCES worker_skills(skill_id, version) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_worker_skill_checks_version
    ON worker_skill_checks (skill_id, skill_version, id ASC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_skill_checks_unique_name
    ON worker_skill_checks (skill_id, skill_version, check_name);

CREATE OR REPLACE FUNCTION prevent_worker_skill_content_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(
        OLD.skill_id, OLD.version, OLD.origin, OLD.skill_kind, OLD.purpose, OLD.instructions,
        OLD.preferred_models, OLD.allowed_tools, OLD.forbidden_tools,
        OLD.context_budget, OLD.input_schema, OLD.output_schema,
        OLD.stop_conditions, OLD.retry_policy, OLD.require_evidence,
        OLD.content_sha256, OLD.created_by_job_id, OLD.created_at
    ) IS DISTINCT FROM ROW(
        NEW.skill_id, NEW.version, NEW.origin, NEW.skill_kind, NEW.purpose, NEW.instructions,
        NEW.preferred_models, NEW.allowed_tools, NEW.forbidden_tools,
        NEW.context_budget, NEW.input_schema, NEW.output_schema,
        NEW.stop_conditions, NEW.retry_policy, NEW.require_evidence,
        NEW.content_sha256, NEW.created_by_job_id, NEW.created_at
    ) THEN
        RAISE EXCEPTION 'worker skill content is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS worker_skills_immutable_content ON worker_skills;
CREATE TRIGGER worker_skills_immutable_content
BEFORE UPDATE ON worker_skills
FOR EACH ROW EXECUTE FUNCTION prevent_worker_skill_content_update();

CREATE OR REPLACE FUNCTION prevent_worker_skill_embedding_update()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'worker skill embeddings are immutable; insert a distinct model identity';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS worker_skill_embeddings_immutable ON worker_skill_embeddings;
CREATE TRIGGER worker_skill_embeddings_immutable
BEFORE UPDATE ON worker_skill_embeddings
FOR EACH ROW EXECUTE FUNCTION prevent_worker_skill_embedding_update();
