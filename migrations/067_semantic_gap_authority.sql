LOCK TABLE job_steps, job_step_attempts, llm_call_evidence IN ACCESS EXCLUSIVE MODE;

CREATE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
RETURNS BOOLEAN AS $$
    SELECT CASE work_kind
        WHEN 'application_classification' THEN station='coding_surface'
        WHEN 'application_identity' THEN station='coding_product_identity'
        WHEN 'requirement_partition' THEN station='coding_requirement_partition'
        WHEN 'repository_search_term' THEN station='coding_repository_search_term'
        WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'
        WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'
        WHEN 'conversation_response' THEN station='conversation_response'
        WHEN 'grounded_answer' THEN station='grounded_answer'
        WHEN 'web_search_terms' THEN station='web_search_terms'
        WHEN 'web_relevance' THEN station='web_relevance'
        WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'
        WHEN 'artifact_handling' THEN station='coding_artifact_handling'
        WHEN 'capability_relation' THEN station='coding_capability_relation'
        WHEN 'skill_selection' THEN station='coding_skill_selection'
        WHEN 'skill_procedure' THEN station='coding_skill_procedure'
        WHEN 'fragment_generation' THEN station='coding_fragment'
        WHEN 'fragment_modification' THEN station='coding_fragment'
        WHEN 'fragment_correction' THEN station='coding_fragment_correction'
        WHEN 'response_correction' THEN station_owns_portable_work(
            station,payload->'original'->>'kind',payload->'original'->'payload'
        )
        ELSE FALSE
    END;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE TABLE station_gap_openings (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    station TEXT NOT NULL CHECK (station<>'' AND station=BTRIM(station) AND octet_length(station)<=128),
    scope TEXT NOT NULL CHECK (scope IN ('portable_semantic_worker','portable_fragment_worker')),
    portable_schema TEXT NOT NULL CHECK (portable_schema='omnidex.portable-job.v1'),
    work_id TEXT NOT NULL CHECK (work_id~'^[0-9a-f]{64}$' AND work_id=gap_id),
    work_kind TEXT NOT NULL CHECK (work_kind<>'' AND work_kind=BTRIM(work_kind) AND octet_length(work_kind)<=128),
    portable_payload TEXT NOT NULL CHECK (portable_payload<>'' AND octet_length(portable_payload)<=16384),
    portable_payload_sha256 TEXT NOT NULL CHECK (
        portable_payload_sha256~'^[0-9a-f]{64}$' AND
        portable_payload_sha256=encode(digest(portable_payload,'sha256'),'hex')
    ),
    portable_envelope TEXT NOT NULL CHECK (octet_length(portable_envelope)<=32768),
    portable_envelope_sha256 TEXT NOT NULL CHECK (
        portable_envelope_sha256~'^[0-9a-f]{64}$' AND
        portable_envelope_sha256=encode(digest(portable_envelope,'sha256'),'hex')
    ),
    renderer_version TEXT NOT NULL CHECK (renderer_version='omnidex.render-portable-job.v1'),
    prompt TEXT NOT NULL CHECK (prompt<>'' AND BTRIM(prompt)<>'' AND octet_length(prompt)<=65536),
    response_schema TEXT NOT NULL CHECK (octet_length(response_schema)<=32768),
    projection_envelope TEXT NOT NULL CHECK (octet_length(projection_envelope)<=131072),
    projection_sha256 TEXT NOT NULL CHECK (
        projection_sha256~'^[0-9a-f]{64}$' AND
        projection_sha256=encode(digest(projection_envelope,'sha256'),'hex')
    ),
    context_tokens INTEGER NOT NULL CHECK (context_tokens BETWEEN 1 AND 262144),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens BETWEEN 1 AND 16384),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK ((portable_payload::jsonb) IS NOT NULL),
    CHECK (station_owns_portable_work(station,work_kind,portable_payload::jsonb)),
    CHECK (work_id=encode(digest(
        convert_to(portable_schema,'UTF8')||decode('00','hex')||
        convert_to(work_kind,'UTF8')||decode('00','hex')||convert_to(portable_payload,'UTF8'),
        'sha256'
    ),'hex')),
    CHECK (
        (portable_envelope::jsonb)->>'schema'=portable_schema AND
        (portable_envelope::jsonb)->>'id'=work_id AND
        (portable_envelope::jsonb)->>'kind'=work_kind AND
        (portable_envelope::jsonb)->'payload'=portable_payload::jsonb
    ),
    CHECK (
        (projection_envelope::jsonb)->>'renderer'=renderer_version AND
        (projection_envelope::jsonb)->>'prompt'=prompt AND
        (projection_envelope::jsonb)->'response_schema'=response_schema::jsonb
    )
);

