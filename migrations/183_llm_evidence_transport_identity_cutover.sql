BEGIN;

LOCK TABLE station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    station_gap_function_source TEXT;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (VALUES
            ('llm_call_evidence_request_sha256_check',
             '1e3b04b4cb1c823cdac9eeb8fce4c7e98c9b202e5aab7cd8e822a906018b475a'),
            ('llm_call_evidence_response_format_check',
             '544dcaa6bbffddfa504427670f875c2b6d28b9a53ad61fe9a12ae43e543749f2'),
            ('llm_call_evidence_current_raw_transport',
             'cf2775374afaa1cd7948e9edc383d4ee4b52661e329baf5a7dcef10c59751dd7'),
            ('llm_call_evidence_status_check',
             '688db1b0a0b1a3e3b3752cf70a68dfcd747846aea85a0902e6447ae10770e166'),
            ('llm_call_evidence_check5',
             '863efc880ec602b59a0946d4a55585f014eade2e56ca7c99bccf420e4038e6fa'),
            ('llm_call_evidence_check6',
             '2a9f98ec1b098cf24de946a2082f46f0edbe2333c7a1612b1d44b9b5fb19c6ef')
        ) AS expected(constraint_name,definition_sha256)
        LEFT JOIN pg_constraint AS actual
          ON actual.conrelid='llm_call_evidence'::regclass
         AND actual.conname=expected.constraint_name
         AND actual.contype='c'
        WHERE actual.oid IS NULL OR actual.convalidated IS DISTINCT FROM TRUE OR
              encode(digest(
                  convert_to(pg_get_constraintdef(actual.oid),'UTF8'),'sha256'
              ),'hex')<>expected.definition_sha256
    ) THEN
        RAISE EXCEPTION
            'LLM evidence transport identity cutover requires the exact prior constraints';
    END IF;

    SELECT prosrc INTO station_gap_function_source
    FROM pg_proc
    WHERE oid=to_regprocedure('require_llm_call_station_gap()');
    IF station_gap_function_source IS NULL OR
       encode(digest(convert_to(station_gap_function_source,'UTF8'),'sha256'),'hex')<>
           '137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50' OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='llm_call_evidence'::regclass
             AND tgname='llm_call_evidence_require_station_gap'
             AND tgfoid=to_regprocedure('require_llm_call_station_gap()')
             AND tgenabled='O' AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid='llm_call_evidence'::regclass
             AND attname='request_sha256'
             AND format_type(atttypid,atttypmod)='text'
             AND attnotnull AND NOT attisdropped
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid='llm_call_evidence'::regclass
             AND attname='response_format'
             AND format_type(atttypid,atttypmod)='text'
             AND attnotnull AND NOT attisdropped
       ) THEN
        RAISE EXCEPTION
            'LLM evidence transport identity cutover requires the exact station-call authority';
    END IF;

    IF EXISTS (SELECT 1 FROM llm_call_evidence) THEN
        RAISE EXCEPTION
            'LLM evidence transport identity cutover requires a fresh reset: immutable evidence exists';
    END IF;
END;
$precondition$;

ALTER TABLE llm_call_evidence
    DROP CONSTRAINT llm_call_evidence_request_sha256_check,
    DROP CONSTRAINT llm_call_evidence_response_format_check,
    DROP CONSTRAINT llm_call_evidence_current_raw_transport,
    DROP CONSTRAINT llm_call_evidence_status_check,
    DROP CONSTRAINT llm_call_evidence_check5,
    DROP CONSTRAINT llm_call_evidence_check6,
    DROP COLUMN request_sha256,
    DROP COLUMN response_format,
    ADD CONSTRAINT llm_call_evidence_status_check CHECK (
        status IN ('generation_failed','succeeded')
    );

DO $postcondition$
DECLARE
    status_hash TEXT;
BEGIN
    SELECT encode(digest(
        convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'
    ),'hex') INTO status_hash
    FROM pg_constraint
    WHERE conrelid='llm_call_evidence'::regclass
      AND conname='llm_call_evidence_status_check'
      AND contype='c' AND convalidated;

    IF status_hash IS DISTINCT FROM
       '142000be76074a80703ad0af90e1ee0826a239583519af274d71f712830e37c5' OR EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid='llm_call_evidence'::regclass
          AND attname IN ('request_sha256','response_format')
          AND NOT attisdropped
    ) OR EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='llm_call_evidence'::regclass
          AND conname IN (
              'llm_call_evidence_current_raw_transport',
              'llm_call_evidence_check5','llm_call_evidence_check6'
          )
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_proc
        WHERE oid=to_regprocedure('require_llm_call_station_gap()')
          AND encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')=
              '137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50'
    ) OR EXISTS (SELECT 1 FROM llm_call_evidence) THEN
        RAISE EXCEPTION
            'LLM evidence transport identity cutover postcondition failed: status hash %',
            status_hash;
    END IF;
END;
$postcondition$;

COMMIT;
