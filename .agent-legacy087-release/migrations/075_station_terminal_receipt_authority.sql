LOCK TABLE station_provider_discovery_receipts, station_call_receipts,
    station_call_openings, station_gap_outcomes, llm_call_evidence,
    job_step_attempts IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
BEGIN
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('validate_station_provider_discovery_receipt_insert()');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> '446e4967f4e6d51aedf280338a5e956e074d9d8c4eb005c4cf612f7cb3d2b8cd' THEN
        RAISE EXCEPTION 'cannot install terminal receipt authority: discovery validator hash % is not frozen', observed_sha256;
    END IF;
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('validate_station_call_receipt_insert()');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> '56cf275de4cdef8114fa9969f419cf35fb022cff2bdb7bfc7245890090c187ed' THEN
        RAISE EXCEPTION 'cannot install terminal receipt authority: call validator hash % is not frozen', observed_sha256;
    END IF;
    SELECT p.prosrc INTO observed_source
    FROM pg_proc AS p
    WHERE p.oid=to_regprocedure('require_station_call_receipt_before_gap_outcome()');
    observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
    IF observed_sha256 <> '487257a8fa2248b32747d6e7342d8bd370b0c914f291d8229472ea3bf4911311' THEN
        RAISE EXCEPTION 'cannot install terminal receipt authority: gap validator hash % is not frozen', observed_sha256;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM station_provider_discovery_receipts WHERE status='failed') THEN
        RAISE EXCEPTION 'cannot classify historical failed provider discovery receipts without an explicit authority decision';
    END IF;
    IF EXISTS (
        SELECT 1 FROM station_call_receipts receipts
        LEFT JOIN llm_call_evidence evidence ON evidence.station_call_opening_id=receipts.opening_id
        WHERE evidence.id IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot install terminal receipt authority while station receipts lack immutable evidence';
    END IF;
END $$;

DROP TRIGGER station_provider_discovery_receipts_validate_insert
    ON station_provider_discovery_receipts;
CREATE OR REPLACE FUNCTION validate_station_provider_discovery_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_provider_discoveries%ROWTYPE;
    capture_count INTEGER;
    mismatched_captures INTEGER;
    evidence JSONB;
    envelope JSONB;
    failure_reason TEXT;
BEGIN
    SELECT * INTO opening FROM station_provider_discoveries WHERE id=NEW.opening_id FOR SHARE;
    envelope := NEW.observation::jsonb;
    evidence := envelope->'evidence';
    failure_reason := envelope->>'failure_reason';
    SELECT COUNT(*) INTO capture_count FROM station_provider_discovery_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatched_captures
    FROM station_provider_discovery_captures AS captures
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(evidence->'operations') WITH ORDINALITY AS values(item,ordinality)
        WHERE values.ordinality=captures.operation_index+1
    ) operations ON TRUE
    WHERE captures.opening_id=NEW.opening_id AND (
        operations.item IS NULL OR operations.item->>'operation'<>captures.operation OR
        operations.item->>'method'<>captures.method OR operations.item->>'endpoint'<>captures.endpoint OR
        operations.item->>'request_disposition'<>captures.request_disposition OR
        operations.item->>'request_sha256'<>captures.request_sha256 OR
        (operations.item->>'request_bytes')::INTEGER<>captures.request_bytes OR
        (operations.item->>'http_status')::INTEGER<>captures.http_status OR
        operations.item->>'disposition'<>captures.disposition OR
        (operations.item->>'response_complete')::BOOLEAN IS DISTINCT FROM captures.response_complete OR
        operations.item->'content_encoding' IS DISTINCT FROM captures.content_encoding::jsonb OR
        operations.item->>'response_sha256'<>captures.response_sha256 OR
        (operations.item->>'response_bytes')::INTEGER<>captures.response_bytes
    );
    IF opening.id IS NULL OR capture_count<>5 OR mismatched_captures<>0 OR
       jsonb_array_length(evidence->'operations')<>5 OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.status='succeeded' AND (
           failure_reason IS NOT NULL OR envelope ? 'failure_reason' OR
           envelope->>'challenge_sha256'<>opening.challenge
       )) OR
       (NEW.status='failed' AND (
           failure_reason NOT IN ('evidence_rejected','observation_rejected','provider_contract_rejected') OR
           (failure_reason='evidence_rejected' AND
              (envelope ? 'attestation' OR envelope ? 'observation')) OR
           (failure_reason<>'evidence_rejected' AND
              (NOT envelope ? 'attestation' OR NOT envelope ? 'observation'))
       )) THEN
        RAISE EXCEPTION 'station provider discovery receipt differs from its typed opening, captures, or failure authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_provider_discovery_receipts_validate_insert
