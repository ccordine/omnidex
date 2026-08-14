LOCK TABLE repository_mutation_operations, repository_mutation_files,
    repository_snapshots, repository_files, repository_exclusions
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM repository_mutation_operations
        WHERE status IN ('prepared', 'applying', 'indeterminate')
    ) THEN
        RAISE EXCEPTION 'unresolved repository mutations must be recovered before file-state migration';
    END IF;
END;
$$;

DROP TRIGGER repository_mutation_files_immutable ON repository_mutation_files;
DROP TRIGGER repository_mutation_files_truncate_immutable ON repository_mutation_files;
DROP TRIGGER repository_mutation_file_source_validate ON repository_mutation_files;

ALTER TABLE repository_mutation_operations
    DROP CONSTRAINT repository_mutation_operations_contract_id_check,
    ADD CONSTRAINT repository_mutation_operations_contract_id_check CHECK (
        contract_id ~ '^change_contract_[0-9a-f]{64}$'
        OR contract_id ~ '^desired_graph_[0-9a-f]{64}$'
    );

ALTER TABLE repository_mutation_files
    ADD COLUMN source_present BOOLEAN,
    ADD COLUMN source_mode INT,
    ADD COLUMN expected_present BOOLEAN,
    ADD COLUMN expected_mode INT,
    ALTER COLUMN source_sha256 DROP NOT NULL,
    ALTER COLUMN source_size DROP NOT NULL,
    ALTER COLUMN expected_sha256 DROP NOT NULL,
    ALTER COLUMN expected_size DROP NOT NULL;

UPDATE repository_mutation_files AS mutation
SET source_present=TRUE,
    source_mode=source.mode_bits,
    expected_present=TRUE,
    expected_mode=source.mode_bits
FROM repository_mutation_operations AS operation
JOIN repository_files AS source
  ON source.snapshot_id=operation.source_snapshot_id
WHERE mutation.operation_id=operation.id
  AND source.file_id=mutation.file_id
  AND source.path=mutation.path
  AND source.entry_kind='regular'
  AND source.content_sha256=mutation.source_sha256
  AND source.size_bytes=mutation.source_size;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM repository_mutation_files
        WHERE source_present IS NULL OR source_mode IS NULL
           OR expected_present IS NULL OR expected_mode IS NULL
    ) THEN
        RAISE EXCEPTION 'legacy repository mutation file authority cannot be backfilled exactly';
    END IF;
END;
$$;

ALTER TABLE repository_mutation_files
    ALTER COLUMN source_present SET NOT NULL,
    ALTER COLUMN expected_present SET NOT NULL,
    DROP CONSTRAINT repository_mutation_files_check,
    ADD CONSTRAINT repository_mutation_files_source_state_check CHECK (
        (source_present AND source_sha256 ~ '^[0-9a-f]{64}$'
            AND source_size >= 0 AND source_mode BETWEEN 0 AND 511)
        OR
        (NOT source_present AND source_sha256 IS NULL
            AND source_size IS NULL AND source_mode IS NULL)
    ),
    ADD CONSTRAINT repository_mutation_files_expected_state_check CHECK (
        (expected_present AND expected_sha256 ~ '^[0-9a-f]{64}$'
            AND expected_size >= 0 AND expected_mode BETWEEN 0 AND 511)
        OR
        (NOT expected_present AND expected_sha256 IS NULL
            AND expected_size IS NULL AND expected_mode IS NULL)
    ),
    ADD CONSTRAINT repository_mutation_files_transition_check CHECK (
        source_present OR expected_present
    ),
    ADD CONSTRAINT repository_mutation_files_changed_state_check CHECK (
        ROW(source_present, source_sha256, source_size, source_mode)
        IS DISTINCT FROM
        ROW(expected_present, expected_sha256, expected_size, expected_mode)
    );

CREATE OR REPLACE FUNCTION validate_repository_mutation_file_source()
RETURNS TRIGGER AS $$
DECLARE
    source_snapshot TEXT;
    source_repository TEXT;
    operation_sealed_at TIMESTAMPTZ;
    matches_source BOOLEAN;
BEGIN
    SELECT operation.source_snapshot_id, snapshot.repository_id, operation.sealed_at
    INTO source_snapshot, source_repository, operation_sealed_at
    FROM repository_mutation_operations AS operation
    JOIN repository_snapshots AS snapshot ON snapshot.id=operation.source_snapshot_id
    WHERE operation.id=NEW.operation_id
    FOR UPDATE OF operation;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'repository mutation file has no operation';
    END IF;
    IF operation_sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'repository mutation file authority is sealed';
    END IF;
    IF NEW.source_present THEN
        SELECT EXISTS (
            SELECT 1 FROM repository_files
            WHERE snapshot_id=source_snapshot AND file_id=NEW.file_id
              AND path=NEW.path AND entry_kind='regular'
              AND content_sha256=NEW.source_sha256 AND size_bytes=NEW.source_size
              AND mode_bits=NEW.source_mode
        ) INTO matches_source;
        IF matches_source IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'repository mutation file disagrees with immutable source snapshot';
        END IF;
    ELSE
        SELECT NOT EXISTS (
            SELECT 1 FROM repository_files
            WHERE snapshot_id=source_snapshot AND (file_id=NEW.file_id OR path=NEW.path)
        ) AND NOT EXISTS (
            SELECT 1 FROM repository_exclusions
            WHERE snapshot_id=source_snapshot AND path=NEW.path
        ) AND NEW.file_id = 'file_' || encode(digest(
            convert_to(source_repository, 'UTF8') || decode('00', 'hex') ||
            convert_to(NEW.path, 'UTF8') || decode('00', 'hex'), 'sha256'
        ), 'hex') INTO matches_source;
        IF matches_source IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'repository mutation source absence disagrees with immutable source snapshot';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER repository_mutation_file_source_validate
BEFORE INSERT ON repository_mutation_files
FOR EACH ROW EXECUTE FUNCTION validate_repository_mutation_file_source();

CREATE TRIGGER repository_mutation_files_immutable
BEFORE UPDATE OR DELETE ON repository_mutation_files
FOR EACH ROW EXECUTE FUNCTION prevent_repository_mutation_file_change();
CREATE TRIGGER repository_mutation_files_truncate_immutable
BEFORE TRUNCATE ON repository_mutation_files
FOR EACH STATEMENT EXECUTE FUNCTION prevent_repository_mutation_file_change();