CREATE UNIQUE INDEX station_gap_openings_one_identity
    ON station_gap_openings(job_id,generation,step_id,step_attempt,gap_id);
CREATE INDEX station_gap_openings_attempt
    ON station_gap_openings(job_id,generation,step_id,step_attempt,id);

CREATE TABLE station_gap_outcomes (
    id BIGSERIAL PRIMARY KEY,
    opening_id BIGINT NOT NULL REFERENCES station_gap_openings(id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('resolved','failed')),
    response TEXT,
    response_sha256 TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK (
        (status='resolved' AND response IS NOT NULL AND BTRIM(response)<>'' AND
            octet_length(response)<=65536 AND response_sha256~'^[0-9a-f]{64}$' AND
            response_sha256=encode(digest(response,'sha256'),'hex') AND error IS NULL) OR
        (status='failed' AND response IS NULL AND response_sha256 IS NULL AND
            error IS NOT NULL AND BTRIM(error)<>'' AND octet_length(error)<=8192)
    )
);

CREATE UNIQUE INDEX station_gap_outcomes_one_terminal ON station_gap_outcomes(opening_id);

CREATE FUNCTION validate_station_gap_outcome_insert()
RETURNS TRIGGER AS $$
DECLARE opening station_gap_openings%ROWTYPE;
BEGIN
    SELECT * INTO opening FROM station_gap_openings WHERE id=NEW.opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(opening.job_id,opening.generation,opening.step_id,opening.step_attempt,opening.worker_id,opening.gap_id) THEN
        RAISE EXCEPTION 'station gap outcome does not match opening authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER station_gap_outcomes_validate_insert
BEFORE INSERT ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION validate_station_gap_outcome_insert();

CREATE FUNCTION prevent_station_gap_history_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'station gap history is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER station_gap_openings_immutable BEFORE UPDATE OR DELETE ON station_gap_openings
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_gap_openings_truncate_immutable BEFORE TRUNCATE ON station_gap_openings
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_gap_outcomes_immutable BEFORE UPDATE OR DELETE ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_gap_outcomes_truncate_immutable BEFORE TRUNCATE ON station_gap_outcomes
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();