BEFORE INSERT ON station_provider_discovery_receipts
FOR EACH ROW EXECUTE FUNCTION validate_station_provider_discovery_receipt_insert();

DROP TRIGGER station_call_receipts_validate_insert ON station_call_receipts;
CREATE OR REPLACE FUNCTION validate_station_call_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_call_openings%ROWTYPE;
    attempt_status TEXT;
    attempt_worker TEXT;
    receipt_generation JSONB;
    identity_count INTEGER;
    response_count INTEGER;
    mismatched_identity INTEGER;
    authority_reason TEXT;
    zero_observation JSONB := '{
        "schema":"","observed_at":"0001-01-01T00:00:00Z","attestation_sha256":"",
        "version_body_sha256":"","installed_body_sha256":"","tokenizer_request_sha256":"",
        "tokenizer_body_sha256":"","preload_body_sha256":"","runner_body_sha256":"",
        "preload_method":"","preload_endpoint":"","preload_request_sha256":"",
        "challenge_sha256":"","evidence":{"schema":"","id":"","sha256":"","bytes":0},
        "observation_sha256":""
    }'::jsonb;
BEGIN
    SELECT * INTO opening FROM station_call_openings WHERE id=NEW.opening_id FOR SHARE;
    SELECT status,worker_id INTO attempt_status,attempt_worker FROM job_step_attempts
    WHERE job_id=opening.job_id AND job_step_attempts.generation=opening.generation AND
          step_id=opening.step_id AND attempt=opening.step_attempt FOR SHARE;
    receipt_generation := NEW.generation_json::jsonb;
    authority_reason := receipt_generation->>'provider_request_failure_reason';
    SELECT COUNT(*) INTO identity_count FROM station_call_identity_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO response_count FROM station_call_response_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatched_identity
    FROM station_call_identity_captures AS captures
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(receipt_generation->'provider_identity_evidence'->'operations')
            WITH ORDINALITY AS values(item,ordinality)
        WHERE values.ordinality=captures.operation_index+1
    ) operations ON TRUE
    WHERE captures.opening_id=NEW.opening_id AND (
        operations.item IS NULL OR operations.item->>'operation'<>captures.operation OR
        operations.item->>'method'<>captures.method OR operations.item->>'endpoint'<>captures.endpoint OR
        operations.item->>'request_sha256'<>captures.request_sha256 OR
        (operations.item->>'request_bytes')::INTEGER<>captures.request_bytes OR
        operations.item->>'response_sha256'<>captures.response_sha256 OR
        (operations.item->>'response_bytes')::INTEGER<>captures.response_bytes
    );
    IF opening.id IS NULL OR attempt_worker IS DISTINCT FROM opening.worker_id OR
       identity_count<>5 OR mismatched_identity<>0 OR
       jsonb_array_length(receipt_generation->'provider_identity_evidence'->'operations')<>5 OR
       response_count IS DISTINCT FROM (CASE
           WHEN COALESCE(receipt_generation->>'provider_response_disposition','') IN ('','transport_error') THEN 0 ELSE 1 END) OR
       (response_count=1 AND NOT EXISTS (
           SELECT 1 FROM station_call_response_captures captures
           WHERE captures.opening_id=NEW.opening_id AND
             captures.capture_sha256=receipt_generation->>'provider_response_capture_sha256' AND
             captures.captured_bytes=(receipt_generation->>'provider_response_captured_bytes')::INTEGER
       )) OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,opening.worker_id,opening.gap_id) OR
       receipt_generation->>'protocol'<>opening.protocol OR
       (NEW.status='succeeded' AND (
           receipt_generation->>'provider_request_disposition'<>'dispatched' OR authority_reason IS NOT NULL OR
           receipt_generation->>'provider_request_sha256'<>opening.wire_request_sha256 OR
           receipt_generation->'provider_observation'->>'challenge_sha256'<>opening.observation_challenge OR
           receipt_generation->>'provider_response_disposition'<>'succeeded' OR response_count<>1 OR
           COALESCE(BTRIM(receipt_generation->>'content'),'')='' OR
           COALESCE((receipt_generation->>'provider_done_present')::BOOLEAN,FALSE)<>TRUE OR
           COALESCE((receipt_generation->>'provider_done')::BOOLEAN,FALSE)<>TRUE
       )) OR
       (NEW.status='failed' AND receipt_generation->>'provider_request_disposition'='not_dispatched' AND (
           response_count<>0 OR
           (attempt_status='active' AND (
               authority_reason IS NOT NULL OR NOT (
                   (COALESCE(receipt_generation->>'provider_response_disposition','')='' AND
                    COALESCE(receipt_generation->>'provider_request_sha256','')='' AND
                    receipt_generation->'provider_observation'=zero_observation) OR
                   (receipt_generation->>'provider_response_disposition'='transport_error' AND
                    receipt_generation->>'provider_request_sha256'=opening.wire_request_sha256 AND
                    receipt_generation->'provider_observation'->>'challenge_sha256'=opening.observation_challenge)
               )
           )) OR
           (attempt_status IN ('canceled','superseded','expired') AND (
               authority_reason IS DISTINCT FROM 'authority_'||attempt_status OR
               receipt_generation->>'provider_request_sha256'<>opening.wire_request_sha256 OR
               COALESCE(receipt_generation->>'provider_response_disposition','')<>'' OR
               receipt_generation->'provider_observation'<>zero_observation
           )) OR
           attempt_status NOT IN ('active','canceled','superseded','expired')
       )) OR
       (receipt_generation->>'provider_request_disposition'<>'not_dispatched' AND (
           receipt_generation->>'provider_request_disposition' NOT IN ('dispatched','write_indeterminate') OR
           authority_reason IS NOT NULL OR
           receipt_generation->>'provider_request_sha256'<>opening.wire_request_sha256 OR
           receipt_generation->'provider_observation'->>'challenge_sha256'<>opening.observation_challenge
       )) THEN
        RAISE EXCEPTION 'station call receipt does not match its exact opening, attempt, and dispatch authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_call_receipts_validate_insert BEFORE INSERT ON station_call_receipts
