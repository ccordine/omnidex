LOCK TABLE workspace_settings, data_source_channels, ai_channels IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_item TEXT;
    duplicate_id TEXT;
    invalid_catalog TEXT;
    invalid_channel TEXT;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM workspace_settings
        WHERE key='data_sources' AND jsonb_typeof(value) IS DISTINCT FROM 'array'
    ) THEN
        RAISE EXCEPTION 'cannot install relational data-source authority: legacy data_sources value is not an array';
    END IF;

    SELECT format('ordinal %s', item.ordinal)
    INTO invalid_item
    FROM workspace_settings AS settings
    CROSS JOIN LATERAL jsonb_array_elements(settings.value)
        WITH ORDINALITY AS item(element, ordinal)
    WHERE settings.key='data_sources'
      AND (
          jsonb_typeof(item.element) IS DISTINCT FROM 'object' OR
          NOT (item.element ?& ARRAY[
              'id','name','driver','host','port','database_name','username','ssl_mode','use_dsn',
              'read_only','last_test_status','last_test_message','created_at','updated_at'
          ]::text[]) OR
          -- These three retired keys are accepted only as legacy input and are discarded.
          EXISTS (
              SELECT 1
              FROM jsonb_object_keys(item.element) AS field(name)
              WHERE field.name <> ALL (ARRAY[
                  'id','name','driver','domain','context_prompt','privacy_mode',
                  'host','port','database_name','username','password','ssl_mode',
                  'use_dsn','dsn','read_only','last_test_status','last_test_message',
                  'last_test_at','catalog_updated_at','created_at','updated_at'
              ]::text[])
          ) OR
          jsonb_typeof(item.element->'id') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'name') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'driver') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'host') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'port') IS DISTINCT FROM 'number' OR
          (item.element->>'port') !~ '^[0-9]{1,5}$' OR
          (item.element->>'port')::integer NOT BETWEEN 1 AND 65535 OR
          jsonb_typeof(item.element->'database_name') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'username') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'ssl_mode') IS DISTINCT FROM 'string' OR
          item.element->>'ssl_mode' NOT IN (
              'disable','allow','prefer','require','verify-ca','verify-full'
          ) OR
          jsonb_typeof(item.element->'use_dsn') IS DISTINCT FROM 'boolean' OR
          jsonb_typeof(item.element->'read_only') IS DISTINCT FROM 'boolean' OR
          jsonb_typeof(item.element->'last_test_status') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'last_test_message') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'created_at') IS DISTINCT FROM 'string' OR
          jsonb_typeof(item.element->'updated_at') IS DISTINCT FROM 'string' OR
          (item.element ? 'password' AND jsonb_typeof(item.element->'password') IS DISTINCT FROM 'string') OR
          (item.element ? 'dsn' AND jsonb_typeof(item.element->'dsn') IS DISTINCT FROM 'string') OR
          (
              (item.element->>'use_dsn')::boolean AND
              (NOT (item.element ? 'dsn') OR btrim(item.element->>'dsn')='')
          ) OR
          (
              NOT (item.element->>'use_dsn')::boolean AND
              (
                  btrim(item.element->>'host')='' OR
                  btrim(item.element->>'database_name')='' OR
                  btrim(item.element->>'username')=''
              )
          ) OR
          (item.element ? 'last_test_at' AND jsonb_typeof(item.element->'last_test_at') IS DISTINCT FROM 'string') OR
          (item.element ? 'catalog_updated_at' AND jsonb_typeof(item.element->'catalog_updated_at') IS DISTINCT FROM 'string') OR
          item.element->>'id' !~ '^[a-z0-9][a-z0-9_.:-]{0,127}$' OR
          btrim(item.element->>'name')='' OR
          item.element->>'name' <> btrim(item.element->>'name') OR
          item.element->>'driver' <> 'postgres' OR
          (item.element->>'read_only')::boolean IS DISTINCT FROM TRUE
      )
    ORDER BY item.ordinal
    LIMIT 1;
    IF invalid_item IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install relational data-source authority: legacy data source % is invalid or contains unrecognized authority',
            invalid_item;
    END IF;

    SELECT item.element->>'id'
    INTO duplicate_id
    FROM workspace_settings AS settings
    CROSS JOIN LATERAL jsonb_array_elements(settings.value) AS item(element)
    WHERE settings.key='data_sources'
    GROUP BY item.element->>'id'
    HAVING COUNT(*) <> 1
    ORDER BY item.element->>'id'
    LIMIT 1;
    IF duplicate_id IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install relational data-source authority: duplicate legacy data source id %',
            duplicate_id;
    END IF;

    SELECT settings.key
    INTO invalid_catalog
    FROM workspace_settings AS settings
    WHERE settings.key LIKE 'data_source_catalog:%'
      AND (
          settings.key='data_source_catalog:' OR
          jsonb_typeof(settings.value) IS DISTINCT FROM 'object' OR
          NOT (settings.value ?& ARRAY[
              'schema','source_id','source_name','driver','fingerprint','captured_at','relations'
          ]::text[]) OR
          settings.value - ARRAY[
              'schema','source_id','source_name','driver','fingerprint','captured_at','relations'
          ]::text[] <> '{}'::jsonb OR
          settings.value->>'schema' <> 'omnidex.datasource-schema.v1' OR
          jsonb_typeof(settings.value->'source_id') IS DISTINCT FROM 'string' OR
          settings.value->>'source_id' <> substring(settings.key FROM length('data_source_catalog:') + 1) OR
          jsonb_typeof(settings.value->'source_name') IS DISTINCT FROM 'string' OR
          settings.value->>'driver' <> 'postgres' OR
          jsonb_typeof(settings.value->'fingerprint') IS DISTINCT FROM 'string' OR
          settings.value->>'fingerprint' !~ '^[0-9a-f]{64}$' OR
          jsonb_typeof(settings.value->'captured_at') IS DISTINCT FROM 'string' OR
          jsonb_typeof(settings.value->'relations') IS DISTINCT FROM 'array' OR
          jsonb_array_length(settings.value->'relations')=0 OR
          NOT EXISTS (
              SELECT 1
              FROM workspace_settings AS sources
              CROSS JOIN LATERAL jsonb_array_elements(sources.value) AS item(element)
              WHERE sources.key='data_sources'
                AND item.element->>'id'=substring(settings.key FROM length('data_source_catalog:') + 1)
          )
      )
    ORDER BY settings.key
    LIMIT 1;
    IF invalid_catalog IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install relational data-source authority: legacy schema snapshot % is invalid or unbound',
            invalid_catalog;
    END IF;

    SELECT channel.id
    INTO invalid_channel
    FROM data_source_channels AS channel
    WHERE NOT EXISTS (
        SELECT 1
        FROM workspace_settings AS sources
        CROSS JOIN LATERAL jsonb_array_elements(sources.value) AS item(element)
        WHERE sources.key='data_sources' AND item.element->>'id'=channel.data_source_id
    )
    ORDER BY channel.id
    LIMIT 1;
    IF invalid_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install relational data-source authority: legacy data-source channel % is unbound',
            invalid_channel;
    END IF;
