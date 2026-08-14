LOCK TABLE station_gap_openings, station_gap_outcomes,
    station_provider_discoveries, station_provider_discovery_receipts,
    job_steps, job_step_attempts, llm_call_evidence IN ACCESS EXCLUSIVE MODE;

DROP TRIGGER station_gap_outcomes_require_discovery_receipt ON station_gap_outcomes;
DROP FUNCTION require_terminal_discovery_before_gap_outcome();

CREATE TABLE station_call_openings (
    id BIGSERIAL PRIMARY KEY,
    gap_opening_id BIGINT NOT NULL REFERENCES station_gap_openings(id) ON DELETE RESTRICT,
    discovery_receipt_id BIGINT NOT NULL REFERENCES station_provider_discovery_receipts(id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    protocol TEXT NOT NULL CHECK (protocol IN (
        'omnidex.ollama-raw-generate-request.v1',
        'omnidex.ollama-raw-text-generate-request.v1'
    )),
    tokenizer_profile TEXT NOT NULL CHECK (tokenizer_profile='ollama-0.24.0-qwen35-gpt2-boundary-v1'),
    provider_method TEXT NOT NULL CHECK (provider_method='POST'),
    provider_endpoint TEXT NOT NULL CHECK (provider_endpoint='/api/generate'),
    wire_request BYTEA NOT NULL CHECK (octet_length(wire_request) BETWEEN 1 AND 131072),
    wire_request_sha256 TEXT NOT NULL CHECK (
        wire_request_sha256~'^[0-9a-f]{64}$' AND wire_request_sha256=encode(digest(wire_request,'sha256'),'hex')
    ),
    wire_request_bytes INTEGER NOT NULL CHECK (wire_request_bytes BETWEEN 1 AND 131072 AND wire_request_bytes=octet_length(wire_request)),
    expectation TEXT NOT NULL CHECK (octet_length(expectation)<=8192),
    expectation_sha256 TEXT NOT NULL CHECK (
        expectation_sha256~'^[0-9a-f]{64}$' AND expectation_sha256=encode(digest(expectation,'sha256'),'hex')
    ),
    observation_challenge TEXT NOT NULL CHECK (observation_challenge~'^[0-9a-f]{64}$'),
    model TEXT NOT NULL CHECK (model<>'' AND model=BTRIM(model) AND octet_length(model)<=512),
    context_tokens INTEGER NOT NULL CHECK (context_tokens BETWEEN 1 AND 262144),
    max_input_tokens INTEGER NOT NULL CHECK (max_input_tokens>0),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens BETWEEN 1 AND 16384),
    model_input TEXT NOT NULL CHECK (octet_length(model_input) BETWEEN 1 AND 131072),
    model_input_sha256 TEXT NOT NULL CHECK (
        model_input_sha256~'^[0-9a-f]{64}$' AND model_input_sha256=encode(digest(model_input,'sha256'),'hex')
    ),
    model_input_bytes INTEGER NOT NULL CHECK (model_input_bytes=octet_length(model_input)),
    model_input_token_upper_bound INTEGER NOT NULL CHECK (
        model_input_token_upper_bound>0 AND model_input_token_upper_bound<=max_input_tokens AND
        model_input_token_upper_bound+max_output_tokens<=context_tokens
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK (max_input_tokens=context_tokens-max_output_tokens),
    CHECK ((expectation::jsonb)->>'backend'='ollama'),
    CHECK ((expectation::jsonb)->>'backend_version'='0.24.0'),
    CHECK ((expectation::jsonb)->>'model'=model),
    CHECK ((expectation::jsonb)->>'tokenizer_profile'=tokenizer_profile),
    CHECK (((expectation::jsonb)->>'native_context_limit')::INTEGER=context_tokens)
);

CREATE UNIQUE INDEX station_call_openings_one_gap ON station_call_openings(gap_opening_id);
CREATE INDEX station_call_openings_attempt
    ON station_call_openings(job_id,generation,step_id,step_attempt,id);

CREATE FUNCTION validate_station_call_opening_insert()
RETURNS TRIGGER AS $$
DECLARE
    gap station_gap_openings%ROWTYPE;
    discovery station_provider_discovery_receipts%ROWTYPE;
BEGIN
    SELECT * INTO gap FROM station_gap_openings WHERE id=NEW.gap_opening_id FOR SHARE;
    SELECT * INTO discovery FROM station_provider_discovery_receipts WHERE id=NEW.discovery_receipt_id FOR SHARE;
    IF gap.id IS NULL OR discovery.id IS NULL OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id,
           NEW.context_tokens,NEW.max_output_tokens)
       IS DISTINCT FROM
       ROW(gap.job_id,gap.generation,gap.step_id,gap.step_attempt,gap.worker_id,gap.gap_id,
           gap.context_tokens,gap.max_output_tokens) OR
       discovery.status<>'succeeded' OR discovery.gap_id<>gap.gap_id OR
       discovery.job_id<>gap.job_id OR discovery.generation<>gap.generation OR
       discovery.step_id<>gap.step_id OR discovery.step_attempt<>gap.step_attempt OR
       discovery.worker_id<>gap.worker_id OR discovery.expectation::jsonb<>NEW.expectation::jsonb THEN
        RAISE EXCEPTION 'station call opening does not match its exact gap authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_call_openings_validate_insert BEFORE INSERT ON station_call_openings
