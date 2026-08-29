BEGIN;

LOCK TABLE station_gap_openings, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $precondition$
DECLARE
    function_source TEXT;
    function_language TEXT;
    function_volatility "char";
    function_strict BOOLEAN;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (VALUES
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_check7',
                '78836fc90f5d8eabafe093461c49331932f114cf684def5ed6aebdf0610677bc'
            ),
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_current_raw_transport',
                '226a74d0e551571c652eb19100abc59fde8483a6d88057867fcfb9ad4e8d6a77'
            ),
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_renderer_version_check',
                '45db118259a5e0022be45eacdde9f2cbc2c025aea050b5131081358c90884e0b'
            ),
            (
                'llm_call_evidence'::regclass,
                'llm_call_evidence_check1',
                '720ae7b8a23f4cb60fb48a1fdec70579b3dbd20ffd31890e7f2119bb413053bd'
            ),
            (
                'llm_call_evidence'::regclass,
                'llm_call_evidence_current_raw_transport',
                'f560297e22e57a367dfa531ed637372c8d5208b540c796599b9fbd7cb5e44d45'
            )
        ) AS expected(relation_oid,constraint_name,definition_sha256)
        LEFT JOIN pg_constraint AS actual
          ON actual.conrelid=expected.relation_oid
         AND actual.conname=expected.constraint_name
         AND actual.contype='c'
        WHERE actual.oid IS NULL OR actual.convalidated IS DISTINCT FROM TRUE OR
              encode(digest(
                  convert_to(pg_get_constraintdef(actual.oid),'UTF8'),
                  'sha256'
              ),'hex')<>expected.definition_sha256
    ) THEN
        RAISE EXCEPTION
            'response-schema authority retirement requires the exact prior constraints';
    END IF;

    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
      INTO function_source,function_language,function_volatility,
           function_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure('require_llm_call_station_gap()');

    IF function_source IS NULL OR function_language<>'plpgsql' OR
       function_volatility<>'v' OR function_strict OR
       encode(digest(convert_to(function_source,'UTF8'),'sha256'),'hex')<>
       '43d018602e2004cb252c9fe87e0aa64052b1ea3e2494729279f33936dec524d9' OR
       NOT EXISTS (
           SELECT 1
           FROM pg_trigger
           WHERE tgrelid='llm_call_evidence'::regclass
             AND tgname='llm_call_evidence_require_station_gap'
             AND tgfoid=to_regprocedure('require_llm_call_station_gap()')
             AND tgenabled='O'
             AND NOT tgisinternal
       ) OR NOT EXISTS (
           SELECT 1
           FROM pg_attribute
           WHERE attrelid='station_gap_openings'::regclass
             AND attname='response_schema'
             AND format_type(atttypid,atttypmod)='text'
             AND attnotnull
             AND NOT attisdropped
       ) OR NOT EXISTS (
           SELECT 1
           FROM pg_attribute
           WHERE attrelid='llm_call_evidence'::regclass
             AND attname='response_schema'
             AND format_type(atttypid,atttypmod)='jsonb'
             AND NOT attnotnull
             AND NOT attisdropped
       ) THEN
        RAISE EXCEPTION
            'response-schema authority retirement requires the exact prior columns and trigger function';
    END IF;

    IF EXISTS (SELECT 1 FROM station_gap_openings) OR
       EXISTS (SELECT 1 FROM llm_call_evidence) THEN
        RAISE EXCEPTION
            'response-schema authority retirement requires a fresh reset: immutable inference history exists';
    END IF;
END;
$precondition$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_check7,
    DROP CONSTRAINT station_gap_openings_current_raw_transport,
    DROP CONSTRAINT station_gap_openings_renderer_version_check,
    DROP COLUMN response_schema;

ALTER TABLE llm_call_evidence
    DROP CONSTRAINT llm_call_evidence_check1,
    DROP CONSTRAINT llm_call_evidence_current_raw_transport,
    DROP COLUMN response_schema;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version='omnidex.render-portable-job.v5'
    ),
    ADD CONSTRAINT station_gap_openings_projection_v5 CHECK (
        jsonb_typeof(projection_envelope::jsonb)='object' AND
        projection_envelope::jsonb ?& ARRAY['prompt','renderer'] AND
        projection_envelope::jsonb-'prompt'-'renderer'='{}'::jsonb AND
        jsonb_typeof(projection_envelope::jsonb->'prompt')='string' AND
        jsonb_typeof(projection_envelope::jsonb->'renderer')='string' AND
        projection_envelope::jsonb->>'prompt'=prompt AND
        projection_envelope::jsonb->>'renderer'=renderer_version
    ),
    ADD CONSTRAINT station_gap_openings_current_raw_transport CHECK (
        CASE
            WHEN work_kind='application_target_tree' THEN
                scope='portable_structural_worker'
            WHEN work_kind IN (
                'fragment_generation',
                'fragment_modification',
                'fragment_correction'
            ) THEN scope='portable_fragment_worker'
            ELSE scope='portable_semantic_worker'
        END
    );

