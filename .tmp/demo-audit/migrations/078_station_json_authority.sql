LOCK TABLE station_provider_discovery_receipts, station_provider_discovery_captures,
    station_call_receipts, station_call_openings, station_call_identity_captures,
    station_call_response_captures, job_step_attempts IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    observed_source TEXT;
    observed_sha256 TEXT;
    expected RECORD;
BEGIN
    FOR expected IN SELECT * FROM (VALUES
        ('validate_station_provider_discovery_receipt_insert',
         '572bbdf8c7469738a13b6add6f857919f81ebdc3647bbba20a6b20ad2b0cc07e'),
        ('validate_station_call_receipt_insert',
         'b8dd0f77a20b826373b0862513bc367c92607acdf5cdb955dc601bd21effd9e5'),
        ('require_station_call_receipt_before_gap_outcome',
         '9f86c596542f7bc57fe624086429702120392e5228c95afcfb9e14ec86cd19f3'),
        ('validate_station_llm_call_evidence_identity',
         'b06ddd5874d1a811e914d87eee2e6661aeadc0a487cf9123b1a2b2821e9c2cc5')
    ) AS values(function_name,function_sha256) LOOP
        SELECT p.prosrc INTO observed_source FROM pg_proc AS p
        WHERE p.oid=to_regprocedure(expected.function_name||'()');
        observed_sha256 := encode(digest(convert_to(observed_source,'UTF8'),'sha256'),'hex');
        IF observed_sha256 IS DISTINCT FROM expected.function_sha256 THEN
            RAISE EXCEPTION 'cannot install station JSON authority: function % hash % is not frozen',
                expected.function_name,observed_sha256;
        END IF;
    END LOOP;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM station_provider_discovery_receipts receipts
        WHERE jsonb_typeof(receipts.observation::jsonb) IS DISTINCT FROM 'object' OR
              jsonb_typeof(receipts.observation::jsonb->'evidence') IS DISTINCT FROM 'object' OR
              jsonb_typeof(receipts.observation::jsonb->'evidence'->'operations') IS DISTINCT FROM 'array'
    ) OR EXISTS (
        SELECT 1 FROM station_call_receipts receipts
        WHERE jsonb_typeof(receipts.generation_json::jsonb) IS DISTINCT FROM 'object' OR
              jsonb_typeof(receipts.generation_json::jsonb->'provider_identity_evidence') IS DISTINCT FROM 'object' OR
              jsonb_typeof(receipts.generation_json::jsonb->'provider_identity_evidence'->'operations') IS DISTINCT FROM 'array' OR
              jsonb_typeof(receipts.generation_json::jsonb->'provider_observation') IS DISTINCT FROM 'object'
    ) THEN
        RAISE EXCEPTION 'cannot install station JSON authority while sparse terminal receipts exist';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION validate_station_provider_discovery_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_provider_discoveries%ROWTYPE;
    captures INTEGER;
    mismatches INTEGER;
    envelope JSONB := NEW.observation::jsonb;
    evidence JSONB;
    operations JSONB;
    reason TEXT;
