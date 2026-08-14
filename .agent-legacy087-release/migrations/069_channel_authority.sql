LOCK TABLE ai_channels, ai_channel_messages, jobs IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_channel TEXT;
    invalid_message BIGINT;
    duplicate_channel TEXT;
BEGIN
    SELECT id INTO invalid_channel
    FROM ai_channels
    WHERE id LIKE 'thought\_%' ESCAPE '\'
       OR id LIKE 'internal-%'
       OR tags && ARRAY['thought-channel','internal:thought']::TEXT[]
    ORDER BY id
    LIMIT 1;
    IF invalid_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel authority: rejected internal channel % still exists',
            invalid_channel;
    END IF;

    SELECT id INTO invalid_channel
    FROM ai_channels
    WHERE id !~ '^[a-z0-9][a-z0-9_.:-]{0,95}$'
       OR octet_length(name) NOT BETWEEN 1 AND 256
       OR name <> btrim(name)
       OR cardinality(tags) > 32
       OR EXISTS (
           SELECT 1 FROM unnest(tags) AS tag
           WHERE tag IS NULL
              OR octet_length(tag) NOT BETWEEN 1 AND 64
              OR tag <> btrim(tag)
              OR tag <> lower(tag)
       )
       OR cardinality(tags) <> (
           SELECT count(DISTINCT tag) FROM unnest(tags) AS tag
       )
    ORDER BY id
    LIMIT 1;
    IF invalid_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel authority: channel % requires an explicit identity/name/tag correction',
            invalid_channel;
    END IF;

    SELECT id INTO invalid_message
    FROM ai_channel_messages
    WHERE role NOT IN ('user','assistant')
       OR octet_length(content) NOT BETWEEN 1 AND 65536
       OR btrim(content)=''
    ORDER BY id
    LIMIT 1;
    IF invalid_message IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel authority: message % has unsupported role or content',
            invalid_message;
    END IF;

    SELECT metadata->>'channel_id' INTO duplicate_channel
    FROM jobs
    WHERE status IN ('pending','running','waiting_input')
      AND metadata ? 'channel_id'
    GROUP BY metadata->>'channel_id'
    HAVING count(*) > 1
    ORDER BY metadata->>'channel_id'
    LIMIT 1;
    IF duplicate_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel authority: channel % has multiple active turns',
            duplicate_channel;
    END IF;
END $$;

CREATE FUNCTION channel_tags_are_exact(tags TEXT[])
RETURNS BOOLEAN AS $$
    SELECT cardinality(tags) <= 32
       AND NOT EXISTS (
           SELECT 1 FROM unnest(tags) AS tag
           WHERE tag IS NULL
              OR octet_length(tag) NOT BETWEEN 1 AND 64
              OR tag <> btrim(tag)
              OR tag <> lower(tag)
       )
       AND cardinality(tags) = (
           SELECT count(DISTINCT tag) FROM unnest(tags) AS tag
       );
$$ LANGUAGE sql IMMUTABLE STRICT;

ALTER TABLE ai_channels ADD COLUMN scope TEXT;

UPDATE ai_channels
SET scope = 'user';

ALTER TABLE ai_channels
    ALTER COLUMN scope SET NOT NULL,
    ADD CONSTRAINT ai_channels_scope_check
        CHECK (scope = 'user'),
    ADD CONSTRAINT ai_channels_identity_check
        CHECK (id ~ '^[a-z0-9][a-z0-9_.:-]{0,95}$'),
    ADD CONSTRAINT ai_channels_name_check
        CHECK (octet_length(name) BETWEEN 1 AND 256 AND name=btrim(name)),
    ADD CONSTRAINT ai_channels_tags_check
        CHECK (channel_tags_are_exact(tags));

ALTER TABLE ai_channel_messages
    ADD CONSTRAINT ai_channel_messages_role_check
        CHECK (role IN ('user','assistant')),
    ADD CONSTRAINT ai_channel_messages_content_check
        CHECK (octet_length(content) BETWEEN 1 AND 65536 AND btrim(content) <> '');

DROP INDEX idx_ai_channels_updated;
CREATE INDEX idx_ai_channels_scope_updated
    ON ai_channels(scope, updated_at DESC, id ASC);

CREATE UNIQUE INDEX idx_jobs_one_active_channel_turn
    ON jobs ((metadata->>'channel_id'))
    WHERE status IN ('pending','running','waiting_input')
      AND metadata ? 'channel_id';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM ai_channels
        WHERE scope <> 'user'
           OR id !~ '^[a-z0-9][a-z0-9_.:-]{0,95}$'
           OR NOT channel_tags_are_exact(tags)
    ) OR EXISTS (
        SELECT 1 FROM ai_channel_messages
        WHERE role NOT IN ('user','assistant')
           OR octet_length(content) NOT BETWEEN 1 AND 65536
           OR btrim(content)=''
    ) OR to_regclass(current_schema() || '.idx_ai_channels_updated') IS NOT NULL OR
       to_regclass(current_schema() || '.idx_ai_channels_scope_updated') IS NULL OR
       to_regclass(current_schema() || '.idx_jobs_one_active_channel_turn') IS NULL THEN
        RAISE EXCEPTION 'channel authority postcondition failed';
    END IF;
END $$;
