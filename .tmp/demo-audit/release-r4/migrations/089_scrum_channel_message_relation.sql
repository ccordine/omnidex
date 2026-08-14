LOCK TABLE lifecycle_operation_registry, scrum_channel_operations, scrum_flow_events, scrum_cards
    IN ACCESS EXCLUSIVE MODE;

DO $clean_start$
DECLARE
    card_count BIGINT;
    operation_count BIGINT;
    flow_count BIGINT;
    registry_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO card_count FROM scrum_cards;
    SELECT COUNT(*) INTO operation_count FROM scrum_channel_operations;
    SELECT COUNT(*) INTO flow_count FROM scrum_flow_events;
    SELECT COUNT(*) INTO registry_count
    FROM lifecycle_operation_registry
    WHERE kind = 'scrum_channel_message';

    IF card_count <> 0 OR operation_count <> 0 OR flow_count <> 0 OR registry_count <> 0 THEN
        RAISE EXCEPTION
            'migration 089 reset required: legacy Scrum state is nonempty (scrum_cards=%, scrum_channel_operations=%, scrum_flow_events=%, scrum_channel_registry=%); reset Scrum state before retrying',
            card_count, operation_count, flow_count, registry_count;
    END IF;
END;
$clean_start$;

DROP TABLE scrum_channel_operations;
DROP TABLE scrum_flow_events;
DROP FUNCTION prevent_scrum_channel_operation_mutation();

CREATE FUNCTION scrum_trim_space(value TEXT)
RETURNS TEXT
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN btrim(
    value,
    E' \t\n\r' || chr(11) || chr(12) || chr(133) || chr(160) || chr(5760) ||
    chr(8192) || chr(8193) || chr(8194) || chr(8195) || chr(8196) ||
    chr(8197) || chr(8198) || chr(8199) || chr(8200) || chr(8201) ||
    chr(8202) || chr(8232) || chr(8233) || chr(8239) || chr(8287) || chr(12288)
);

CREATE FUNCTION scrum_canonical_timestamp(value TIMESTAMPTZ)
RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN isfinite(value) AND
       value = date_trunc('microseconds', value) AND
       EXTRACT(YEAR FROM value AT TIME ZONE 'UTC') BETWEEN 1 AND 9999;

