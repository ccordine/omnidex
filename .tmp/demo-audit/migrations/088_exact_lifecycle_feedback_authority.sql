LOCK TABLE job_generations, task_entries IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_sha256 TEXT;
BEGIN
    IF current_setting('server_encoding') <> 'UTF8' THEN
        RAISE EXCEPTION
            'exact lifecycle feedback requires UTF8 server_encoding, observed %',
            current_setting('server_encoding');
    END IF;

    SELECT encode(
        digest(convert_to(pg_get_constraintdef(oid,true),'UTF8'),'sha256'),
        'hex'
    ) INTO observed_sha256
    FROM pg_constraint
    WHERE conrelid='job_generations'::regclass
      AND conname='job_generations_authoritative_shape'
      AND contype='c';
    IF observed_sha256 IS DISTINCT FROM
       '6d35378110ee10f551a3db1f9384099ddcae7bbf2e15763262bafcb437e493b3' THEN
        RAISE EXCEPTION
            'job generation feedback predecessor differs from frozen authority: %',
            observed_sha256;
    END IF;

    SELECT encode(digest(convert_to(pg_get_constraintdef(oid,true),'UTF8'),'sha256'),'hex')
    INTO observed_sha256
    FROM pg_constraint
    WHERE conrelid='task_entries'::regclass
      AND conname='task_entries_content_check'
      AND contype='c';
    IF observed_sha256 IS DISTINCT FROM
       '29d070c6112b6bc76d0a91de11036e4796d7ca767073867fb3361a78c1ac927f' THEN
        RAISE EXCEPTION
            'task feedback content predecessor differs from frozen authority: %',
            observed_sha256;
    END IF;

    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
    INTO observed_sha256
    FROM pg_proc
    WHERE oid='task_ledger_text_is_exact(text)'::regprocedure;
    IF observed_sha256 IS DISTINCT FROM
       '05066f0dac05a1d6bf621abe53778ff76bbafc8326b67b8ddd0428d80cda25cb' THEN
        RAISE EXCEPTION
            'task ledger exact-text authority differs from frozen predecessor: %',
            observed_sha256;
    END IF;
END $$;

CREATE FUNCTION lifecycle_feedback_is_valid(value TEXT, maximum_bytes INTEGER)
RETURNS BOOLEAN AS $$
    SELECT maximum_bytes BETWEEN 1 AND 65536
       AND octet_length(value) BETWEEN 1 AND maximum_bytes
       AND btrim(
            value,
            U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
       ) <> ''
       AND convert_from(convert_to(value, 'UTF8'), 'UTF8') = value
       -- PostgreSQL TEXT rejects NUL before a constraint can run; retain an
       -- explicit byte-level postcondition so this authority cannot drift.
       AND position(decode('00','hex') in convert_to(value,'UTF8')) = 0;
$$ LANGUAGE SQL IMMUTABLE STRICT;

DO $$
DECLARE
    invalid_job BIGINT;
    invalid_entry TEXT;
BEGIN
    SELECT job_id INTO invalid_job
    FROM job_generations
    WHERE purpose='replan'
      AND (
        NOT lifecycle_feedback_is_valid(feedback,65536) OR
        feedback_sha256 <> encode(digest(feedback, 'sha256'), 'hex')
      )
    ORDER BY job_id,generation
    LIMIT 1;
    IF invalid_job IS NOT NULL THEN
        RAISE EXCEPTION
            'historical lifecycle feedback for job % is outside exact authority',
            invalid_job;
    END IF;

    SELECT ledger_id||'/'||id INTO invalid_entry
    FROM task_entries
    WHERE kind='feedback'
      AND (
        NOT lifecycle_feedback_is_valid(content,65536) OR
        content_sha256 <> encode(digest(content, 'sha256'), 'hex')
      )
    ORDER BY ledger_id,id
    LIMIT 1;
    IF invalid_entry IS NOT NULL THEN
        RAISE EXCEPTION
            'historical lifecycle feedback entry % is outside exact authority',
            invalid_entry;
    END IF;
END $$;

ALTER TABLE job_generations
    DROP CONSTRAINT job_generations_authoritative_shape,
    ADD CONSTRAINT job_generations_authoritative_shape CHECK (
        (generation=1 AND purpose='initial' AND
            predecessor_generation IS NULL AND boundary_action IS NULL AND
            feedback IS NULL AND feedback_sha256 IS NULL) OR
        (generation>1 AND purpose='replan' AND
            predecessor_generation=generation-1 AND
            boundary_action IN ('v3_coding','objective_resolve','v3_planning') AND
            lifecycle_feedback_is_valid(feedback,65536) AND
            feedback_sha256 ~ '^[0-9a-f]{64}$' AND
            feedback_sha256 = encode(digest(feedback, 'sha256'), 'hex'))
    );

ALTER TABLE task_entries
    DROP CONSTRAINT task_entries_content_check,
    ADD CONSTRAINT task_entries_content_check CHECK (
        (kind='feedback' AND lifecycle_feedback_is_valid(content,65536)) OR
        (kind<>'feedback' AND task_ledger_text_is_exact(content))
    );

DO $$
DECLARE
    generation_definition TEXT;
    entry_definition TEXT;
    helper_sha256 TEXT;
    function_count INTEGER;
BEGIN
    SELECT pg_get_constraintdef(oid,true) INTO generation_definition
    FROM pg_constraint
    WHERE conrelid='job_generations'::regclass
      AND conname='job_generations_authoritative_shape'
      AND contype='c' AND convalidated AND NOT connoinherit;
    SELECT pg_get_constraintdef(oid,true) INTO entry_definition
    FROM pg_constraint
    WHERE conrelid='task_entries'::regclass
      AND conname='task_entries_content_check'
      AND contype='c' AND convalidated AND NOT connoinherit;
    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
    INTO helper_sha256
    FROM pg_proc
    WHERE oid='task_ledger_text_is_exact(text)'::regprocedure;
    SELECT COUNT(*) INTO function_count
    FROM pg_proc
    WHERE oid='lifecycle_feedback_is_valid(text,integer)'::regprocedure
      AND provolatile='i' AND proisstrict;

    IF generation_definition IS NULL OR
       generation_definition NOT LIKE '%lifecycle_feedback_is_valid(feedback, 65536)%' OR
       generation_definition LIKE '%task_ledger_text_is_exact(feedback)%' OR
       entry_definition IS NULL OR
       entry_definition NOT LIKE '%kind = ''feedback''%lifecycle_feedback_is_valid(content, 65536)%' OR
       entry_definition NOT LIKE '%kind <> ''feedback''%task_ledger_text_is_exact(content)%' OR
       helper_sha256 <> '05066f0dac05a1d6bf621abe53778ff76bbafc8326b67b8ddd0428d80cda25cb' OR
       function_count <> 1 THEN
        RAISE EXCEPTION 'exact lifecycle feedback postcondition failed';
    END IF;
END $$;