FOR EACH ROW EXECUTE FUNCTION validate_station_call_receipt_insert();

CREATE FUNCTION validate_station_llm_call_evidence_identity()
RETURNS TRIGGER AS $$
DECLARE
    opening station_call_openings%ROWTYPE;
BEGIN
    SELECT * INTO opening FROM station_call_openings
    WHERE id=NEW.station_call_opening_id FOR SHARE;
    IF opening.id IS NULL OR NEW.requested_model<>opening.model OR
       NEW.model<>opening.model OR NEW.attempt<>1 THEN
        RAISE EXCEPTION 'LLM call evidence differs from its exact opened model authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER llm_call_evidence_validate_station_identity
BEFORE INSERT ON llm_call_evidence
FOR EACH ROW EXECUTE FUNCTION validate_station_llm_call_evidence_identity();

DROP TRIGGER station_gap_outcomes_require_call_receipt ON station_gap_outcomes;
CREATE OR REPLACE FUNCTION require_station_call_receipt_before_gap_outcome()
RETURNS TRIGGER AS $$
DECLARE
    discovery_count INTEGER;
    discovery_status TEXT;
    call_count INTEGER;
    call_status TEXT;
    call_response TEXT;
    evidence_count INTEGER;
BEGIN
    SELECT COUNT(*),MIN(receipts.status) INTO discovery_count,discovery_status
    FROM station_provider_discoveries discoveries
    LEFT JOIN station_provider_discovery_receipts receipts ON receipts.opening_id=discoveries.id
    WHERE discoveries.gap_opening_id=NEW.opening_id;
    SELECT COUNT(*),MIN(receipts.status),MIN(receipts.generation_json::jsonb->>'content'),COUNT(evidence.id)
    INTO call_count,call_status,call_response,evidence_count
    FROM station_call_openings calls
    LEFT JOIN station_call_receipts receipts ON receipts.opening_id=calls.id
    LEFT JOIN llm_call_evidence evidence ON evidence.station_call_opening_id=calls.id
    WHERE calls.gap_opening_id=NEW.opening_id;
    IF discovery_count<>1 OR discovery_status IS NULL THEN
        RAISE EXCEPTION 'station gap outcome requires one terminal provider discovery receipt';
    END IF;
    IF call_count>0 AND (call_status IS NULL OR evidence_count<>call_count) THEN
        RAISE EXCEPTION 'station gap outcome requires one immutable evidence row for every terminal provider call';
    END IF;
    IF NEW.status='resolved' AND
       (discovery_status<>'succeeded' OR call_count<>1 OR call_status<>'succeeded' OR
        NEW.response IS DISTINCT FROM call_response) THEN
        RAISE EXCEPTION 'resolved station gap requires successful discovery and provider call receipts';
    END IF;
    IF NEW.status='failed' AND discovery_status='succeeded' AND
       (call_count<>1 OR call_status IS NULL) THEN
        RAISE EXCEPTION 'failed station gap requires its terminal provider call receipt';
    END IF;
    IF NEW.status='failed' AND discovery_status='failed' AND call_count<>0 THEN
        RAISE EXCEPTION 'failed provider discovery cannot have a provider call';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_gap_outcomes_require_call_receipt BEFORE INSERT ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION require_station_call_receipt_before_gap_outcome();

DO $$
BEGIN
    IF to_regprocedure('validate_station_llm_call_evidence_identity()') IS NULL THEN
        RAISE EXCEPTION 'terminal receipt authority postcondition failed';
    END IF;
END $$;