ALTER TABLE llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_current_raw_transport CHECK (
        response_format='text'
    );

CREATE OR REPLACE FUNCTION require_llm_call_station_gap()
RETURNS TRIGGER AS $$
DECLARE
    opening station_gap_openings%ROWTYPE;
    receipt station_call_receipts%ROWTYPE;
BEGIN
    IF NEW.station_call_opening_id IS NULL THEN
        RAISE EXCEPTION 'new LLM call evidence requires one persisted station call opening';
    END IF;
    SELECT gaps.* INTO opening FROM station_gap_openings AS gaps
    JOIN station_call_openings AS calls ON calls.gap_opening_id=gaps.id
    WHERE calls.id=NEW.station_call_opening_id FOR SHARE OF gaps;
    SELECT * INTO receipt FROM station_call_receipts
    WHERE opening_id=NEW.station_call_opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.job_generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,
           NEW.scope,NEW.work_id,NEW.work_kind,NEW.system_prompt,
           NEW.context_tokens,NEW.max_output_tokens)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,
           opening.worker_id,opening.scope,opening.work_id,opening.work_kind,opening.prompt,
           opening.context_tokens,opening.max_output_tokens) OR
       receipt.id IS NULL OR NEW.status::text IS DISTINCT FROM (CASE receipt.status
           WHEN 'succeeded' THEN 'succeeded' ELSE 'generation_failed' END) OR
       NEW.response IS DISTINCT FROM NULLIF(receipt.generation_json::jsonb->>'content','') OR
       (receipt.status='failed' AND NEW.error IS NULL) OR
       (receipt.status='succeeded' AND NEW.error IS NOT NULL) THEN
        RAISE EXCEPTION 'LLM call evidence does not match its exact station gap opening';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $postcondition$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_attribute
        WHERE attrelid IN (
            'station_gap_openings'::regclass,
            'llm_call_evidence'::regclass
        ) AND attname='response_schema' AND NOT attisdropped
    ) OR EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='station_gap_openings'::regclass
          AND conname='station_gap_openings_check7'
    ) OR EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid='llm_call_evidence'::regclass
          AND conname='llm_call_evidence_check1'
    ) OR EXISTS (
        SELECT 1
        FROM pg_proc
        WHERE oid=to_regprocedure('require_llm_call_station_gap()')
          AND prosrc LIKE '%response_schema%'
    ) THEN
        RAISE EXCEPTION
            'response-schema authority retirement left retired database authority';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM (VALUES
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_renderer_version_check',
                '522c94915fab9d58a737582594a176fddd2c80322ecaf687edeeef34cd421605'
            ),
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_projection_v5',
                '8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad'
            ),
            (
                'station_gap_openings'::regclass,
                'station_gap_openings_current_raw_transport',
                'a147cbfc147e9f009ab9a4f51c0fac68ec42a5a7e44df60a67671955ac0afc7e'
            ),
            (
                'llm_call_evidence'::regclass,
                'llm_call_evidence_current_raw_transport',
                'cf2775374afaa1cd7948e9edc383d4ee4b52661e329baf5a7dcef10c59751dd7'
            )
        ) AS expected(relation_oid,constraint_name,definition_sha256)
        LEFT JOIN pg_constraint AS actual
          ON actual.conrelid=expected.relation_oid
         AND actual.conname=expected.constraint_name
         AND actual.contype='c'
        WHERE actual.oid IS NULL OR actual.convalidated IS DISTINCT FROM TRUE OR
              encode(digest(
                  convert_to(pg_get_constraintdef(actual.oid),'UTF8'),
                  'sha256'
              ),'hex')<>expected.definition_sha256
    ) OR NOT EXISTS (
        SELECT 1
        FROM pg_proc AS procedure
        JOIN pg_language AS language ON language.oid=procedure.prolang
        WHERE procedure.oid=to_regprocedure('require_llm_call_station_gap()')
          AND language.lanname='plpgsql'
          AND procedure.provolatile='v'
          AND NOT procedure.proisstrict
          AND encode(digest(
              convert_to(procedure.prosrc,'UTF8'),'sha256'
          ),'hex')='137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50'
    ) OR NOT EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgrelid='llm_call_evidence'::regclass
          AND tgname='llm_call_evidence_require_station_gap'
          AND tgfoid=to_regprocedure('require_llm_call_station_gap()')
          AND tgenabled='O'
          AND NOT tgisinternal
    ) THEN
        RAISE EXCEPTION
            'response-schema authority retirement postcondition failed';
    END IF;
END;
$postcondition$;

COMMIT;