CREATE FUNCTION scrum_render_utc_timestamp(value TIMESTAMPTZ)
RETURNS TEXT
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE
    WHEN NOT scrum_canonical_timestamp(value) THEN NULL
    WHEN date_part('microseconds', value)::BIGINT % 1000000 = 0
        THEN to_char(value AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
    ELSE regexp_replace(
        to_char(value AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        '0+Z$', 'Z'
    )
END;

CREATE FUNCTION scrum_database_time()
RETURNS TIMESTAMPTZ
LANGUAGE SQL VOLATILE STRICT PARALLEL UNSAFE
RETURN date_trunc('microseconds', clock_timestamp());

CREATE FUNCTION scrum_valid_message_id(value TEXT)
RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE
RETURN octet_length(value) BETWEEN 1 AND 256 AND
       value ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]*$';

DO $pin_helpers$
DECLARE
    runtime_schema TEXT := current_schema();
BEGIN
    EXECUTE format(
        'ALTER FUNCTION %I.scrum_trim_space(text) SET search_path TO pg_catalog, %I, public, pg_temp',
        runtime_schema, runtime_schema
    );
    EXECUTE format(
        'ALTER FUNCTION %I.scrum_canonical_timestamp(timestamptz) SET search_path TO pg_catalog, %I, public, pg_temp',
        runtime_schema, runtime_schema
    );
    EXECUTE format(
        'ALTER FUNCTION %I.scrum_render_utc_timestamp(timestamptz) SET search_path TO pg_catalog, %I, public, pg_temp',
        runtime_schema, runtime_schema
    );
    EXECUTE format(
        'ALTER FUNCTION %I.scrum_database_time() SET search_path TO pg_catalog, %I, public, pg_temp',
        runtime_schema, runtime_schema
    );
    EXECUTE format(
        'ALTER FUNCTION %I.scrum_valid_message_id(text) SET search_path TO pg_catalog, %I, public, pg_temp',
        runtime_schema, runtime_schema
    );
END;
$pin_helpers$;

ALTER TABLE scrum_cards
    DROP CONSTRAINT scrum_cards_sync_job_authority,
    DROP CONSTRAINT scrum_cards_sync_cursors_nonnegative,
    DROP COLUMN chat,
    DROP COLUMN planning_chat,
    DROP COLUMN console_log,
    DROP COLUMN agent_stream_chat_cursor,
    DROP COLUMN agent_stream_console_cursor,
    ADD COLUMN channel_message_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN channel_content_bytes BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT scrum_cards_project_identity UNIQUE (project_id, id),
    ADD CONSTRAINT scrum_cards_channel_counters_closed CHECK (
        channel_message_count BETWEEN 0 AND 9007199254740991 AND
        channel_content_bytes BETWEEN 0 AND 9007199254740991
    ),
    ADD CONSTRAINT scrum_cards_timestamps_closed CHECK (
        scrum_canonical_timestamp(created_at) AND
        scrum_canonical_timestamp(updated_at)
    ),
    ADD CONSTRAINT scrum_cards_sync_job_authority CHECK (
        (sync_job_id = '' AND
         step_context_cursor = 0 AND
         play_state <> 'running' AND
         NOT (column_name = 'in_progress' AND job_id <> '')) OR
        (sync_job_id <> '' AND sync_job_id = job_id)
    ),
    ADD CONSTRAINT scrum_cards_sync_cursors_nonnegative CHECK (
        step_context_cursor >= 0
    );

CREATE TABLE scrum_card_messages (
    project_id BIGINT NOT NULL,
    card_id TEXT NOT NULL,
    ordinal BIGINT NOT NULL,
    message_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (
        role IN ('user','assistant','system','error','tool','thinking','status')
    ),
    content TEXT NOT NULL CHECK (
        octet_length(content) BETWEEN 1 AND 4194304
    ),
    content_bytes BIGINT GENERATED ALWAYS AS (octet_length(content)) STORED,
    created_at TIMESTAMPTZ NOT NULL,
    source_created_at TEXT NOT NULL,
    timestamp_origin TEXT NOT NULL CHECK (timestamp_origin = 'runtime'),
    status TEXT NOT NULL CHECK (status IN ('','running','completed','failed')),
    operation_id TEXT,
    inserted_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (project_id, card_id, ordinal),
    UNIQUE (project_id, card_id, message_id),
    FOREIGN KEY (project_id, card_id)
        REFERENCES scrum_cards(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (operation_id)
        REFERENCES lifecycle_operation_registry(operation_id) ON DELETE RESTRICT,
    CHECK (ordinal BETWEEN 1 AND 9007199254740991),
    CHECK (scrum_valid_message_id(message_id)),
    CHECK (scrum_canonical_timestamp(created_at)),
    CHECK (scrum_canonical_timestamp(inserted_at)),
    CHECK (created_at = inserted_at),
    CHECK (source_created_at = scrum_render_utc_timestamp(created_at)),
    CHECK (operation_id IS NULL OR role = 'user')
);

CREATE INDEX scrum_card_messages_tail
    ON scrum_card_messages(project_id, card_id, ordinal DESC)
    INCLUDE (message_id,role,content,created_at,source_created_at,timestamp_origin,status,operation_id,content_bytes);
CREATE UNIQUE INDEX scrum_card_messages_operation
    ON scrum_card_messages(operation_id) WHERE operation_id IS NOT NULL;

CREATE FUNCTION own_scrum_card_message_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $own_message$
DECLARE
    next_ordinal BIGINT;
    owned_time TIMESTAMPTZ;
    registered_kind TEXT;
    registered_payload JSONB;
BEGIN
    IF NEW.ordinal IS NOT NULL OR NEW.created_at IS NOT NULL OR
       NEW.source_created_at IS NOT NULL OR NEW.timestamp_origin IS NOT NULL OR
       NEW.inserted_at IS NOT NULL THEN
        RAISE EXCEPTION 'Scrum message forbids caller-supplied ordinal or timestamp provenance';
    END IF;
    SELECT channel_message_count + 1
    INTO next_ordinal
    FROM scrum_cards
    WHERE project_id=NEW.project_id AND id=NEW.card_id
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target %/% does not exist', NEW.project_id, NEW.card_id;
    END IF;
    IF next_ordinal > 9007199254740991 THEN
        RAISE EXCEPTION 'Scrum channel message counter exceeds exact transport authority';
    END IF;
    IF NEW.operation_id IS NOT NULL THEN
        SELECT kind,command_payload INTO registered_kind,registered_payload
        FROM lifecycle_operation_registry
        WHERE operation_id=NEW.operation_id
        FOR SHARE;
        IF NOT FOUND OR registered_kind <> 'scrum_channel_message' OR
           registered_payload->>'project_id' <> NEW.project_id::TEXT OR
           registered_payload->>'card_id' <> NEW.card_id OR
           registered_payload->>'message' <> NEW.content THEN
            RAISE EXCEPTION 'Scrum message operation binding does not match its exact command';
        END IF;
    END IF;
    owned_time := scrum_database_time();
    NEW.ordinal := next_ordinal;
    NEW.created_at := owned_time;
    NEW.inserted_at := owned_time;
    NEW.source_created_at := scrum_render_utc_timestamp(owned_time);
    NEW.timestamp_origin := 'runtime';
    RETURN NEW;
END;
$own_message$;

CREATE FUNCTION reject_scrum_card_counter_seed()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $reject_counter_seed$
BEGIN
    IF NEW.channel_message_count <> 0 OR NEW.channel_content_bytes <> 0 THEN
        RAISE EXCEPTION 'new Scrum cards must start with empty relation-owned channel counters';
    END IF;
    RETURN NEW;
END;
$reject_counter_seed$;

CREATE FUNCTION apply_scrum_card_message_counters()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $apply_counters$
BEGIN
    UPDATE scrum_cards
    SET channel_message_count=NEW.ordinal,
        channel_content_bytes=channel_content_bytes+NEW.content_bytes,
        updated_at=GREATEST(scrum_database_time(),updated_at+interval '1 microsecond')
    WHERE project_id=NEW.project_id AND id=NEW.card_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Scrum message target disappeared while applying counters';
    END IF;
    RETURN NULL;
END;
$apply_counters$;

CREATE FUNCTION enforce_scrum_card_message_counters()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $enforce_counters$
DECLARE
    appended_bytes BIGINT;
BEGIN
    IF NEW.channel_message_count <> OLD.channel_message_count + 1 THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    SELECT content_bytes INTO appended_bytes
    FROM scrum_card_messages
    WHERE project_id=NEW.project_id AND card_id=NEW.id
      AND ordinal=NEW.channel_message_count;
    IF NOT FOUND OR NEW.channel_content_bytes <> OLD.channel_content_bytes + appended_bytes THEN
        RAISE EXCEPTION 'Scrum channel counters are relation-owned';
    END IF;
    RETURN NEW;
END;
$enforce_counters$;

CREATE FUNCTION reject_scrum_message_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $reject_message$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (
        SELECT 1 FROM scrum_cards
        WHERE project_id=OLD.project_id AND id=OLD.card_id
    ) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'Scrum card messages are append-only';
END;
$reject_message$;

DO $pin_message_functions$
DECLARE
    runtime_schema TEXT := current_schema();
    function_name TEXT;
BEGIN
    FOREACH function_name IN ARRAY ARRAY[
        'own_scrum_card_message_insert',
        'reject_scrum_card_counter_seed',
        'apply_scrum_card_message_counters',
        'enforce_scrum_card_message_counters',
        'reject_scrum_message_mutation'
    ] LOOP
        EXECUTE format(
            'ALTER FUNCTION %I.%I() SET search_path TO pg_catalog, %I, public, pg_temp',
            runtime_schema, function_name, runtime_schema
        );
    END LOOP;
END;
$pin_message_functions$;

CREATE TRIGGER scrum_card_messages_own_insert
BEFORE INSERT ON scrum_card_messages
FOR EACH ROW EXECUTE FUNCTION own_scrum_card_message_insert();
CREATE TRIGGER scrum_cards_empty_channel_counters
BEFORE INSERT ON scrum_cards
FOR EACH ROW EXECUTE FUNCTION reject_scrum_card_counter_seed();
CREATE TRIGGER scrum_card_messages_apply_counters
AFTER INSERT ON scrum_card_messages
FOR EACH ROW EXECUTE FUNCTION apply_scrum_card_message_counters();
CREATE TRIGGER scrum_cards_message_counters_immutable
BEFORE UPDATE OF channel_message_count,channel_content_bytes ON scrum_cards
FOR EACH ROW EXECUTE FUNCTION enforce_scrum_card_message_counters();
CREATE TRIGGER scrum_card_messages_immutable
BEFORE UPDATE OR DELETE ON scrum_card_messages
FOR EACH ROW EXECUTE FUNCTION reject_scrum_message_mutation();
CREATE TRIGGER scrum_card_messages_truncate_immutable
BEFORE TRUNCATE ON scrum_card_messages
FOR EACH STATEMENT EXECUTE FUNCTION reject_scrum_message_mutation();
