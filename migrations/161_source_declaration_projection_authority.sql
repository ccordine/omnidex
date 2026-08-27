BEGIN;

LOCK TABLE station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    station_call_openings, station_call_receipts, llm_call_evidence
    IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    constraint_source TEXT;
    receipt_guard_source TEXT;
    receipt_guard_language TEXT;
    receipt_guard_volatility "char";
    receipt_guard_strict BOOLEAN;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO constraint_source
    FROM pg_constraint
    WHERE conrelid='station_gap_outcomes'::regclass AND
          conname='station_gap_outcomes_projected_response' AND contype='c';
    SELECT procedure.prosrc,language.lanname,procedure.provolatile,
           procedure.proisstrict
    INTO receipt_guard_source,receipt_guard_language,
         receipt_guard_volatility,receipt_guard_strict
    FROM pg_proc AS procedure
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE procedure.oid=to_regprocedure(
        'require_station_call_receipt_before_gap_outcome()'
    );
    IF constraint_source IS NULL OR
       encode(digest(convert_to(constraint_source,'UTF8'),'sha256'),'hex')<>
       '7107b5b1702f19fc23bd2d8d196edeb40c48132a921d22b373d64ca5c29e6409' OR
       receipt_guard_source IS NULL OR receipt_guard_language<>'plpgsql' OR
       receipt_guard_volatility<>'v' OR receipt_guard_strict IS DISTINCT FROM FALSE OR
       encode(digest(convert_to(receipt_guard_source,'UTF8'),'sha256'),'hex')<>
       '1738890c05fe810cf0978011e6e512f7a132e8622b1eaee3cedda4cd7618b163' THEN
        RAISE EXCEPTION
            'source declaration projection requires the exact prior projection authority';
    END IF;
END $$;

ALTER TABLE station_gap_outcomes
    DROP CONSTRAINT station_gap_outcomes_projected_response;

ALTER TABLE station_gap_outcomes
    ADD CONSTRAINT station_gap_outcomes_projected_response CHECK (
        (status='resolved' AND response IS NOT NULL AND BTRIM(response)<>'' AND
            octet_length(response)<=16777216 AND response_sha256~'^[0-9a-f]{64}$' AND
            response_sha256=encode(digest(response,'sha256'),'hex') AND error IS NULL AND
            projection_kind IS NOT NULL AND
            projection_kind IN (
                'exact_response','source_declaration','typescript_function'
            ) AND
            call_receipt_sha256 IS NOT NULL AND call_receipt_sha256~'^[0-9a-f]{64}$' AND
            source_response_sha256 IS NOT NULL AND source_response_sha256~'^[0-9a-f]{64}$' AND
            source_start_byte IS NOT NULL AND source_end_byte IS NOT NULL AND
            source_start_byte>=0 AND source_end_byte>source_start_byte AND
            source_end_byte-source_start_byte=octet_length(response)) OR
        (status='failed' AND response IS NULL AND response_sha256 IS NULL AND
            projection_kind IS NULL AND call_receipt_sha256 IS NULL AND
            source_response_sha256 IS NULL AND source_start_byte IS NULL AND
            source_end_byte IS NULL AND error IS NOT NULL AND BTRIM(error)<>'' AND
            octet_length(error)<=8192)
    ) NOT VALID;

CREATE OR REPLACE FUNCTION require_station_call_receipt_before_gap_outcome()
RETURNS TRIGGER AS $$
DECLARE
    discovery_count INTEGER;
    discovery_status TEXT;
    call_count INTEGER;
    call_status TEXT;
    call_response TEXT;
    call_receipt_sha256 TEXT;
    call_response_sha256 TEXT;
    evidence_count INTEGER;
    gap_work_kind TEXT;
    gap_payload JSONB;
BEGIN
    SELECT COUNT(*),MIN(receipts.status) INTO discovery_count,discovery_status
    FROM station_provider_discoveries discoveries
    LEFT JOIN station_provider_discovery_receipts receipts
      ON receipts.opening_id=discoveries.id
    WHERE discoveries.gap_opening_id=NEW.opening_id;

    SELECT COUNT(*),MIN(receipts.status),MIN(receipts.generation_json::jsonb->>'content'),
           MIN(receipts.generation_sha256),MIN(evidence.response_sha256),COUNT(evidence.id)
    INTO call_count,call_status,call_response,call_receipt_sha256,
         call_response_sha256,evidence_count
    FROM station_call_openings calls
    LEFT JOIN station_call_receipts receipts ON receipts.opening_id=calls.id
    LEFT JOIN llm_call_evidence evidence ON evidence.station_call_opening_id=calls.id
    WHERE calls.gap_opening_id=NEW.opening_id;

    SELECT work_kind,portable_payload::jsonb INTO gap_work_kind,gap_payload
    FROM station_gap_openings WHERE id=NEW.opening_id;

    IF discovery_count<>1 OR discovery_status IS NULL THEN
        RAISE EXCEPTION
            'station gap outcome requires one terminal provider discovery receipt';
    END IF;
    IF call_count>0 AND (call_status IS NULL OR evidence_count<>call_count) THEN
        RAISE EXCEPTION
            'station gap outcome requires one immutable evidence row for every terminal provider call';
    END IF;
    IF NEW.status='resolved' AND
       (discovery_status<>'succeeded' OR call_count<>1 OR call_status<>'succeeded' OR
        NEW.call_receipt_sha256 IS DISTINCT FROM call_receipt_sha256 OR
        NEW.source_response_sha256 IS DISTINCT FROM call_response_sha256 OR
        NEW.source_end_byte>octet_length(call_response) OR
        substring(convert_to(call_response,'UTF8') FROM NEW.source_start_byte+1
                  FOR NEW.source_end_byte-NEW.source_start_byte)
            IS DISTINCT FROM convert_to(NEW.response,'UTF8') OR
        (NEW.projection_kind='exact_response' AND
            (NEW.source_start_byte<>0 OR
             NEW.source_end_byte<>octet_length(call_response))) OR
        (NEW.projection_kind='source_declaration' AND NOT (
            gap_work_kind='fragment_correction' OR
            (gap_work_kind='fragment_generation' AND
             gap_payload->>'language' IN ('go','javascript','java','rust','php')) OR
            (gap_work_kind='fragment_modification' AND
             gap_payload->>'language'='go')
        )) OR
        (NEW.projection_kind='typescript_function' AND NOT (
            gap_work_kind IN ('fragment_generation','fragment_correction') AND
            gap_payload->>'language'='typescript' AND
            NOT (gap_payload ? 'repair_region')
        ))) THEN
        RAISE EXCEPTION
            'resolved station gap projection differs from its exact provider receipt';
    END IF;
    IF NEW.status='failed' AND discovery_status='succeeded' AND
       (call_count<>1 OR call_status IS NULL) THEN
        RAISE EXCEPTION
            'failed station gap requires its terminal provider call receipt';
    END IF;
    IF NEW.status='failed' AND discovery_status='failed' AND call_count<>0 THEN
        RAISE EXCEPTION
            'failed provider discovery cannot have a provider call';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMIT;
