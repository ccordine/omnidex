CREATE TABLE cognition_provider_process_observations (
    observation_id TEXT PRIMARY KEY CHECK (
        observation_id~'^provider_process_observation_[0-9a-f]{64}$'
    ),
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    purpose TEXT NOT NULL CHECK (purpose='episode_invocation'),
    sequence BIGINT NOT NULL CHECK (sequence>0),
    stable_brain_json TEXT NOT NULL CHECK (
        jsonb_typeof(stable_brain_json::jsonb)='object' AND octet_length(stable_brain_json)<=65536
    ),
    stable_brain_json_sha256 TEXT NOT NULL CHECK (
        stable_brain_json_sha256~'^[0-9a-f]{64}$' AND
        stable_brain_json_sha256=encode(digest(stable_brain_json,'sha256'),'hex')
    ),
    stable_brain_sha256 TEXT NOT NULL CHECK (stable_brain_sha256~'^[0-9a-f]{64}$'),
    provider_attestation_sha256 TEXT NOT NULL CHECK (
        provider_attestation_sha256~'^[0-9a-f]{64}$'
    ),
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
    challenge_sha256 TEXT NOT NULL CHECK (challenge_sha256~'^[0-9a-f]{64}$'),
    receipt_json TEXT NOT NULL CHECK (
        jsonb_typeof(receipt_json::jsonb)='object' AND octet_length(receipt_json)<=131072
    ),
    receipt_sha256 TEXT NOT NULL CHECK (
        receipt_sha256~'^[0-9a-f]{64}$' AND
        receipt_sha256=encode(digest(receipt_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id)
        ON DELETE RESTRICT,
    UNIQUE (episode_id,sequence),
    UNIQUE (episode_id,observation_id)
);

CREATE INDEX cognition_provider_process_identity_by_evidence
ON cognition_provider_process_observations(evidence_id,episode_id,sequence);

CREATE TABLE cognition_provider_postseal_observations (
    observation_id TEXT PRIMARY KEY CHECK (
        observation_id~'^provider_process_observation_[0-9a-f]{64}$'
    ),
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    purpose TEXT NOT NULL CHECK (purpose='episode_invocation'),
    sequence BIGINT NOT NULL CHECK (sequence>0),
    terminal_trace_sha256 TEXT NOT NULL CHECK (terminal_trace_sha256~'^[0-9a-f]{64}$'),
    previous_chain_sha256 TEXT NOT NULL CHECK (previous_chain_sha256~'^[0-9a-f]{64}$'),
    chain_sha256 TEXT NOT NULL UNIQUE CHECK (chain_sha256~'^[0-9a-f]{64}$'),
    stable_brain_json TEXT NOT NULL CHECK (
        jsonb_typeof(stable_brain_json::jsonb)='object' AND octet_length(stable_brain_json)<=65536
    ),
    stable_brain_json_sha256 TEXT NOT NULL CHECK (
        stable_brain_json_sha256~'^[0-9a-f]{64}$' AND
        stable_brain_json_sha256=encode(digest(stable_brain_json,'sha256'),'hex')
    ),
    stable_brain_sha256 TEXT NOT NULL CHECK (stable_brain_sha256~'^[0-9a-f]{64}$'),
    provider_attestation_sha256 TEXT NOT NULL CHECK (
        provider_attestation_sha256~'^[0-9a-f]{64}$'
    ),
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
    challenge_sha256 TEXT NOT NULL CHECK (challenge_sha256~'^[0-9a-f]{64}$'),
    receipt_json TEXT NOT NULL CHECK (
        jsonb_typeof(receipt_json::jsonb)='object' AND octet_length(receipt_json)<=131072
    ),
    receipt_sha256 TEXT NOT NULL CHECK (
        receipt_sha256~'^[0-9a-f]{64}$' AND
        receipt_sha256=encode(digest(receipt_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (episode_id) REFERENCES cognition_terminal_seals(episode_id) ON DELETE RESTRICT,
    UNIQUE (episode_id,sequence),
    UNIQUE (episode_id,observation_id)
);

CREATE INDEX cognition_provider_postseal_identity_by_evidence
ON cognition_provider_postseal_observations(evidence_id,episode_id,sequence);

CREATE TRIGGER cognition_provider_process_observations_immutable
BEFORE UPDATE OR DELETE ON cognition_provider_process_observations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_provider_process_observations_no_truncate
BEFORE TRUNCATE ON cognition_provider_process_observations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_provider_postseal_observations_immutable
BEFORE UPDATE OR DELETE ON cognition_provider_postseal_observations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_provider_postseal_observations_no_truncate
BEFORE TRUNCATE ON cognition_provider_postseal_observations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
