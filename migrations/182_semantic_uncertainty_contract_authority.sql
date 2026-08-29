BEGIN;

LOCK TABLE station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    history_function_source TEXT;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (VALUES
            ('station_gap_openings_renderer_version_check',
             '522c94915fab9d58a737582594a176fddd2c80322ecaf687edeeef34cd421605'),
            ('station_gap_openings_projection_v5',
             '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad'),
            ('station_gap_openings_current_raw_transport',
             'a147cbfc147e9f009ab9a4f51c0fac68ec42a5a7e44df60a67671955ac0afc7e')
        ) AS expected(constraint_name,definition_sha256)
        LEFT JOIN pg_constraint AS actual
          ON actual.conrelid='station_gap_openings'::regclass
         AND actual.conname=expected.constraint_name
         AND actual.contype='c'
        WHERE actual.oid IS NULL OR actual.convalidated IS DISTINCT FROM TRUE OR
              encode(digest(
                  convert_to(pg_get_constraintdef(actual.oid),'UTF8'),'sha256'
              ),'hex')<>expected.definition_sha256
    ) THEN
        RAISE EXCEPTION
            'semantic uncertainty authority requires the exact response-schema retirement constraints';
    END IF;

    SELECT prosrc INTO history_function_source
    FROM pg_proc
    WHERE oid=to_regprocedure('prevent_station_gap_history_mutation()');
    IF history_function_source IS NULL OR
       encode(digest(convert_to(history_function_source,'UTF8'),'sha256'),'hex')<>
           '59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58' OR
       NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_immutable'
             AND tgfoid=to_regprocedure('prevent_station_gap_history_mutation()')
             AND tgenabled='O' AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid='station_gap_openings'::regclass
             AND tgname='station_gap_openings_truncate_immutable'
             AND tgfoid=to_regprocedure('prevent_station_gap_history_mutation()')
             AND tgenabled='O' AND NOT tgisinternal
       ) OR EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid='station_gap_openings'::regclass
             AND attname IN (
                 'semantic_uncertainty_contract',
                 'semantic_uncertainty_contract_sha256'
             ) AND NOT attisdropped
       ) THEN
        RAISE EXCEPTION
            'semantic uncertainty authority requires the exact append-only opening schema';
    END IF;

    IF EXISTS (SELECT 1 FROM station_gap_openings) OR
       EXISTS (SELECT 1 FROM station_gap_outcomes) OR
       EXISTS (SELECT 1 FROM station_provider_discoveries) OR
       EXISTS (SELECT 1 FROM station_provider_discovery_receipts) OR
       EXISTS (SELECT 1 FROM station_call_openings) OR
       EXISTS (SELECT 1 FROM station_call_receipts) OR
       EXISTS (SELECT 1 FROM llm_call_evidence) THEN
        RAISE EXCEPTION
            'semantic uncertainty authority requires a fresh reset: inference history exists';
    END IF;
END;
$precondition$;