BEGIN
    SELECT * INTO opening FROM station_provider_discoveries WHERE id=NEW.opening_id FOR SHARE;
    evidence := envelope->'evidence';
    operations := evidence->'operations';
    reason := envelope->>'failure_reason';
    IF opening.id IS NULL OR jsonb_typeof(envelope) IS DISTINCT FROM 'object' OR
       jsonb_typeof(evidence) IS DISTINCT FROM 'object' OR
       evidence->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence.v1' OR
       jsonb_typeof(evidence->'ref') IS DISTINCT FROM 'object' OR
       evidence->'ref'->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence-ref.v1' OR
       evidence->'ref'->>'sha256' IS NULL OR
       evidence->'ref'->>'sha256' !~ '^[0-9a-f]{64}$' OR
       evidence->'ref'->>'id' IS DISTINCT FROM
           'provider_identity_'||(evidence->'ref'->>'sha256') OR
       (CASE
           WHEN jsonb_typeof(evidence->'ref'->'bytes')='number' AND
                (evidence->'ref'->>'bytes')~'^[1-9][0-9]{0,7}$'
           THEN ((evidence->'ref'->>'bytes')::INTEGER BETWEEN 1 AND 29360135)
           ELSE FALSE
       END) IS DISTINCT FROM TRUE OR
       jsonb_typeof(operations) IS DISTINCT FROM 'array' OR jsonb_array_length(operations)<>5 THEN
        RAISE EXCEPTION 'station provider discovery receipt contains sparse JSON authority';
    END IF;
    SELECT COUNT(*) INTO captures FROM station_provider_discovery_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatches
    FROM station_provider_discovery_captures AS capture
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(operations) WITH ORDINALITY AS value(item,ordinality)
        WHERE value.ordinality=capture.operation_index+1
    ) operation ON TRUE
    WHERE capture.opening_id=NEW.opening_id AND (
        operation.item->>'operation' IS DISTINCT FROM capture.operation OR
        operation.item->>'method' IS DISTINCT FROM capture.method OR
        operation.item->>'endpoint' IS DISTINCT FROM capture.endpoint OR
        operation.item->>'request_disposition' IS DISTINCT FROM capture.request_disposition OR
        operation.item->>'request_sha256' IS DISTINCT FROM capture.request_sha256 OR
        operation.item->'request_bytes' IS DISTINCT FROM to_jsonb(capture.request_bytes) OR
        operation.item->'http_status' IS DISTINCT FROM to_jsonb(capture.http_status) OR
        operation.item->>'disposition' IS DISTINCT FROM capture.disposition OR
        operation.item->'response_complete' IS DISTINCT FROM to_jsonb(capture.response_complete) OR
        operation.item->'content_encoding' IS DISTINCT FROM capture.content_encoding::jsonb OR
        operation.item->>'response_sha256' IS DISTINCT FROM capture.response_sha256 OR
        operation.item->'response_bytes' IS DISTINCT FROM to_jsonb(capture.response_bytes)
    );
    IF captures<>5 OR mismatches<>0 OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM ROW(opening.job_id,opening.generation,opening.step_id,
                            opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.status='succeeded' AND (
           envelope ? 'failure_reason' OR envelope->>'schema' IS DISTINCT FROM
               'omnidex.provider-identity-observation.v2' OR
           envelope->>'challenge_sha256' IS DISTINCT FROM opening.challenge OR
           jsonb_typeof(envelope->'observed_at') IS DISTINCT FROM 'string'
       )) OR
       (NEW.status='failed' AND (
           NOT envelope ? 'failure_reason' OR
           reason IS NULL OR reason NOT IN
               ('evidence_rejected','observation_rejected','provider_contract_rejected') OR
           (reason='evidence_rejected' AND
               (envelope ? 'attestation' OR envelope ? 'observation')) OR
           (reason IN ('observation_rejected','provider_contract_rejected') AND
               (jsonb_typeof(envelope->'attestation') IS DISTINCT FROM 'object' OR
                jsonb_typeof(envelope->'observation') IS DISTINCT FROM 'object'))
       )) THEN
        RAISE EXCEPTION 'station provider discovery receipt differs from exact typed JSON authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_station_call_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_call_openings%ROWTYPE;
    attempt_status TEXT;
    attempt_worker TEXT;
    generation JSONB := NEW.generation_json::jsonb;
    evidence JSONB;
    operations JSONB;
    observation JSONB;
    identity_count INTEGER;
    response_count INTEGER;
    mismatches INTEGER;
    request_disposition TEXT;
    response_disposition TEXT;
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
    SELECT attempts.status,attempts.worker_id INTO attempt_status,attempt_worker
    FROM job_step_attempts AS attempts
    WHERE attempts.job_id=opening.job_id AND attempts.generation=opening.generation AND
          attempts.step_id=opening.step_id AND attempts.attempt=opening.step_attempt FOR SHARE;
    evidence := generation->'provider_identity_evidence';
    operations := evidence->'operations';
    observation := generation->'provider_observation';
    request_disposition := generation->>'provider_request_disposition';
    response_disposition := generation->>'provider_response_disposition';
    authority_reason := generation->>'provider_request_failure_reason';
    IF opening.id IS NULL OR jsonb_typeof(generation) IS DISTINCT FROM 'object' OR
       generation->>'schema' IS DISTINCT FROM 'omnidex.prepared-generation.v1' OR
       generation->>'protocol' IS DISTINCT FROM opening.protocol OR
       request_disposition IS NULL OR request_disposition NOT IN
           ('not_dispatched','dispatched','write_indeterminate') OR
       NOT generation ? 'provider_response_disposition' OR response_disposition IS NULL OR
       jsonb_typeof(observation) IS DISTINCT FROM 'object' OR
       jsonb_typeof(evidence) IS DISTINCT FROM 'object' OR
       evidence->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence.v1' OR
       jsonb_typeof(evidence->'ref') IS DISTINCT FROM 'object' OR
       evidence->'ref'->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-evidence-ref.v1' OR
       evidence->'ref'->>'sha256' IS NULL OR
       evidence->'ref'->>'sha256' !~ '^[0-9a-f]{64}$' OR
       evidence->'ref'->>'id' IS DISTINCT FROM
           'provider_identity_'||(evidence->'ref'->>'sha256') OR
       (CASE
           WHEN jsonb_typeof(evidence->'ref'->'bytes')='number' AND
                (evidence->'ref'->>'bytes')~'^[1-9][0-9]{0,7}$'
           THEN ((evidence->'ref'->>'bytes')::INTEGER BETWEEN 1 AND 29360135)
           ELSE FALSE
       END) IS DISTINCT FROM TRUE OR
       jsonb_typeof(operations) IS DISTINCT FROM 'array' OR jsonb_array_length(operations)<>5 THEN
        RAISE EXCEPTION 'station call receipt contains sparse JSON authority';
    END IF;
    SELECT COUNT(*) INTO identity_count FROM station_call_identity_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO response_count FROM station_call_response_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatches
    FROM station_call_identity_captures AS capture
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(operations) WITH ORDINALITY AS value(item,ordinality)
        WHERE value.ordinality=capture.operation_index+1
    ) operation ON TRUE
    WHERE capture.opening_id=NEW.opening_id AND (
        operation.item->>'operation' IS DISTINCT FROM capture.operation OR
        operation.item->>'method' IS DISTINCT FROM capture.method OR
        operation.item->>'endpoint' IS DISTINCT FROM capture.endpoint OR
        operation.item->>'request_sha256' IS DISTINCT FROM capture.request_sha256 OR
        operation.item->'request_bytes' IS DISTINCT FROM to_jsonb(capture.request_bytes) OR
        operation.item->>'response_sha256' IS DISTINCT FROM capture.response_sha256 OR
        operation.item->'response_bytes' IS DISTINCT FROM to_jsonb(capture.response_bytes)
    );
    IF attempt_worker IS DISTINCT FROM opening.worker_id OR identity_count<>5 OR mismatches<>0 OR
       response_count IS DISTINCT FROM (CASE WHEN response_disposition IN ('','transport_error') THEN 0 ELSE 1 END) OR
       (response_count=1 AND NOT EXISTS (
           SELECT 1 FROM station_call_response_captures capture WHERE capture.opening_id=NEW.opening_id AND
             capture.capture_sha256 IS NOT DISTINCT FROM generation->>'provider_response_capture_sha256' AND
             to_jsonb(capture.captured_bytes) IS NOT DISTINCT FROM
                 generation->'provider_response_captured_bytes'
       )) OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM ROW(opening.job_id,opening.generation,opening.step_id,
                            opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.status='succeeded' AND (
           request_disposition IS DISTINCT FROM 'dispatched' OR
           generation ? 'provider_request_failure_reason' OR
           generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
           observation->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-observation.v2' OR
           observation->>'challenge_sha256' IS DISTINCT FROM opening.observation_challenge OR
           response_disposition IS DISTINCT FROM 'succeeded' OR response_count<>1 OR
           COALESCE(BTRIM(generation->>'content'),'')='' OR
           generation->'provider_done_present' IS DISTINCT FROM 'true'::jsonb OR
           generation->'provider_done' IS DISTINCT FROM 'true'::jsonb
       )) OR
       (NEW.status='failed' AND request_disposition='not_dispatched' AND (
           response_count<>0 OR
           (attempt_status='active' AND (
               generation ? 'provider_request_failure_reason' OR NOT (
                   (response_disposition='' AND
                    generation->>'provider_request_sha256' IS NOT DISTINCT FROM '' AND
                    observation IS NOT DISTINCT FROM zero_observation) OR
                   (response_disposition='transport_error' AND
                    generation->>'provider_request_sha256' IS NOT DISTINCT FROM opening.wire_request_sha256 AND
                    observation->>'schema' IS NOT DISTINCT FROM 'omnidex.provider-identity-observation.v2' AND
                    observation->>'challenge_sha256' IS NOT DISTINCT FROM opening.observation_challenge)
               )
           )) OR
           (attempt_status IN ('canceled','superseded','expired') AND (
               authority_reason IS DISTINCT FROM 'authority_'||attempt_status OR
               generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
               response_disposition IS DISTINCT FROM '' OR observation IS DISTINCT FROM zero_observation
           )) OR attempt_status NOT IN ('active','canceled','superseded','expired')
       )) OR
       (request_disposition IN ('dispatched','write_indeterminate') AND (
           generation ? 'provider_request_failure_reason' OR
           generation->>'provider_request_sha256' IS DISTINCT FROM opening.wire_request_sha256 OR
           observation->>'schema' IS DISTINCT FROM 'omnidex.provider-identity-observation.v2' OR
           observation->>'challenge_sha256' IS DISTINCT FROM opening.observation_challenge OR
           response_disposition IS NULL OR response_disposition NOT IN
               ('succeeded','transport_error','http_error','body_limit','body_read_error','invalid_json','empty_content')
       )) THEN
        RAISE EXCEPTION 'station call receipt differs from exact typed JSON authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