END $$;

CREATE TABLE data_sources (
    id TEXT PRIMARY KEY,
    sort_order BIGINT GENERATED BY DEFAULT AS IDENTITY UNIQUE NOT NULL,
    name TEXT NOT NULL,
    driver TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    database_name TEXT NOT NULL,
    username TEXT NOT NULL,
    password TEXT NOT NULL DEFAULT '',
    ssl_mode TEXT NOT NULL,
    use_dsn BOOLEAN NOT NULL,
    dsn TEXT NOT NULL DEFAULT '',
    read_only BOOLEAN NOT NULL,
    last_test_status TEXT NOT NULL,
    last_test_message TEXT NOT NULL,
    last_test_at TIMESTAMPTZ,
    schema_catalog JSONB,
    catalog_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT data_sources_identity_check CHECK (
        id ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'
    ),
    CONSTRAINT data_sources_name_check CHECK (name<>'' AND name=btrim(name)),
    CONSTRAINT data_sources_driver_check CHECK (driver='postgres'),
    CONSTRAINT data_sources_port_check CHECK (port BETWEEN 1 AND 65535),
    CONSTRAINT data_sources_ssl_mode_check CHECK (
        ssl_mode IN ('disable','allow','prefer','require','verify-ca','verify-full')
    ),
    CONSTRAINT data_sources_connection_shape_check CHECK (
        host=btrim(host) AND database_name=btrim(database_name) AND
        username=btrim(username) AND dsn=btrim(dsn) AND
        (
            (use_dsn AND dsn<>'') OR
            (NOT use_dsn AND host<>'' AND database_name<>'' AND username<>'')
        )
    ),
    CONSTRAINT data_sources_read_only_check CHECK (read_only),
    CONSTRAINT data_sources_schema_snapshot_shape_check CHECK (
        schema_catalog IS NULL OR (
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
    )
);

INSERT INTO data_sources (
    id, sort_order, name, driver, host, port, database_name, username, password, ssl_mode, use_dsn, dsn,
    read_only, last_test_status, last_test_message, last_test_at,
    catalog_updated_at, created_at, updated_at
)
SELECT
    item.element->>'id', item.ordinal, item.element->>'name', item.element->>'driver',
    item.element->>'host', (item.element->>'port')::integer,
    item.element->>'database_name', item.element->>'username',
    COALESCE(item.element->>'password', ''), item.element->>'ssl_mode',
    (item.element->>'use_dsn')::boolean, COALESCE(item.element->>'dsn', ''),
    (item.element->>'read_only')::boolean, item.element->>'last_test_status',
    item.element->>'last_test_message', (item.element->>'last_test_at')::timestamptz,
    (item.element->>'catalog_updated_at')::timestamptz,
    (item.element->>'created_at')::timestamptz, (item.element->>'updated_at')::timestamptz
FROM workspace_settings AS settings
CROSS JOIN LATERAL jsonb_array_elements(settings.value)
    WITH ORDINALITY AS item(element, ordinal)
WHERE settings.key='data_sources'
ORDER BY item.ordinal;

UPDATE data_sources AS source
SET schema_catalog=settings.value
FROM workspace_settings AS settings
WHERE settings.key='data_source_catalog:' || source.id;

SELECT setval(
    pg_get_serial_sequence('data_sources', 'sort_order'),
    COALESCE((SELECT MAX(sort_order) FROM data_sources), 1),
    EXISTS (SELECT 1 FROM data_sources)
);

ALTER TABLE data_source_channels
    ADD CONSTRAINT data_source_channels_source_fkey
    FOREIGN KEY (data_source_id) REFERENCES data_sources(id) ON DELETE RESTRICT;

ALTER TABLE ai_channels
    ADD COLUMN data_source_id TEXT REFERENCES data_sources(id) ON DELETE RESTRICT;

CREATE INDEX idx_ai_channels_data_source_updated
    ON ai_channels(data_source_id, updated_at DESC, id ASC)
    WHERE data_source_id IS NOT NULL;

CREATE OR REPLACE FUNCTION reject_channel_binding_update()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.project_id IS DISTINCT FROM OLD.project_id OR
       NEW.workspace_root IS DISTINCT FROM OLD.workspace_root OR
       NEW.data_source_id IS DISTINCT FROM OLD.data_source_id THEN
        RAISE EXCEPTION 'channel project, workspace, and data-source binding is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DELETE FROM workspace_settings
WHERE key='data_sources' OR key LIKE 'data_source_catalog:%';

ALTER TABLE workspace_settings
    ADD CONSTRAINT workspace_settings_retired_data_source_authority_absent CHECK (
        key <> 'data_sources' AND key NOT LIKE 'data_source_catalog:%'
    );

DO $$
DECLARE
    legacy_count BIGINT;
    invalid_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO legacy_count
    FROM workspace_settings
    WHERE key='data_sources' OR key LIKE 'data_source_catalog:%';

    SELECT COUNT(*) INTO invalid_count
    FROM ai_channels AS channel
    LEFT JOIN data_sources AS source ON source.id=channel.data_source_id
    WHERE channel.data_source_id IS NOT NULL AND source.id IS NULL;

    IF legacy_count <> 0 OR invalid_count <> 0 OR
       to_regclass(current_schema() || '.data_sources') IS NULL OR
       EXISTS (
           SELECT 1 FROM information_schema.columns
           WHERE table_schema=current_schema() AND table_name='data_sources'
             AND column_name IN ('domain','context_prompt','privacy_mode')
       ) OR
       to_regclass(current_schema() || '.idx_ai_channels_data_source_updated') IS NULL OR
       NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='workspace_settings'::regclass
             AND conname='workspace_settings_retired_data_source_authority_absent'
             AND contype='c' AND convalidated
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='ai_channels'::regclass
             AND tgname='ai_channels_binding_immutable'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='data_source_channels'::regclass
             AND conname='data_source_channels_source_fkey'
             AND contype='f' AND convalidated
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='data_sources'::regclass
             AND conname IN (
                 'data_sources_ssl_mode_check',
                 'data_sources_connection_shape_check',
                 'data_sources_read_only_check'
             )
             AND contype='c' AND convalidated
           GROUP BY conrelid
           HAVING COUNT(*)=3
       ) THEN
        RAISE EXCEPTION 'relational data-source authority postcondition failed';
    END IF;
END $$;
