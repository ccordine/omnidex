ALTER TABLE data_sources
    ADD COLUMN execution_mode TEXT,
    ADD COLUMN authority_url TEXT,
    ADD COLUMN credential_env TEXT;

UPDATE data_sources
SET execution_mode='direct', authority_url='', credential_env='';

ALTER TABLE data_sources
    ALTER COLUMN execution_mode SET NOT NULL,
    ALTER COLUMN authority_url SET NOT NULL,
    ALTER COLUMN credential_env SET NOT NULL,
    DROP CONSTRAINT data_sources_port_check,
    DROP CONSTRAINT data_sources_ssl_mode_check,
    DROP CONSTRAINT data_sources_connection_shape_check,
    DROP CONSTRAINT data_sources_schema_snapshot_shape_check;

ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_execution_mode_check CHECK (
        execution_mode IN ('direct','delegated')
    ),
    ADD CONSTRAINT data_sources_execution_authority_shape_check CHECK (
        (
            execution_mode='direct' AND
            port BETWEEN 1 AND 65535 AND
            ssl_mode IN ('disable','allow','prefer','require','verify-ca','verify-full') AND
            authority_url='' AND credential_env='' AND
            host=btrim(host) AND database_name=btrim(database_name) AND
            username=btrim(username) AND dsn=btrim(dsn) AND
            (
                (use_dsn AND dsn<>'') OR
                (NOT use_dsn AND host<>'' AND database_name<>'' AND username<>'')
            )
        ) OR (
            execution_mode='delegated' AND
            host='' AND port=0 AND database_name='' AND username='' AND
            password='' AND ssl_mode='' AND NOT use_dsn AND dsn='' AND
            authority_url=btrim(authority_url) AND
            length(authority_url) BETWEEN 8 AND 2048 AND
            (authority_url LIKE 'http://%' OR authority_url LIKE 'https://%') AND
            credential_env ~ '^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$'
        )
    ),
    ADD CONSTRAINT data_sources_schema_snapshot_shape_check CHECK (
        schema_catalog IS NULL OR (
            execution_mode='direct' AND
            jsonb_typeof(schema_catalog)='object' AND
            schema_catalog ?& ARRAY[
                'schema','source_id','source_name','driver','fingerprint','captured_at','relations'
            ]::text[] AND
            schema_catalog - ARRAY[
                'schema','source_id','source_name','driver','fingerprint','captured_at','relations'
            ]::text[] = '{}'::jsonb AND
            schema_catalog->>'schema'='omnidex.datasource-schema.v1' AND
            schema_catalog->>'source_id'=id AND
            jsonb_typeof(schema_catalog->'source_name')='string' AND
            schema_catalog->>'driver'='postgres' AND
            schema_catalog->>'fingerprint' ~ '^[0-9a-f]{64}$' AND
            jsonb_typeof(schema_catalog->'captured_at')='string' AND
            jsonb_typeof(schema_catalog->'relations')='array' AND
            jsonb_array_length(schema_catalog->'relations')>0
        )
    ),
    ADD CONSTRAINT data_sources_delegated_catalog_absent_check CHECK (
        execution_mode='direct' OR (schema_catalog IS NULL AND catalog_updated_at IS NULL)
    );
