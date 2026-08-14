LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    constraint_name TEXT;
    constraint_count INTEGER;
BEGIN
    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(portable_payload) <= 16384%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy portable payload constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_gap_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(portable_envelope) <= 32768%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy portable envelope constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_gap_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(prompt) <= 65536%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy prompt constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_gap_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_gap_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(projection_envelope) <= 131072%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy projection constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_gap_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(wire_request)%131072%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy wire request constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_call_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%wire_request_bytes%131072%octet_length(wire_request)%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy wire byte constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_call_openings DROP CONSTRAINT %I', constraint_name);

    SELECT MIN(conname),COUNT(*) INTO constraint_name,constraint_count
    FROM pg_constraint
    WHERE conrelid='station_call_openings'::regclass AND contype='c' AND
          pg_get_constraintdef(oid) LIKE '%octet_length(model_input)%131072%';
    IF constraint_count<>1 THEN
        RAISE EXCEPTION 'station prompt transport expected one legacy model input constraint, found %',
            constraint_count;
    END IF;
    EXECUTE format('ALTER TABLE station_call_openings DROP CONSTRAINT %I', constraint_name);
END $$;

-- These checks are deliberately loose transport/resource protection. Exact
-- model-input admission and completion remain owned by the provider context
-- contract and tokenizer-counted receipt, never by these byte counts.
ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_portable_payload_resource_ceiling CHECK (
        portable_payload<>'' AND octet_length(portable_payload)<=1048576
    ),
    ADD CONSTRAINT station_gap_openings_portable_envelope_resource_ceiling CHECK (
        octet_length(portable_envelope)<=1048576
    ),
    ADD CONSTRAINT station_gap_openings_prompt_resource_ceiling CHECK (
        prompt<>'' AND BTRIM(prompt)<>'' AND octet_length(prompt)<=1048576
    ),
    ADD CONSTRAINT station_gap_openings_projection_resource_ceiling CHECK (
        octet_length(projection_envelope)<=1048576
    );

ALTER TABLE station_call_openings
    ADD CONSTRAINT station_call_openings_wire_request_resource_ceiling CHECK (
        octet_length(wire_request) BETWEEN 1 AND 1048576
    ),
    ADD CONSTRAINT station_call_openings_wire_request_bytes_resource_ceiling CHECK (
        wire_request_bytes BETWEEN 1 AND 1048576 AND
        wire_request_bytes=octet_length(wire_request)
    ),
    ADD CONSTRAINT station_call_openings_model_input_resource_ceiling CHECK (
        octet_length(model_input) BETWEEN 1 AND 1048576
    );