FOR EACH ROW EXECUTE FUNCTION validate_station_call_opening_insert();

CREATE TABLE station_call_response_captures (
    opening_id BIGINT PRIMARY KEY REFERENCES station_call_openings(id) ON DELETE RESTRICT,
    capture BYTEA NOT NULL CHECK (octet_length(capture)<=16777217),
    capture_sha256 TEXT NOT NULL CHECK (
        capture_sha256~'^[0-9a-f]{64}$' AND capture_sha256=encode(digest(capture,'sha256'),'hex')
    ),
    captured_bytes INTEGER NOT NULL CHECK (captured_bytes=octet_length(capture) AND captured_bytes<=16777217),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE station_call_identity_captures (
    opening_id BIGINT NOT NULL REFERENCES station_call_openings(id) ON DELETE RESTRICT,
    operation_index INTEGER NOT NULL CHECK (operation_index BETWEEN 0 AND 4),
    operation TEXT NOT NULL CHECK (operation IN ('version','installed','tokenizer','preload','runner')),
    method TEXT NOT NULL CHECK (method IN ('GET','POST')),
    endpoint TEXT NOT NULL CHECK (endpoint IN ('/api/version','/api/tags','/api/show','/api/generate','/api/ps')),
    request_capture BYTEA NOT NULL CHECK (octet_length(request_capture)<=4194304),
    request_sha256 TEXT NOT NULL CHECK (request_sha256~'^[0-9a-f]{64}$' AND request_sha256=encode(digest(request_capture,'sha256'),'hex')),
    request_bytes INTEGER NOT NULL CHECK (request_bytes=octet_length(request_capture)),
    response_capture BYTEA NOT NULL CHECK (octet_length(response_capture)<=4194305),
    response_sha256 TEXT NOT NULL CHECK (response_sha256~'^[0-9a-f]{64}$' AND response_sha256=encode(digest(response_capture,'sha256'),'hex')),
    response_bytes INTEGER NOT NULL CHECK (response_bytes=octet_length(response_capture)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (opening_id,operation_index),
    UNIQUE (opening_id,operation),
    CHECK (
        (operation_index=0 AND operation='version' AND method='GET' AND endpoint='/api/version') OR
        (operation_index=1 AND operation='installed' AND method='GET' AND endpoint='/api/tags') OR
        (operation_index=2 AND operation='tokenizer' AND method='POST' AND endpoint='/api/show') OR
        (operation_index=3 AND operation='preload' AND method='POST' AND endpoint='/api/generate') OR
        (operation_index=4 AND operation='runner' AND method='GET' AND endpoint='/api/ps')
    )
);

CREATE TABLE station_call_receipts (
    id BIGSERIAL PRIMARY KEY,
    opening_id BIGINT NOT NULL REFERENCES station_call_openings(id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('succeeded','failed')),
    generation_json TEXT NOT NULL CHECK (octet_length(generation_json)<=131072),
    generation_sha256 TEXT NOT NULL CHECK (
        generation_sha256~'^[0-9a-f]{64}$' AND generation_sha256=encode(digest(generation_json,'sha256'),'hex')
    ),
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK (
        (status='succeeded' AND error IS NULL) OR
        (status='failed' AND error IS NOT NULL AND error=BTRIM(error) AND error<>'' AND octet_length(error)<=8192)
    ),
    CHECK ((generation_json::jsonb)->>'schema'='omnidex.prepared-generation.v1')
);
CREATE UNIQUE INDEX station_call_receipts_one_terminal ON station_call_receipts(opening_id);

CREATE FUNCTION validate_station_call_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_call_openings%ROWTYPE;
    identity_count INTEGER;
    response_count INTEGER;
    mismatched_identity INTEGER;
    generation JSONB;
BEGIN
    SELECT * INTO opening FROM station_call_openings WHERE id=NEW.opening_id FOR SHARE;
    generation := NEW.generation_json::jsonb;
    SELECT COUNT(*) INTO identity_count FROM station_call_identity_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO response_count FROM station_call_response_captures WHERE opening_id=NEW.opening_id;
    SELECT COUNT(*) INTO mismatched_identity
    FROM station_call_identity_captures AS captures
    LEFT JOIN LATERAL (
        SELECT item FROM jsonb_array_elements(generation->'provider_identity_evidence'->'operations')
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
    IF opening.id IS NULL OR identity_count<>5 OR mismatched_identity<>0 OR
       jsonb_array_length(generation->'provider_identity_evidence'->'operations')<>5 OR
       response_count IS DISTINCT FROM (CASE
           WHEN generation->>'provider_response_disposition' IN ('','transport_error') THEN 0 ELSE 1 END) OR
       (response_count=1 AND NOT EXISTS (
           SELECT 1 FROM station_call_response_captures captures
           WHERE captures.opening_id=NEW.opening_id AND
             captures.capture_sha256=generation->>'provider_response_capture_sha256' AND
             captures.captured_bytes=(generation->>'provider_response_captured_bytes')::INTEGER
       )) OR
       (NEW.status='succeeded' AND (
           generation->>'provider_response_disposition'<>'succeeded' OR
           COALESCE(BTRIM(generation->>'content'),'')='' OR
           COALESCE((generation->>'provider_done_present')::BOOLEAN,FALSE)<>TRUE OR
           COALESCE((generation->>'provider_done')::BOOLEAN,FALSE)<>TRUE
       )) OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,opening.worker_id,opening.gap_id) OR
       (NEW.generation_json::jsonb)->>'protocol'<>opening.protocol OR
       ((NEW.generation_json::jsonb)->>'provider_request_disposition'<>'not_dispatched' AND
        ((NEW.generation_json::jsonb)->>'provider_request_sha256'<>opening.wire_request_sha256 OR
         (NEW.generation_json::jsonb)->'provider_observation'->>'challenge_sha256'<>opening.observation_challenge)) THEN
        RAISE EXCEPTION 'station call receipt does not match its exact opening and captures';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_call_receipts_validate_insert BEFORE INSERT ON station_call_receipts
FOR EACH ROW EXECUTE FUNCTION validate_station_call_receipt_insert();

CREATE FUNCTION require_station_call_receipt_before_gap_outcome()
RETURNS TRIGGER AS $$
DECLARE
    discovery_count INTEGER;
    discovery_status TEXT;
    call_count INTEGER;
    call_status TEXT;
    call_response TEXT;
BEGIN
    SELECT COUNT(*),MIN(receipts.status) INTO discovery_count,discovery_status
    FROM station_provider_discoveries AS discoveries
    LEFT JOIN station_provider_discovery_receipts AS receipts ON receipts.opening_id=discoveries.id
    WHERE discoveries.gap_opening_id=NEW.opening_id;
    SELECT COUNT(*),MIN(receipts.status),MIN(receipts.generation_json::jsonb->>'content')
    INTO call_count,call_status,call_response
    FROM station_call_openings AS calls
    LEFT JOIN station_call_receipts AS receipts ON receipts.opening_id=calls.id
    WHERE calls.gap_opening_id=NEW.opening_id;
    IF discovery_count<>1 OR discovery_status IS NULL THEN
        RAISE EXCEPTION 'station gap outcome requires one terminal provider discovery receipt';
    END IF;
    IF call_count>0 AND call_status IS NULL THEN
        RAISE EXCEPTION 'station gap outcome requires one terminal provider call receipt';
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

CREATE TRIGGER station_call_openings_immutable BEFORE UPDATE OR DELETE ON station_call_openings
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_openings_truncate_immutable BEFORE TRUNCATE ON station_call_openings
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_response_captures_immutable BEFORE UPDATE OR DELETE ON station_call_response_captures
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_response_captures_truncate_immutable BEFORE TRUNCATE ON station_call_response_captures
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_identity_captures_immutable BEFORE UPDATE OR DELETE ON station_call_identity_captures
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_identity_captures_truncate_immutable BEFORE TRUNCATE ON station_call_identity_captures
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_receipts_immutable BEFORE UPDATE OR DELETE ON station_call_receipts
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_call_receipts_truncate_immutable BEFORE TRUNCATE ON station_call_receipts
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();

ALTER TABLE llm_call_evidence
    ADD COLUMN station_call_opening_id BIGINT REFERENCES station_call_openings(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX llm_call_evidence_one_station_gap ON llm_call_evidence(station_call_opening_id)
WHERE station_call_opening_id IS NOT NULL;

CREATE FUNCTION require_llm_call_station_gap()
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
       (opening.response_schema='null' AND NEW.response_schema IS NOT NULL) OR
       (opening.response_schema<>'null' AND NEW.response_schema IS DISTINCT FROM opening.response_schema::jsonb) OR
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
CREATE TRIGGER llm_call_evidence_require_station_gap BEFORE INSERT ON llm_call_evidence
FOR EACH ROW EXECUTE FUNCTION require_llm_call_station_gap();
