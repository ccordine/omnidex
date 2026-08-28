BEGIN;

LOCK TABLE roleplay_portable_result_reuses IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    schema_definition TEXT;
    id_definition TEXT;
    kind_definition TEXT;
    payload_definition TEXT;
    identity_definition TEXT;
    schema_validated BOOLEAN;
    id_validated BOOLEAN;
    kind_validated BOOLEAN;
    payload_validated BOOLEAN;
    identity_validated BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO schema_definition,schema_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_target_portable_envelope_check1'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO id_definition,id_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_check3'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO kind_definition,kind_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_check4'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO payload_definition,payload_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_check5'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO identity_definition,identity_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_check6'
      AND contype='c';

    IF schema_definition IS NULL OR schema_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(schema_definition,'UTF8'),'sha256'),'hex')<>
       'b86a3011debcab72d5f5b0d2da4f416cb2062c688640b412945170997e75a491' OR
       id_definition IS NULL OR id_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(id_definition,'UTF8'),'sha256'),'hex')<>
       '76ddc54c99e377319580620b9f74bb220940fb2e1abf04eaa828d1f7918e3d5f' OR
       kind_definition IS NULL OR kind_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(kind_definition,'UTF8'),'sha256'),'hex')<>
       '021fa2a48d50615e2f012746b3a5bbca158b2db0679162f03889f0dc1e43c94e' OR
       payload_definition IS NULL OR payload_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(payload_definition,'UTF8'),'sha256'),'hex')<>
       '56f11c1bcafa04d69666f21d37220dddaddb16374763d1208b2adb74be9d800e' OR
       identity_definition IS NULL OR identity_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(identity_definition,'UTF8'),'sha256'),'hex')<>
       'cc977547c1f6a142ab1dc18815e1e01458a0956b760bb527ed24c1bd9bb7bc68' THEN
        RAISE EXCEPTION
            'roleplay portable result reuse V2 requires the exact prior V1 target authority';
    END IF;

    -- Before migration 163, every station opening was constrained to the V1
    -- envelope. Migration 163 rejects those openings, and every reuse receipt
    -- references one source opening. The old target check then rejects every
    -- V2 envelope emitted after that cutover. Nonempty reuse state therefore
    -- cannot be legitimate authority on a database that crossed migration 163.
    IF EXISTS (SELECT 1 FROM roleplay_portable_result_reuses) THEN
        RAISE EXCEPTION
            'roleplay portable result reuse V2 requires fresh reuse state established by migration 163';
    END IF;
END $$;

ALTER TABLE roleplay_portable_result_reuses
    DROP CONSTRAINT roleplay_portable_result_reuses_target_portable_envelope_check1,
    DROP CONSTRAINT roleplay_portable_result_reuses_check3,
    DROP CONSTRAINT roleplay_portable_result_reuses_check4,
    DROP CONSTRAINT roleplay_portable_result_reuses_check5,
    DROP CONSTRAINT roleplay_portable_result_reuses_check6;

ALTER TABLE roleplay_portable_result_reuses
    ADD CONSTRAINT roleplay_portable_result_reuses_target_schema_v2 CHECK (
        jsonb_typeof(target_portable_envelope::jsonb->'schema')='string' AND
        target_portable_envelope::jsonb->>'schema'
            IS NOT DISTINCT FROM 'omnidex.portable-job.v2'
    ),
    ADD CONSTRAINT roleplay_portable_result_reuses_target_envelope_v2 CHECK (
        jsonb_typeof(target_portable_envelope::jsonb)='object' AND
        target_portable_envelope::jsonb ?& ARRAY['schema','id','kind','payload'] AND
        target_portable_envelope::jsonb-
            'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb AND
        jsonb_typeof(target_portable_envelope::jsonb->'schema')='string' AND
        jsonb_typeof(target_portable_envelope::jsonb->'id')='string' AND
        jsonb_typeof(target_portable_envelope::jsonb->'kind')='string' AND
        target_portable_envelope::jsonb->>'id'
            IS NOT DISTINCT FROM target_root_work_id AND
        target_portable_envelope::jsonb->>'kind'
            IS NOT DISTINCT FROM target_work_kind AND
        target_portable_envelope::jsonb->'payload'
            IS NOT DISTINCT FROM target_portable_payload::jsonb AND
        (
            NOT (target_portable_envelope::jsonb ? 'source_projection') OR
            (
                jsonb_typeof(
                    target_portable_envelope::jsonb->'source_projection'
                )='string' AND
                target_portable_envelope::jsonb->>'source_projection' IN (
                    'go','javascript','java','rust','php'
                ) AND
                target_work_kind='fragment_correction' AND
                target_portable_payload::jsonb ?& ARRAY[
                    'current_declaration','repair_guidance'
                ] AND
                target_portable_payload::jsonb-
                    'current_declaration'-'repair_guidance'='{}'::jsonb
            )
        )
    ),
    ADD CONSTRAINT roleplay_portable_result_reuses_target_identity_v2 CHECK (
        target_root_work_id IS NOT DISTINCT FROM encode(digest(
            convert_to(
                (target_portable_envelope::jsonb)->>'schema','UTF8'
            )||decode('00','hex')||
            convert_to(target_work_kind,'UTF8')||decode('00','hex')||
            convert_to(target_portable_payload,'UTF8')||
            CASE
                WHEN target_portable_envelope::jsonb ? 'source_projection' THEN
                    decode('00','hex')||convert_to(
                        target_portable_envelope::jsonb->>'source_projection','UTF8'
                    )
                ELSE ''::bytea
            END,
            'sha256'
        ),'hex')
    );

DO $$
DECLARE
    schema_definition TEXT;
    envelope_definition TEXT;
    identity_definition TEXT;
    schema_validated BOOLEAN;
    envelope_validated BOOLEAN;
    identity_validated BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO schema_definition,schema_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_target_schema_v2'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO envelope_definition,envelope_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_target_envelope_v2'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO identity_definition,identity_validated
    FROM pg_constraint
    WHERE conrelid='roleplay_portable_result_reuses'::regclass
      AND conname='roleplay_portable_result_reuses_target_identity_v2'
      AND contype='c';

    IF schema_definition IS NULL OR schema_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(schema_definition,'UTF8'),'sha256'),'hex')<>
       'b4db41260539620703cf92cdf4698ae8ec37380bd25478b999e99c7a8acf6e47' OR
       envelope_definition IS NULL OR envelope_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(envelope_definition,'UTF8'),'sha256'),'hex')<>
       '26fd0a7d5339490a764e80e8b0cb0cb35ac6116ce6a33d40740a10df22c9c8a7' OR
       identity_definition IS NULL OR identity_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(identity_definition,'UTF8'),'sha256'),'hex')<>
       '26eff63bbc0307313aa3f6ba6570bd14cb335dbb4de7f92019285803c819858e' OR
       EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='roleplay_portable_result_reuses'::regclass
             AND conname IN (
                 'roleplay_portable_result_reuses_target_portable_envelope_check1',
                 'roleplay_portable_result_reuses_check3',
                 'roleplay_portable_result_reuses_check4',
                 'roleplay_portable_result_reuses_check5',
                 'roleplay_portable_result_reuses_check6'
             )
       ) THEN
        RAISE EXCEPTION
            'roleplay portable result reuse V2 target authority postcondition failed';
    END IF;
END $$;

COMMIT;