ALTER TABLE station_gap_openings
    ADD COLUMN semantic_uncertainty_contract JSONB NOT NULL,
    ADD COLUMN semantic_uncertainty_contract_sha256 TEXT NOT NULL,
    ADD CONSTRAINT station_gap_openings_semantic_uncertainty_shape CHECK (
        jsonb_typeof(semantic_uncertainty_contract)='object' AND
        semantic_uncertainty_contract ?& ARRAY[
            'id','work_kind','exact_question','deterministic_limitation',
            'required_information','single_result','deterministic_consumer'
        ] AND
        semantic_uncertainty_contract-
            'id'-'work_kind'-'exact_question'-'deterministic_limitation'-
            'required_information'-'single_result'-'deterministic_consumer'='{}'::jsonb AND
        jsonb_typeof(semantic_uncertainty_contract->'id')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'work_kind')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'exact_question')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_limitation')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'required_information')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'single_result')='string' AND
        jsonb_typeof(semantic_uncertainty_contract->'deterministic_consumer')='string' AND
        semantic_uncertainty_contract->>'work_kind'=work_kind AND
        semantic_uncertainty_contract->>'id'=
            'omnidex.semantic-uncertainty.'||work_kind||'.v1' AND
        octet_length(semantic_uncertainty_contract->>'id') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'id'=btrim(semantic_uncertainty_contract->>'id') AND
        octet_length(semantic_uncertainty_contract->>'exact_question') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'exact_question'=
            btrim(semantic_uncertainty_contract->>'exact_question') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_limitation') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_limitation'=
            btrim(semantic_uncertainty_contract->>'deterministic_limitation') AND
        octet_length(semantic_uncertainty_contract->>'required_information') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'required_information'=
            btrim(semantic_uncertainty_contract->>'required_information') AND
        octet_length(semantic_uncertainty_contract->>'single_result') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'single_result'=
            btrim(semantic_uncertainty_contract->>'single_result') AND
        octet_length(semantic_uncertainty_contract->>'deterministic_consumer') BETWEEN 1 AND 512 AND
        semantic_uncertainty_contract->>'deterministic_consumer'=
            btrim(semantic_uncertainty_contract->>'deterministic_consumer') AND
        length(semantic_uncertainty_contract->>'exact_question')-
            length(replace(semantic_uncertainty_contract->>'exact_question','?',''))=1 AND
        right(semantic_uncertainty_contract->>'exact_question',1)='?' AND
        left(semantic_uncertainty_contract->>'single_result',4)='One ' AND
        (
            (semantic_uncertainty_contract->>'id')||
            (semantic_uncertainty_contract->>'work_kind')||
            (semantic_uncertainty_contract->>'exact_question')||
            (semantic_uncertainty_contract->>'deterministic_limitation')||
            (semantic_uncertainty_contract->>'required_information')||
            (semantic_uncertainty_contract->>'single_result')||
            (semantic_uncertainty_contract->>'deterministic_consumer')
        ) !~ E'[\\r\\n]'
    ),
    ADD CONSTRAINT station_gap_openings_semantic_uncertainty_digest CHECK (
        semantic_uncertainty_contract_sha256~'^[0-9a-f]{64}$' AND
        semantic_uncertainty_contract_sha256=encode(digest(
            convert_to(semantic_uncertainty_contract->>'id','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'work_kind','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'exact_question','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'deterministic_limitation','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'required_information','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'single_result','UTF8')||decode('00','hex')||
            convert_to(semantic_uncertainty_contract->>'deterministic_consumer','UTF8')||decode('00','hex'),
            'sha256'
        ),'hex')
    );

DO $postcondition$
DECLARE
    observed_constraint_hashes TEXT;
BEGIN
    SELECT string_agg(
        conname||'='||encode(digest(
            convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'
        ),'hex'),', ' ORDER BY conname
    ) INTO observed_constraint_hashes
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname IN (
          'station_gap_openings_semantic_uncertainty_shape',
          'station_gap_openings_semantic_uncertainty_digest'
      );
    IF EXISTS (
        SELECT 1
        FROM (VALUES
            ('station_gap_openings_semantic_uncertainty_shape',
             'ab24dce607eb8b487a5428124003ff8d1936cfb4119613741a79436952cc9583'),
            ('station_gap_openings_semantic_uncertainty_digest',
             'e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea')
        ) AS expected(constraint_name,definition_sha256)
        LEFT JOIN pg_constraint AS actual
          ON actual.conrelid='station_gap_openings'::regclass
         AND actual.conname=expected.constraint_name
         AND actual.contype='c'
        WHERE actual.oid IS NULL OR actual.convalidated IS DISTINCT FROM TRUE OR
              encode(digest(
                  convert_to(pg_get_constraintdef(actual.oid),'UTF8'),'sha256'
              ),'hex')<>expected.definition_sha256
    ) OR EXISTS (
        SELECT 1
        FROM pg_attribute
        WHERE attrelid='station_gap_openings'::regclass
          AND attname IN (
              'semantic_uncertainty_contract',
              'semantic_uncertainty_contract_sha256'
          ) AND (
              attisdropped OR NOT attnotnull OR atthasdef OR attgenerated<>'' OR
              CASE attname
                  WHEN 'semantic_uncertainty_contract' THEN
                      format_type(atttypid,atttypmod)<>'jsonb'
                  ELSE format_type(atttypid,atttypmod)<>'text'
              END
          )
    ) OR (
        SELECT COUNT(*) FROM pg_attribute
        WHERE attrelid='station_gap_openings'::regclass
          AND attname IN (
              'semantic_uncertainty_contract',
              'semantic_uncertainty_contract_sha256'
          ) AND NOT attisdropped
    )<>2 OR EXISTS (SELECT 1 FROM station_gap_openings) THEN
        RAISE EXCEPTION
            'semantic uncertainty authority postcondition failed: %',
            observed_constraint_hashes;
    END IF;
END;
$postcondition$;

COMMIT;
