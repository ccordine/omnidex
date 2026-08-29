BEGIN;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    envelope_definition TEXT;
    identity_definition TEXT;
    envelope_validated BOOLEAN;
    identity_validated BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO envelope_definition,envelope_validated
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_portable_envelope_authority'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO identity_definition,identity_validated
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_portable_job_identity'
      AND contype='c';

    IF envelope_definition IS NULL OR envelope_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(envelope_definition,'UTF8'),'sha256'),'hex')<>
       'c409ff59831ba5afce4ab802bd8e1c695c3f01db117bf04456841c007cd60c63' OR
       identity_definition IS NULL OR identity_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(identity_definition,'UTF8'),'sha256'),'hex')<>
       'd50107974696b6779d265ef6b03910ce85ccfb0ce84097f88b794e704179b1d3' THEN
        RAISE EXCEPTION
            'station gap opening portable envelope V2 requires the exact prior portable envelope and job identity authority';
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM station_gap_openings
        WHERE (
            jsonb_typeof(portable_envelope::jsonb)='object' AND
            portable_envelope::jsonb ?& ARRAY['schema','id','kind','payload'] AND
            portable_envelope::jsonb-
                'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb AND
            jsonb_typeof(portable_envelope::jsonb->'schema')='string' AND
            jsonb_typeof(portable_envelope::jsonb->'id')='string' AND
            jsonb_typeof(portable_envelope::jsonb->'kind')='string' AND
            portable_envelope::jsonb->>'schema'
                IS NOT DISTINCT FROM portable_schema AND
            portable_envelope::jsonb->>'id'
                IS NOT DISTINCT FROM work_id AND
            portable_envelope::jsonb->>'kind'
                IS NOT DISTINCT FROM work_kind AND
            portable_envelope::jsonb->'payload'
                IS NOT DISTINCT FROM portable_payload::jsonb AND
            (
                NOT (portable_envelope::jsonb ? 'source_projection') OR
                (
                    jsonb_typeof(
                        portable_envelope::jsonb->'source_projection'
                    )='string' AND
                    portable_envelope::jsonb->>'source_projection' IN (
                        'go','javascript','java','rust','php'
                    ) AND
                    work_kind='fragment_correction' AND
                    portable_payload::jsonb ?& ARRAY[
                        'current_declaration','repair_guidance'
                    ] AND
                    portable_payload::jsonb-
                        'current_declaration'-'repair_guidance'='{}'::jsonb
                )
            )
        ) IS DISTINCT FROM TRUE
    ) THEN
        RAISE EXCEPTION
            'station gap opening portable envelope V2 rejects malformed existing envelope authority';
    END IF;
END $$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_portable_envelope_authority;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_portable_envelope_v2 CHECK (
        jsonb_typeof(portable_envelope::jsonb)='object' AND
        portable_envelope::jsonb ?& ARRAY['schema','id','kind','payload'] AND
        portable_envelope::jsonb-
            'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb AND
        jsonb_typeof(portable_envelope::jsonb->'schema')='string' AND
        jsonb_typeof(portable_envelope::jsonb->'id')='string' AND
        jsonb_typeof(portable_envelope::jsonb->'kind')='string' AND
        portable_envelope::jsonb->>'schema'
            IS NOT DISTINCT FROM portable_schema AND
        portable_envelope::jsonb->>'id'
            IS NOT DISTINCT FROM work_id AND
        portable_envelope::jsonb->>'kind'
            IS NOT DISTINCT FROM work_kind AND
        portable_envelope::jsonb->'payload'
            IS NOT DISTINCT FROM portable_payload::jsonb AND
        (
            NOT (portable_envelope::jsonb ? 'source_projection') OR
            (
                jsonb_typeof(
                    portable_envelope::jsonb->'source_projection'
                )='string' AND
                portable_envelope::jsonb->>'source_projection' IN (
                    'go','javascript','java','rust','php'
                ) AND
                work_kind='fragment_correction' AND
                portable_payload::jsonb ?& ARRAY[
                    'current_declaration','repair_guidance'
                ] AND
                portable_payload::jsonb-
                    'current_declaration'-'repair_guidance'='{}'::jsonb
            )
        )
    );

DO $$
DECLARE
    envelope_definition TEXT;
    identity_definition TEXT;
    envelope_validated BOOLEAN;
    identity_validated BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid),convalidated
      INTO envelope_definition,envelope_validated
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_portable_envelope_v2'
      AND contype='c';
    SELECT pg_get_constraintdef(oid),convalidated
      INTO identity_definition,identity_validated
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass
      AND conname='station_gap_openings_portable_job_identity'
      AND contype='c';

    IF envelope_definition IS NULL OR envelope_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(envelope_definition,'UTF8'),'sha256'),'hex')<>
       '2224a480e0a2f3b57d1c3edae9f7e82ff1ec8dec1f56097a969c39c2c59e6a19' OR
       identity_definition IS NULL OR identity_validated IS DISTINCT FROM TRUE OR
       encode(digest(convert_to(identity_definition,'UTF8'),'sha256'),'hex')<>
       'd50107974696b6779d265ef6b03910ce85ccfb0ce84097f88b794e704179b1d3' OR
       EXISTS (
           SELECT 1 FROM pg_constraint
           WHERE conrelid='station_gap_openings'::regclass
             AND conname='station_gap_openings_portable_envelope_authority'
       ) THEN
        RAISE EXCEPTION
            'station gap opening portable envelope V2 authority postcondition failed';
    END IF;
END $$;

COMMIT;
