CREATE TABLE repository_mutation_operations (
    id TEXT PRIMARY KEY
        CHECK (id ~ '^repository_mutation_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL
        CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    job_id BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation > 0),
    worker_id TEXT NOT NULL
        CHECK (worker_id <> '' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    contract_id TEXT NOT NULL
        CHECK (contract_id ~ '^change_contract_[0-9a-f]{64}$'),
    stage_id TEXT NOT NULL
        CHECK (stage_id ~ '^repository_change_stage_[0-9a-f]{64}$'),
    source_snapshot_id TEXT NOT NULL,
    patch TEXT NOT NULL CHECK (octet_length(patch) BETWEEN 1 AND 1048576),
    patch_sha256 TEXT NOT NULL
        CHECK (patch_sha256 ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL
        CHECK (status IN ('prepared', 'applying', 'applied', 'indeterminate')),
    attempt_count INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error TEXT,
    evidence_id BIGINT,
    prepared_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sealed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ,
    CHECK (id = 'repository_mutation_' || command_sha256),
    CHECK (patch_sha256 = encode(digest(convert_to(patch, 'UTF8'), 'sha256'), 'hex')),
    CHECK (last_error IS NULL OR (last_error <> '' AND last_error=BTRIM(last_error))),
    CHECK ((status = 'applied') = (evidence_id IS NOT NULL)),
    CHECK ((status = 'applied') = (applied_at IS NOT NULL)),
    CHECK (status <> 'indeterminate' OR last_error IS NOT NULL),
    UNIQUE (job_id, stage_id),
    UNIQUE (job_id, id),
    FOREIGN KEY (job_id, generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, generation, step_id)
        REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT,
    FOREIGN KEY (source_snapshot_id)
        REFERENCES repository_snapshots(id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, evidence_id)
        REFERENCES evidence(job_id, id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX idx_repository_mutations_unresolved_generation
    ON repository_mutation_operations (job_id, generation)
    WHERE status IN ('prepared', 'applying', 'indeterminate');

CREATE TABLE repository_mutation_files (
    operation_id TEXT NOT NULL
        REFERENCES repository_mutation_operations(id) ON DELETE RESTRICT,
    ordinal INT NOT NULL CHECK (ordinal BETWEEN 0 AND 7),
    file_id TEXT NOT NULL CHECK (file_id ~ '^file_[0-9a-f]{64}$'),
    path TEXT NOT NULL
        CHECK (path <> '' AND path=BTRIM(path) AND octet_length(path)<=4096),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    source_size BIGINT NOT NULL CHECK (source_size >= 0),
    expected_sha256 TEXT NOT NULL CHECK (expected_sha256 ~ '^[0-9a-f]{64}$'),
    expected_size BIGINT NOT NULL CHECK (expected_size >= 0),
    PRIMARY KEY (operation_id, ordinal),
    UNIQUE (operation_id, file_id),
    UNIQUE (operation_id, path),
    CHECK (source_sha256 <> expected_sha256 OR source_size <> expected_size)
);

CREATE OR REPLACE FUNCTION validate_repository_mutation_source()
RETURNS TRIGGER AS $$
DECLARE
    matches_source BOOLEAN;
BEGIN
    SELECT jobs.project_id IS NOT NULL AND jobs.project_id=snapshot.project_id
    INTO matches_source
    FROM jobs
    JOIN repository_snapshots AS snapshot ON snapshot.id=NEW.source_snapshot_id
    WHERE jobs.id=NEW.job_id;
    IF matches_source IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'repository mutation source snapshot is not owned by its job project';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_source_validate
BEFORE INSERT ON repository_mutation_operations
FOR EACH ROW EXECUTE FUNCTION validate_repository_mutation_source();

CREATE OR REPLACE FUNCTION validate_repository_mutation_file_source()
RETURNS TRIGGER AS $$
DECLARE
    source_snapshot TEXT;
    operation_sealed_at TIMESTAMPTZ;
    matches_source BOOLEAN;
BEGIN
    SELECT source_snapshot_id, sealed_at INTO source_snapshot, operation_sealed_at
    FROM repository_mutation_operations WHERE id=NEW.operation_id
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'repository mutation file has no operation';
    END IF;
    IF operation_sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'repository mutation file authority is sealed';
    END IF;
    SELECT EXISTS (
        SELECT 1 FROM repository_files
        WHERE snapshot_id=source_snapshot AND file_id=NEW.file_id
          AND path=NEW.path AND entry_kind='regular'
          AND content_sha256=NEW.source_sha256 AND size_bytes=NEW.source_size
    ) INTO matches_source;
    IF matches_source IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'repository mutation file disagrees with immutable source snapshot';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_file_source_validate
BEFORE INSERT ON repository_mutation_files
FOR EACH ROW EXECUTE FUNCTION validate_repository_mutation_file_source();

CREATE OR REPLACE FUNCTION require_repository_mutation_files()
RETURNS TRIGGER AS $$
DECLARE
    file_count INT;
    operation_sealed_at TIMESTAMPTZ;
BEGIN
    SELECT sealed_at INTO operation_sealed_at
    FROM repository_mutation_operations WHERE id=NEW.id;
    SELECT COUNT(*) INTO file_count FROM repository_mutation_files WHERE operation_id=NEW.id;
    IF operation_sealed_at IS NULL OR file_count < 1 OR file_count > 8 THEN
        RAISE EXCEPTION 'repository mutation must be sealed with 1-8 exact files';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER repository_mutation_requires_files
AFTER INSERT ON repository_mutation_operations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_repository_mutation_files();

CREATE OR REPLACE FUNCTION prevent_repository_mutation_identity_change()
RETURNS TRIGGER AS $$
DECLARE
    valid_evidence BOOLEAN;
BEGIN
    IF (OLD.id, OLD.command_sha256, OLD.job_id, OLD.step_id, OLD.generation,
        OLD.worker_id, OLD.contract_id, OLD.stage_id, OLD.source_snapshot_id,
        OLD.patch, OLD.patch_sha256, OLD.prepared_at)
       IS DISTINCT FROM
       (NEW.id, NEW.command_sha256, NEW.job_id, NEW.step_id, NEW.generation,
        NEW.worker_id, NEW.contract_id, NEW.stage_id, NEW.source_snapshot_id,
        NEW.patch, NEW.patch_sha256, NEW.prepared_at) THEN
        RAISE EXCEPTION 'repository mutation identity is immutable';
    END IF;
    IF OLD.sealed_at IS NULL AND NEW.sealed_at IS NOT NULL THEN
        IF OLD.status <> 'prepared' OR NEW.status <> 'prepared' OR
           OLD.attempt_count <> 0 OR NEW.attempt_count <> 0 OR
           ROW(OLD.last_error, OLD.evidence_id, OLD.updated_at, OLD.applied_at)
             IS DISTINCT FROM
           ROW(NEW.last_error, NEW.evidence_id, NEW.updated_at, NEW.applied_at) THEN
            RAISE EXCEPTION 'repository mutation sealing transition is invalid';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.sealed_at IS DISTINCT FROM OLD.sealed_at THEN
        RAISE EXCEPTION 'repository mutation file seal is immutable';
    END IF;
    IF OLD.status = 'applied' AND ROW(
        OLD.status, OLD.attempt_count, OLD.last_error, OLD.evidence_id,
        OLD.updated_at, OLD.applied_at
    ) IS DISTINCT FROM ROW(
        NEW.status, NEW.attempt_count, NEW.last_error, NEW.evidence_id,
        NEW.updated_at, NEW.applied_at
    ) THEN
        RAISE EXCEPTION 'applied repository mutation is terminal';
    END IF;
    IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','applied','indeterminate')) OR
       (OLD.status='applying' AND NEW.status NOT IN ('applying','prepared','applied','indeterminate')) OR
       (OLD.status='indeterminate' AND NEW.status NOT IN ('applying','applied','indeterminate')) THEN
        RAISE EXCEPTION 'invalid repository mutation status transition from % to %', OLD.status, NEW.status;
    END IF;
    IF (NEW.status='applying' AND NEW.attempt_count <> OLD.attempt_count+1) OR
       (NEW.status<>'applying' AND NEW.attempt_count <> OLD.attempt_count) THEN
        RAISE EXCEPTION 'repository mutation attempt transition is invalid';
    END IF;
    IF NEW.status = 'applied' THEN
        SELECT EXISTS (
            SELECT 1 FROM evidence
            WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id
              AND kind='generated_diff' AND source_type='repository'
              AND source_ref=NEW.stage_id
              AND payload_json->>'hash'=NEW.patch_sha256
              AND payload_json->'metadata'->>'repository_mutation_operation_id'=NEW.id
        ) INTO valid_evidence;
        IF valid_evidence IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'applied repository mutation has invalid generated-diff evidence';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_identity_immutable
BEFORE UPDATE ON repository_mutation_operations
FOR EACH ROW EXECUTE FUNCTION prevent_repository_mutation_identity_change();

CREATE OR REPLACE FUNCTION prevent_repository_mutation_file_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'repository mutation file authority is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_files_immutable
BEFORE UPDATE OR DELETE ON repository_mutation_files
FOR EACH ROW EXECUTE FUNCTION prevent_repository_mutation_file_change();
CREATE TRIGGER repository_mutation_files_truncate_immutable
BEFORE TRUNCATE ON repository_mutation_files
FOR EACH STATEMENT EXECUTE FUNCTION prevent_repository_mutation_file_change();

CREATE OR REPLACE FUNCTION prevent_repository_mutation_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'repository mutation journal is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_operations_delete_immutable
BEFORE DELETE ON repository_mutation_operations
FOR EACH ROW EXECUTE FUNCTION prevent_repository_mutation_delete();
CREATE TRIGGER repository_mutation_operations_truncate_immutable
BEFORE TRUNCATE ON repository_mutation_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_repository_mutation_delete();

CREATE OR REPLACE FUNCTION prevent_repository_mutation_evidence_change()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM repository_mutation_operations WHERE evidence_id=OLD.id
    ) THEN
        RAISE EXCEPTION 'accepted repository mutation evidence is immutable';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_evidence_immutable
BEFORE UPDATE OR DELETE ON evidence
FOR EACH ROW EXECUTE FUNCTION prevent_repository_mutation_evidence_change();