CREATE TABLE station_provider_discoveries (
    id BIGSERIAL PRIMARY KEY,
    gap_opening_id BIGINT NOT NULL REFERENCES station_gap_openings(id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    selection TEXT NOT NULL CHECK (octet_length(selection)<=1024),
    selection_sha256 TEXT NOT NULL CHECK (
        selection_sha256~'^[0-9a-f]{64}$' AND selection_sha256=encode(digest(selection,'sha256'),'hex')
    ),
    challenge TEXT NOT NULL CHECK (challenge~'^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK (((selection::jsonb)->>'native_context_limit')::INTEGER>0)
);
CREATE UNIQUE INDEX station_provider_discoveries_one_gap ON station_provider_discoveries(gap_opening_id);

CREATE FUNCTION validate_station_provider_discovery_insert()
RETURNS TRIGGER AS $$
DECLARE gap station_gap_openings%ROWTYPE;
BEGIN
    SELECT * INTO gap FROM station_gap_openings WHERE id=NEW.gap_opening_id FOR SHARE;
    IF NOT FOUND OR
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.gap_id)
       IS DISTINCT FROM
       ROW(gap.job_id,gap.generation,gap.step_id,gap.step_attempt,gap.worker_id,gap.gap_id) OR
       ((NEW.selection::jsonb)->>'native_context_limit')::INTEGER<>gap.context_tokens THEN
        RAISE EXCEPTION 'station provider discovery does not match its exact gap authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_provider_discoveries_validate_insert BEFORE INSERT ON station_provider_discoveries
FOR EACH ROW EXECUTE FUNCTION validate_station_provider_discovery_insert();

CREATE TABLE station_provider_discovery_captures (
    opening_id BIGINT NOT NULL REFERENCES station_provider_discoveries(id) ON DELETE RESTRICT,
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
    request_disposition TEXT NOT NULL CHECK (request_disposition IN ('not_dispatched','dispatched','write_indeterminate')),
    http_status INTEGER NOT NULL CHECK (http_status BETWEEN 0 AND 599),
    disposition TEXT NOT NULL CHECK (disposition IN ('not_dispatched','succeeded','transport_error','http_error','body_limit','body_read_error','invalid_json')),
    response_complete BOOLEAN NOT NULL,
    content_encoding TEXT NOT NULL CHECK (octet_length(content_encoding)<=131072),
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

CREATE TABLE station_provider_discovery_receipts (
    id BIGSERIAL PRIMARY KEY,
    opening_id BIGINT NOT NULL REFERENCES station_provider_discoveries(id) ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
    gap_id TEXT NOT NULL CHECK (gap_id~'^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('succeeded','failed')),
    observation TEXT NOT NULL CHECK (octet_length(observation)<=32768),
    observation_sha256 TEXT NOT NULL CHECK (
        observation_sha256~'^[0-9a-f]{64}$' AND observation_sha256=encode(digest(observation,'sha256'),'hex')
    ),
    expectation TEXT,
    expectation_sha256 TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    CHECK (
        (status='succeeded' AND expectation IS NOT NULL AND
            expectation_sha256~'^[0-9a-f]{64}$' AND
            expectation_sha256=encode(digest(expectation,'sha256'),'hex') AND error IS NULL) OR
        (status='failed' AND expectation IS NULL AND expectation_sha256 IS NULL AND
            error IS NOT NULL AND error=BTRIM(error) AND error<>'' AND octet_length(error)<=8192)
    )
);
CREATE UNIQUE INDEX station_provider_discovery_receipts_one_terminal
    ON station_provider_discovery_receipts(opening_id);

CREATE FUNCTION validate_station_provider_discovery_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    opening station_provider_discoveries%ROWTYPE;
    capture_count INTEGER;
    mismatched_captures INTEGER;
    evidence JSONB;
BEGIN
    SELECT * INTO opening FROM station_provider_discoveries WHERE id=NEW.opening_id FOR SHARE;
    SELECT COUNT(*) INTO capture_count FROM station_provider_discovery_captures WHERE opening_id=NEW.opening_id;
    evidence := NEW.observation::jsonb->'evidence';
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
       (NEW.status='succeeded' AND (NEW.observation::jsonb)->>'challenge_sha256'<>opening.challenge) THEN
        RAISE EXCEPTION 'station provider discovery receipt differs from its opening and captures';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_provider_discovery_receipts_validate_insert
BEFORE INSERT ON station_provider_discovery_receipts
FOR EACH ROW EXECUTE FUNCTION validate_station_provider_discovery_receipt_insert();

CREATE FUNCTION require_terminal_discovery_before_gap_outcome()
RETURNS TRIGGER AS $$
DECLARE
    discovery_count INTEGER;
    discovery_status TEXT;
BEGIN
    SELECT COUNT(*),MIN(receipts.status) INTO discovery_count,discovery_status
    FROM station_provider_discoveries AS discoveries
    LEFT JOIN station_provider_discovery_receipts AS receipts ON receipts.opening_id=discoveries.id
    WHERE discoveries.gap_opening_id=NEW.opening_id;
    IF discovery_count<>1 OR discovery_status IS NULL THEN
        RAISE EXCEPTION 'station gap outcome requires one terminal provider discovery receipt';
    END IF;
    IF NEW.status='resolved' OR discovery_status<>'failed' THEN
        RAISE EXCEPTION 'station gap outcome requires exact provider call authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER station_gap_outcomes_require_discovery_receipt
BEFORE INSERT ON station_gap_outcomes
FOR EACH ROW EXECUTE FUNCTION require_terminal_discovery_before_gap_outcome();

CREATE TRIGGER station_provider_discoveries_immutable BEFORE UPDATE OR DELETE ON station_provider_discoveries
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_provider_discoveries_truncate_immutable BEFORE TRUNCATE ON station_provider_discoveries
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_provider_discovery_captures_immutable BEFORE UPDATE OR DELETE ON station_provider_discovery_captures
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_provider_discovery_captures_truncate_immutable BEFORE TRUNCATE ON station_provider_discovery_captures
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_provider_discovery_receipts_immutable BEFORE UPDATE OR DELETE ON station_provider_discovery_receipts
FOR EACH ROW EXECUTE FUNCTION prevent_station_gap_history_mutation();
CREATE TRIGGER station_provider_discovery_receipts_truncate_immutable BEFORE TRUNCATE ON station_provider_discovery_receipts
FOR EACH STATEMENT EXECUTE FUNCTION prevent_station_gap_history_mutation();
