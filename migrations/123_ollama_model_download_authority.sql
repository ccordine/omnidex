CREATE TABLE ollama_model_downloads (
    id TEXT PRIMARY KEY,
    model TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'queued',
    status TEXT NOT NULL DEFAULT 'Queued',
    digest TEXT NOT NULL DEFAULT '',
    total_bytes BIGINT NOT NULL DEFAULT 0,
    completed_bytes BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    CONSTRAINT ollama_model_downloads_identity_check CHECK (
        id ~ '^omd_[0-9a-f]{32}$'
    ),
    CONSTRAINT ollama_model_downloads_model_check CHECK (
        octet_length(model) BETWEEN 1 AND 256 AND model=btrim(model) AND
        model ~ '^[A-Za-z0-9._:/@-]+$'
    ),
    CONSTRAINT ollama_model_downloads_state_check CHECK (
        state IN ('queued','running','completed','failed')
    ),
    CONSTRAINT ollama_model_downloads_text_check CHECK (
        octet_length(status) BETWEEN 1 AND 512 AND status=btrim(status) AND
        octet_length(digest) <= 256 AND digest=btrim(digest) AND
        octet_length(error) <= 2048 AND error=btrim(error)
    ),
    CONSTRAINT ollama_model_downloads_progress_check CHECK (
        total_bytes >= 0 AND completed_bytes >= 0 AND
        (total_bytes=0 OR completed_bytes <= total_bytes)
    ),
    CONSTRAINT ollama_model_downloads_lifecycle_check CHECK (
        (state='queued' AND started_at IS NULL AND finished_at IS NULL AND error='') OR
        (state='running' AND started_at IS NOT NULL AND finished_at IS NULL AND error='') OR
        (state='completed' AND started_at IS NOT NULL AND finished_at IS NOT NULL AND error='') OR
        (state='failed' AND finished_at IS NOT NULL AND error<>'')
    )
);

CREATE UNIQUE INDEX idx_ollama_model_downloads_one_active_model
    ON ollama_model_downloads(model)
    WHERE state IN ('queued','running');
CREATE INDEX idx_ollama_model_downloads_recent
    ON ollama_model_downloads(created_at DESC,id DESC);

CREATE FUNCTION validate_ollama_model_download_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR
       NEW.model IS DISTINCT FROM OLD.model OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'Ollama model download identity is immutable';
    END IF;
    IF OLD.state IN ('completed','failed') THEN
        RAISE EXCEPTION 'terminal Ollama model download is immutable';
    END IF;
    IF OLD.state='queued' AND NEW.state NOT IN ('running','failed') THEN
        RAISE EXCEPTION 'invalid queued Ollama model download transition';
    END IF;
    IF OLD.state='running' AND NEW.state NOT IN ('running','completed','failed') THEN
        RAISE EXCEPTION 'invalid running Ollama model download transition';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ollama_model_downloads_transition_guard
BEFORE UPDATE ON ollama_model_downloads
FOR EACH ROW EXECUTE FUNCTION validate_ollama_model_download_transition();

CREATE FUNCTION reject_ollama_model_download_removal()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Ollama model download history is durable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ollama_model_downloads_delete_rejected
BEFORE DELETE ON ollama_model_downloads
FOR EACH ROW EXECUTE FUNCTION reject_ollama_model_download_removal();
CREATE TRIGGER ollama_model_downloads_truncate_rejected
BEFORE TRUNCATE ON ollama_model_downloads
FOR EACH STATEMENT EXECUTE FUNCTION reject_ollama_model_download_removal();

