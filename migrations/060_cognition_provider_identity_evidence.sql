CREATE TABLE cognition_provider_identity_evidence (
    evidence_id TEXT PRIMARY KEY CHECK (
        evidence_id~'^provider_identity_[0-9a-f]{64}$'
    ),
    manifest_sha256 TEXT NOT NULL CHECK (manifest_sha256~'^[0-9a-f]{64}$'),
    total_bytes BIGINT NOT NULL CHECK (
        total_bytes>0 AND total_bytes<=29360135
    ),
    ref_json TEXT NOT NULL CHECK (
        jsonb_typeof(ref_json::jsonb)='object' AND octet_length(ref_json)<=1024
    ),
    ref_sha256 TEXT NOT NULL CHECK (
        ref_sha256~'^[0-9a-f]{64}$' AND
        ref_sha256=encode(digest(ref_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cognition_provider_identity_evidence_operations (
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    operation_index SMALLINT NOT NULL CHECK (operation_index BETWEEN 0 AND 4),
    operation TEXT NOT NULL,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    request_disposition TEXT NOT NULL CHECK (
        request_disposition IN ('not_dispatched','dispatched','write_indeterminate')
    ),
    request_sha256 TEXT NOT NULL CHECK (request_sha256~'^[0-9a-f]{64}$'),
    request_bytes BIGINT NOT NULL CHECK (
        request_bytes>=0 AND request_bytes<=4194304
    ),
    request_body BYTEA NOT NULL CHECK (
        octet_length(request_body)=request_bytes AND
        encode(digest(request_body,'sha256'),'hex')=request_sha256
    ),
    http_status INTEGER NOT NULL CHECK (http_status BETWEEN 0 AND 599),
    disposition TEXT NOT NULL CHECK (disposition IN (
        'not_dispatched','succeeded','transport_error','http_error',
        'body_limit','body_read_error','invalid_json'
    )),
    response_complete BOOLEAN NOT NULL,
    content_encoding_json TEXT NOT NULL CHECK (
        jsonb_typeof(content_encoding_json::jsonb)='object' AND
        octet_length(content_encoding_json)<=196608
    ),
    response_sha256 TEXT NOT NULL CHECK (response_sha256~'^[0-9a-f]{64}$'),
    response_bytes BIGINT NOT NULL CHECK (
        response_bytes>=0 AND response_bytes<=4194305
    ),
    response_body BYTEA NOT NULL CHECK (
        octet_length(response_body)=response_bytes AND
        encode(digest(response_body,'sha256'),'hex')=response_sha256
    ),
    PRIMARY KEY (evidence_id,operation_index),
    CHECK (
        (operation_index=0 AND operation='version' AND method='GET' AND endpoint='/api/version') OR
        (operation_index=1 AND operation='installed' AND method='GET' AND endpoint='/api/tags') OR
        (operation_index=2 AND operation='tokenizer' AND method='POST' AND endpoint='/api/show') OR
        (operation_index=3 AND operation='preload' AND method='POST' AND endpoint='/api/generate') OR
        (operation_index=4 AND operation='runner' AND method='GET' AND endpoint='/api/ps')
    ),
    CHECK (
        (disposition='not_dispatched' AND request_disposition='not_dispatched' AND http_status=0 AND
         NOT response_complete AND response_bytes=0 AND
         content_encoding_json='{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}') OR
        (disposition='transport_error' AND
         request_disposition IN ('not_dispatched','dispatched','write_indeterminate') AND http_status=0 AND
         NOT response_complete AND response_bytes=0 AND
         content_encoding_json='{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}') OR
        (disposition='body_limit' AND request_disposition='dispatched' AND http_status BETWEEN 100 AND 599 AND
         NOT response_complete AND response_bytes=4194305) OR
        (disposition='body_read_error' AND request_disposition='dispatched' AND
         http_status BETWEEN 100 AND 599 AND NOT response_complete) OR
        (disposition='succeeded' AND request_disposition='dispatched' AND
         http_status BETWEEN 200 AND 299 AND response_complete AND response_bytes<=4194304 AND
         (content_encoding_json::jsonb->>'uncompressed')::BOOLEAN IS FALSE AND
         (content_encoding_json::jsonb->>'complete')::BOOLEAN IS TRUE) OR
        (disposition='http_error' AND request_disposition='dispatched' AND
         http_status BETWEEN 100 AND 599 AND NOT (http_status BETWEEN 200 AND 299) AND
         response_complete AND response_bytes<=4194304) OR
        (disposition='invalid_json' AND request_disposition='dispatched' AND
         http_status BETWEEN 200 AND 299 AND response_complete AND response_bytes<=4194304)
    )
);

CREATE TABLE cognition_policy_call_provider_identity_evidence (
    call_id TEXT PRIMARY KEY,
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES cognition_policy_calls(
            call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
        ) ON DELETE RESTRICT
);

CREATE INDEX cognition_policy_call_identity_by_evidence
ON cognition_policy_call_provider_identity_evidence(evidence_id,call_id);

CREATE TABLE cognition_episode_provider_identity_evidence (
    episode_id TEXT PRIMARY KEY REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX cognition_episode_identity_by_evidence
ON cognition_episode_provider_identity_evidence(evidence_id,episode_id);

CREATE TABLE cognition_episode_replay_provider_identity_evidence (
    replay_id TEXT PRIMARY KEY CHECK (
        replay_id~'^cognition_replay_bootstrap_[0-9a-f]{64}$'
    ),
    episode_id TEXT NOT NULL,
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    provider_observation_json TEXT NOT NULL CHECK (
        jsonb_typeof(provider_observation_json::jsonb)='object' AND
        octet_length(provider_observation_json)<=65536
    ),
    provider_observation_json_sha256 TEXT NOT NULL CHECK (
        provider_observation_json_sha256~'^[0-9a-f]{64}$' AND
        provider_observation_json_sha256=encode(digest(provider_observation_json,'sha256'),'hex')
    ),
    provider_observation_sha256 TEXT NOT NULL CHECK (
        provider_observation_sha256~'^[0-9a-f]{64}$'
    ),
    observed_at TIMESTAMPTZ NOT NULL,
    process_observation_id TEXT NOT NULL UNIQUE CHECK (
        process_observation_id~'^provider_process_observation_[0-9a-f]{64}$'
    ),
    process_receipt_sha256 TEXT NOT NULL CHECK (
        process_receipt_sha256~'^[0-9a-f]{64}$'
    ),
    process_evidence_id TEXT NOT NULL
        REFERENCES cognition_provider_identity_evidence(evidence_id) ON DELETE RESTRICT,
    authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(authority_json::jsonb)='object' AND octet_length(authority_json)<=4096
    ),
    authority_sha256 TEXT NOT NULL CHECK (
        authority_sha256~'^[0-9a-f]{64}$' AND
        authority_sha256=encode(digest(authority_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id)
        ON DELETE RESTRICT,
    UNIQUE (episode_id,replay_id)
);

CREATE INDEX cognition_episode_replay_identity_by_evidence
ON cognition_episode_replay_provider_identity_evidence(evidence_id,episode_id);

CREATE TRIGGER cognition_provider_identity_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_provider_identity_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_provider_identity_evidence_no_truncate
BEFORE TRUNCATE ON cognition_provider_identity_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_provider_identity_operations_immutable
BEFORE UPDATE OR DELETE ON cognition_provider_identity_evidence_operations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_provider_identity_operations_no_truncate
BEFORE TRUNCATE ON cognition_provider_identity_evidence_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_policy_call_identity_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_policy_call_provider_identity_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_policy_call_identity_evidence_no_truncate
BEFORE TRUNCATE ON cognition_policy_call_provider_identity_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_episode_identity_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_episode_provider_identity_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_identity_evidence_no_truncate
BEFORE TRUNCATE ON cognition_episode_provider_identity_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_episode_replay_identity_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_episode_replay_provider_identity_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_replay_identity_evidence_no_truncate
BEFORE TRUNCATE ON cognition_episode_replay_provider_identity_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
