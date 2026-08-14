LOCK TABLE ai_channel_messages IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_message BIGINT;
BEGIN
    SELECT id INTO invalid_message
    FROM ai_channel_messages
    WHERE btrim(content) = ''
       OR (role = 'user' AND octet_length(content) NOT BETWEEN 1 AND 4096)
       OR (role = 'assistant' AND octet_length(content) NOT BETWEEN 1 AND 32768)
       OR role NOT IN ('user','assistant')
    ORDER BY id
    LIMIT 1;
    IF invalid_message IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install channel message role bounds: message % is outside its typed role contract',
            invalid_message;
    END IF;
END $$;

ALTER TABLE ai_channel_messages
    DROP CONSTRAINT ai_channel_messages_content_check,
    ADD CONSTRAINT ai_channel_messages_content_check CHECK (
        btrim(content) <> ''
        AND (
            (role = 'user' AND octet_length(content) BETWEEN 1 AND 4096)
            OR
            (role = 'assistant' AND octet_length(content) BETWEEN 1 AND 32768)
        )
    );

DO $$
DECLARE
    content_constraint TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid, true) INTO content_constraint
    FROM pg_constraint
    WHERE conrelid = 'ai_channel_messages'::regclass
      AND conname = 'ai_channel_messages_content_check'
      AND contype = 'c';

    IF content_constraint IS NULL OR
       content_constraint NOT LIKE '%role = ''user''%4096%' OR
       content_constraint NOT LIKE '%role = ''assistant''%32768%' OR
       EXISTS (
           SELECT 1 FROM ai_channel_messages
           WHERE btrim(content) = ''
              OR (role = 'user' AND octet_length(content) NOT BETWEEN 1 AND 4096)
              OR (role = 'assistant' AND octet_length(content) NOT BETWEEN 1 AND 32768)
              OR role NOT IN ('user','assistant')
       ) THEN
        RAISE EXCEPTION 'channel message role bounds postcondition failed';
    END IF;
END $$;
