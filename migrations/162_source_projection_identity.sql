BEGIN;

LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    work_identity_source TEXT;
    envelope_authority_source TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO work_identity_source
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND
          conname='station_gap_openings_check5' AND contype='c';
    SELECT pg_get_constraintdef(oid) INTO envelope_authority_source
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND
          conname='station_gap_openings_check6' AND contype='c';
    IF work_identity_source IS NULL OR
       encode(digest(convert_to(work_identity_source,'UTF8'),'sha256'),'hex')<>
       '65d7ff6cd58426c491196ef386f0414f9eb58abaee0bcb6d513b95d61accc4be' OR
       envelope_authority_source IS NULL OR
       encode(digest(convert_to(envelope_authority_source,'UTF8'),'sha256'),'hex')<>
       'bced50709cf6a0558483df99eff168e5348871e912db70b59b2e64449a3964cd' THEN
        RAISE EXCEPTION
            'source projection identity requires the exact prior portable envelope authority';
    END IF;
END $$;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_check5,
    DROP CONSTRAINT station_gap_openings_check6;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_portable_job_identity CHECK (
        work_id=encode(digest(
            convert_to(portable_schema,'UTF8')||decode('00','hex')||
            convert_to(work_kind,'UTF8')||decode('00','hex')||
            convert_to(portable_payload,'UTF8')||
            CASE
                WHEN portable_envelope::jsonb ? 'source_projection' THEN
                    decode('00','hex')||convert_to(
                        portable_envelope::jsonb->>'source_projection','UTF8'
                    )
                ELSE ''::bytea
            END,
            'sha256'
        ),'hex')
    ),
    ADD CONSTRAINT station_gap_openings_portable_envelope_authority CHECK (
        jsonb_typeof(portable_envelope::jsonb)='object' AND
        portable_envelope::jsonb ?& ARRAY['schema','id','kind','payload'] AND
        portable_envelope::jsonb-
            'schema'-'id'-'kind'-'payload'-'source_projection'='{}'::jsonb AND
        portable_envelope::jsonb->>'schema'=portable_schema AND
        portable_envelope::jsonb->>'id'=work_id AND
        portable_envelope::jsonb->>'kind'=work_kind AND
        portable_envelope::jsonb->'payload'=portable_payload::jsonb AND
        (
            NOT (portable_envelope::jsonb ? 'source_projection') OR
            (
                jsonb_typeof(portable_envelope::jsonb->'source_projection')='string' AND
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
    ),
    ADD CONSTRAINT station_gap_openings_source_projection_authority CHECK (
        NOT (
            work_kind='fragment_correction' AND
            portable_payload::jsonb ? 'current_declaration' AND
            NOT (portable_payload::jsonb ? 'language')
        ) OR portable_envelope::jsonb ? 'source_projection'
    ) NOT VALID;

COMMIT;
