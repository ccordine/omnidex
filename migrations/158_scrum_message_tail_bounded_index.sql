BEGIN;

LOCK TABLE scrum_card_messages IN SHARE ROW EXCLUSIVE MODE;

DO $preflight$
DECLARE
    inherited_definition TEXT;
BEGIN
    SELECT pg_get_indexdef(index_relation.oid)
      INTO inherited_definition
    FROM pg_class AS index_relation
    JOIN pg_namespace AS index_namespace
      ON index_namespace.oid=index_relation.relnamespace
    JOIN pg_index AS index_authority
      ON index_authority.indexrelid=index_relation.oid
    WHERE index_namespace.nspname=current_schema()
      AND index_relation.relname='scrum_card_messages_tail'
      AND index_authority.indrelid='scrum_card_messages'::regclass
      AND index_authority.indisvalid
      AND index_authority.indisready
      AND NOT index_authority.indisunique
      AND NOT index_authority.indisprimary
      AND index_authority.indpred IS NULL
      AND index_authority.indexprs IS NULL;

    IF substring(inherited_definition FROM 'USING btree .*$') IS DISTINCT FROM
       'USING btree (project_id, card_id, ordinal DESC) INCLUDE (message_id, role, content, created_at, source_created_at, timestamp_origin, status, operation_id, content_bytes)' THEN
        RAISE EXCEPTION 'inherited Scrum message tail index differs';
    END IF;
END $preflight$;

DROP INDEX scrum_card_messages_tail;

CREATE INDEX scrum_card_messages_tail
    ON scrum_card_messages(project_id, card_id, ordinal DESC)
    INCLUDE (
        message_id,role,created_at,source_created_at,timestamp_origin,
        status,operation_id,content_bytes
    );

DO $postcondition$
DECLARE
    installed_definition TEXT;
BEGIN
    SELECT pg_get_indexdef(index_relation.oid)
      INTO installed_definition
    FROM pg_class AS index_relation
    JOIN pg_namespace AS index_namespace
      ON index_namespace.oid=index_relation.relnamespace
    JOIN pg_index AS index_authority
      ON index_authority.indexrelid=index_relation.oid
    WHERE index_namespace.nspname=current_schema()
      AND index_relation.relname='scrum_card_messages_tail'
      AND index_authority.indrelid='scrum_card_messages'::regclass
      AND index_authority.indisvalid
      AND index_authority.indisready
      AND NOT index_authority.indisunique
      AND NOT index_authority.indisprimary
      AND index_authority.indpred IS NULL
      AND index_authority.indexprs IS NULL;

    IF substring(installed_definition FROM 'USING btree .*$') IS DISTINCT FROM
       'USING btree (project_id, card_id, ordinal DESC) INCLUDE (message_id, role, created_at, source_created_at, timestamp_origin, status, operation_id, content_bytes)' THEN
        RAISE EXCEPTION 'bounded Scrum message tail index was not installed exactly';
    END IF;
END $postcondition$;

COMMIT;
