CREATE TABLE IF NOT EXISTS repository_snapshots (
    id TEXT PRIMARY KEY CHECK (id ~ '^snapshot_[0-9a-f]{64}$'),
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    schema_version TEXT NOT NULL CHECK (btrim(schema_version) <> ''),
    repository_id TEXT NOT NULL CHECK (repository_id ~ '^repository_[0-9a-f]{64}$'),
    root TEXT NOT NULL CHECK (btrim(root) <> ''),
    head_commit TEXT NOT NULL CHECK (head_commit ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
    git_state_sha256 TEXT NOT NULL CHECK (git_state_sha256 ~ '^[0-9a-f]{64}$'),
    dirty BOOLEAN NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX IF NOT EXISTS idx_repository_snapshots_project_generated
    ON repository_snapshots (project_id, generated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS repository_files (
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    file_id TEXT NOT NULL CHECK (file_id ~ '^file_[0-9a-f]{64}$'),
    path TEXT NOT NULL CHECK (btrim(path) <> ''),
    entry_kind TEXT NOT NULL CHECK (entry_kind IN ('regular', 'symlink')),
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    mode_bits INT NOT NULL CHECK (mode_bits BETWEEN 0 AND 511),
    language TEXT NOT NULL DEFAULT '',
    manifest_kind TEXT NOT NULL DEFAULT '',
    is_test BOOLEAN NOT NULL DEFAULT FALSE,
    is_generated BOOLEAN NOT NULL DEFAULT FALSE,
    link_target TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (snapshot_id, path),
    UNIQUE (snapshot_id, file_id)
);

CREATE INDEX IF NOT EXISTS idx_repository_files_snapshot_language
    ON repository_files (snapshot_id, language, file_id);
CREATE INDEX IF NOT EXISTS idx_repository_files_path_trgm
    ON repository_files USING gin (path gin_trgm_ops);

CREATE TABLE IF NOT EXISTS repository_exclusions (
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    path TEXT NOT NULL CHECK (btrim(path) <> ''),
    reason TEXT NOT NULL CHECK (reason IN ('sensitive', 'absent_from_worktree', 'unsupported_entry')),
    PRIMARY KEY (snapshot_id, path)
);

CREATE TABLE IF NOT EXISTS repository_analyses (
    id TEXT PRIMARY KEY CHECK (id ~ '^analysis_[0-9a-f]{64}$'),
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    schema_version TEXT NOT NULL CHECK (btrim(schema_version) <> ''),
    adapter_name TEXT NOT NULL CHECK (btrim(adapter_name) <> ''),
    adapter_version TEXT NOT NULL CHECK (btrim(adapter_version) <> ''),
    complete BOOLEAN NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (snapshot_id, id)
);

CREATE INDEX IF NOT EXISTS idx_repository_analyses_snapshot_generated
    ON repository_analyses (snapshot_id, generated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS repository_analysis_diagnostics (
    analysis_id TEXT NOT NULL REFERENCES repository_analyses(id) ON DELETE CASCADE,
    diagnostic_index INT NOT NULL CHECK (diagnostic_index >= 0),
    severity TEXT NOT NULL CHECK (severity IN ('warning', 'error')),
    subject TEXT NOT NULL CHECK (btrim(subject) <> ''),
    detail TEXT NOT NULL CHECK (btrim(detail) <> ''),
    PRIMARY KEY (analysis_id, diagnostic_index)
);

CREATE TABLE IF NOT EXISTS repository_symbols (
    analysis_id TEXT NOT NULL,
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    symbol_id TEXT NOT NULL CHECK (symbol_id ~ '^symbol_[0-9a-f]{64}$'),
    file_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (btrim(kind) <> ''),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    qualified_name TEXT NOT NULL CHECK (btrim(qualified_name) <> ''),
    signature TEXT NOT NULL DEFAULT '',
    start_byte BIGINT NOT NULL CHECK (start_byte >= 0),
    end_byte BIGINT NOT NULL CHECK (end_byte >= start_byte),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    origin TEXT NOT NULL CHECK (btrim(origin) <> ''),
    adapter_name TEXT NOT NULL CHECK (btrim(adapter_name) <> ''),
    adapter_version TEXT NOT NULL CHECK (btrim(adapter_version) <> ''),
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    PRIMARY KEY (analysis_id, symbol_id),
    FOREIGN KEY (snapshot_id, analysis_id)
        REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE,
    FOREIGN KEY (snapshot_id, file_id)
        REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repository_symbols_name
    ON repository_symbols (analysis_id, qualified_name, kind, symbol_id);
CREATE INDEX IF NOT EXISTS idx_repository_symbols_name_trgm
    ON repository_symbols USING gin (qualified_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_repository_symbols_search
    ON repository_symbols USING gin (
        to_tsvector('simple', qualified_name || ' ' || signature)
    );

CREATE TABLE IF NOT EXISTS repository_artifacts (
    analysis_id TEXT NOT NULL,
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    artifact_id TEXT NOT NULL CHECK (artifact_id ~ '^artifact_[0-9a-f]{64}$'),
    file_id TEXT,
    kind TEXT NOT NULL CHECK (btrim(kind) <> ''),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(detail) = 'object'),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    origin TEXT NOT NULL CHECK (btrim(origin) <> ''),
    adapter_name TEXT NOT NULL CHECK (btrim(adapter_name) <> ''),
    adapter_version TEXT NOT NULL CHECK (btrim(adapter_version) <> ''),
    PRIMARY KEY (analysis_id, artifact_id),
    FOREIGN KEY (snapshot_id, analysis_id)
        REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE,
    FOREIGN KEY (snapshot_id, file_id)
        REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repository_artifacts_kind_name
    ON repository_artifacts (analysis_id, kind, name, artifact_id);

CREATE TABLE IF NOT EXISTS repository_edges (
    analysis_id TEXT NOT NULL,
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    edge_id TEXT NOT NULL CHECK (edge_id ~ '^edge_[0-9a-f]{64}$'),
    from_id TEXT NOT NULL CHECK (btrim(from_id) <> ''),
    to_id TEXT NOT NULL CHECK (btrim(to_id) <> ''),
    kind TEXT NOT NULL CHECK (btrim(kind) <> ''),
    evidence_file_id TEXT,
    evidence_start_byte BIGINT CHECK (evidence_start_byte IS NULL OR evidence_start_byte >= 0),
    evidence_end_byte BIGINT CHECK (
        evidence_end_byte IS NULL OR
        (evidence_start_byte IS NOT NULL AND evidence_end_byte >= evidence_start_byte)
    ),
    CHECK (
        (evidence_file_id IS NULL AND evidence_start_byte IS NULL AND evidence_end_byte IS NULL) OR
        (evidence_file_id IS NOT NULL AND evidence_start_byte IS NOT NULL AND evidence_end_byte IS NOT NULL)
    ),
    origin TEXT NOT NULL CHECK (btrim(origin) <> ''),
    adapter_name TEXT NOT NULL CHECK (btrim(adapter_name) <> ''),
    adapter_version TEXT NOT NULL CHECK (btrim(adapter_version) <> ''),
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    PRIMARY KEY (analysis_id, edge_id),
    FOREIGN KEY (snapshot_id, analysis_id)
        REFERENCES repository_analyses(snapshot_id, id) ON DELETE CASCADE,
    FOREIGN KEY (snapshot_id, evidence_file_id)
        REFERENCES repository_files(snapshot_id, file_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repository_edges_from
    ON repository_edges (analysis_id, from_id, kind, edge_id);
CREATE INDEX IF NOT EXISTS idx_repository_edges_to
    ON repository_edges (analysis_id, to_id, kind, edge_id);

CREATE TABLE IF NOT EXISTS repository_embeddings (
    snapshot_id TEXT NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    subject_id TEXT NOT NULL CHECK (btrim(subject_id) <> ''),
    embedding_provider TEXT NOT NULL CHECK (btrim(embedding_provider) <> ''),
    embedding_model TEXT NOT NULL CHECK (btrim(embedding_model) <> ''),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    embedding vector NOT NULL CHECK (vector_dims(embedding) > 0),
    embedding_sha256 TEXT NOT NULL CHECK (embedding_sha256 ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (snapshot_id, subject_id, embedding_provider, embedding_model)
);

CREATE OR REPLACE FUNCTION prevent_repository_fact_update()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'repository facts are immutable; create a new snapshot';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS repository_snapshots_immutable ON repository_snapshots;
CREATE TRIGGER repository_snapshots_immutable
BEFORE UPDATE ON repository_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_files_immutable ON repository_files;
CREATE TRIGGER repository_files_immutable
BEFORE UPDATE ON repository_files
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_exclusions_immutable ON repository_exclusions;
CREATE TRIGGER repository_exclusions_immutable
BEFORE UPDATE ON repository_exclusions
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_analyses_immutable ON repository_analyses;
CREATE TRIGGER repository_analyses_immutable
BEFORE UPDATE ON repository_analyses
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_analysis_diagnostics_immutable ON repository_analysis_diagnostics;
CREATE TRIGGER repository_analysis_diagnostics_immutable
BEFORE UPDATE ON repository_analysis_diagnostics
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_symbols_immutable ON repository_symbols;
CREATE TRIGGER repository_symbols_immutable
BEFORE UPDATE ON repository_symbols
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_artifacts_immutable ON repository_artifacts;
CREATE TRIGGER repository_artifacts_immutable
BEFORE UPDATE ON repository_artifacts
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_edges_immutable ON repository_edges;
CREATE TRIGGER repository_edges_immutable
BEFORE UPDATE ON repository_edges
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();

DROP TRIGGER IF EXISTS repository_embeddings_immutable ON repository_embeddings;
CREATE TRIGGER repository_embeddings_immutable
BEFORE UPDATE ON repository_embeddings
FOR EACH ROW EXECUTE FUNCTION prevent_repository_fact_update();
