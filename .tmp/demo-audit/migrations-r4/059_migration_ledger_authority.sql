LOCK TABLE schema_migrations IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM schema_migrations
        WHERE filename !~ '^[0-9]{3}_[A-Za-z0-9_]+[.]sql$'
           OR octet_length(filename)>255
           OR sha256 !~ '^[0-9a-f]{64}$'
           OR manifest_sha256 !~ '^[0-9a-f]{64}$'
    ) THEN
        RAISE EXCEPTION 'schema migration ledger contains invalid authority';
    END IF;
    IF (SELECT COUNT(DISTINCT manifest_sha256) FROM schema_migrations)<>1 THEN
        RAISE EXCEPTION 'schema migration ledger is not bound to one exact manifest';
    END IF;
END;
$$;

ALTER TABLE schema_migrations
    ADD CONSTRAINT schema_migrations_filename_exact CHECK (
        filename ~ '^[0-9]{3}_[A-Za-z0-9_]+[.]sql$' AND
        octet_length(filename)<=255
    ),
    ADD CONSTRAINT schema_migrations_sha256_exact CHECK (
        sha256 ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT schema_migrations_manifest_sha256_exact CHECK (
        manifest_sha256 ~ '^[0-9a-f]{64}$'
    );

CREATE TABLE schema_migration_bundle_authority (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    manifest_sha256 TEXT NOT NULL UNIQUE CHECK (
        manifest_sha256 ~ '^[0-9a-f]{64}$'
    ),
    installed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

INSERT INTO schema_migration_bundle_authority (singleton,manifest_sha256)
SELECT TRUE,MIN(manifest_sha256) FROM schema_migrations;

ALTER TABLE schema_migrations
    ADD CONSTRAINT schema_migrations_current_manifest_fkey
    FOREIGN KEY (manifest_sha256)
    REFERENCES schema_migration_bundle_authority(manifest_sha256)
    ON UPDATE CASCADE ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION enforce_schema_migration_history()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'schema migration history is immutable';
    END IF;
    IF ROW(OLD.filename,OLD.sha256,OLD.applied_at)
       IS DISTINCT FROM ROW(NEW.filename,NEW.sha256,NEW.applied_at) THEN
        RAISE EXCEPTION 'schema migration identity is immutable';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM schema_migration_bundle_authority
        WHERE singleton AND manifest_sha256=NEW.manifest_sha256
    ) THEN
        RAISE EXCEPTION 'schema migration has no current manifest authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER schema_migrations_update_exact
BEFORE UPDATE ON schema_migrations
FOR EACH ROW EXECUTE FUNCTION enforce_schema_migration_history();
CREATE TRIGGER schema_migrations_delete_immutable
BEFORE DELETE ON schema_migrations
FOR EACH ROW EXECUTE FUNCTION enforce_schema_migration_history();

CREATE OR REPLACE FUNCTION prevent_schema_migration_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'schema migration history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER schema_migrations_truncate_immutable
BEFORE TRUNCATE ON schema_migrations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_schema_migration_truncate();

CREATE OR REPLACE FUNCTION enforce_schema_migration_bundle_authority()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP='DELETE' OR NEW.singleton IS DISTINCT FROM OLD.singleton OR
       NEW.installed_at IS DISTINCT FROM OLD.installed_at THEN
        RAISE EXCEPTION 'schema migration bundle authority identity is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER schema_migration_bundle_authority_update_exact
BEFORE UPDATE ON schema_migration_bundle_authority
FOR EACH ROW EXECUTE FUNCTION enforce_schema_migration_bundle_authority();
CREATE TRIGGER schema_migration_bundle_authority_delete_immutable
BEFORE DELETE ON schema_migration_bundle_authority
FOR EACH ROW EXECUTE FUNCTION enforce_schema_migration_bundle_authority();
CREATE TRIGGER schema_migration_bundle_authority_truncate_immutable
BEFORE TRUNCATE ON schema_migration_bundle_authority
FOR EACH STATEMENT EXECUTE FUNCTION prevent_schema_migration_truncate();
