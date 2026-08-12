DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_provider_postseal_observations) THEN
        RAISE EXCEPTION 'cannot classify existing post-seal provider observations exactly';
    END IF;
END;
$$;

ALTER TABLE cognition_episode_replay_provider_identity_evidence
ADD CONSTRAINT cognition_episode_replay_process_observation_fkey
FOREIGN KEY (process_observation_id)
REFERENCES cognition_provider_process_observations(observation_id)
ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE cognition_provider_postseal_observations
ADD COLUMN source_kind TEXT NOT NULL CHECK (source_kind IN ('direct_audit','episode_replay'));

CREATE TABLE cognition_episode_postseal_replay_bootstrap_audits (
    audit_id TEXT PRIMARY KEY CHECK (
        audit_id~'^cognition_postseal_replay_bootstrap_[0-9a-f]{64}$'
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
    terminal_trace_sha256 TEXT NOT NULL CHECK (terminal_trace_sha256~'^[0-9a-f]{64}$'),
    process_observation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_provider_postseal_observations(observation_id) ON DELETE RESTRICT,
    process_chain_sha256 TEXT NOT NULL CHECK (process_chain_sha256~'^[0-9a-f]{64}$'),
    authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(authority_json::jsonb)='object' AND octet_length(authority_json)<=8192
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
    FOREIGN KEY (episode_id) REFERENCES cognition_terminal_seals(episode_id) ON DELETE RESTRICT,
    UNIQUE (episode_id,job_id,generation,step_id,step_attempt,worker_id)
);

CREATE INDEX cognition_postseal_replay_bootstrap_evidence
ON cognition_episode_postseal_replay_bootstrap_audits(evidence_id,episode_id);

CREATE TRIGGER cognition_postseal_replay_bootstrap_audits_immutable
BEFORE UPDATE OR DELETE ON cognition_episode_postseal_replay_bootstrap_audits
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_postseal_replay_bootstrap_audits_no_truncate
BEFORE TRUNCATE ON cognition_episode_postseal_replay_bootstrap_audits
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE FUNCTION require_active_cognition_episode_replay_bootstrap()
RETURNS TRIGGER AS $$
DECLARE episode_status TEXT;
BEGIN
    SELECT status INTO episode_status FROM cognition_episodes
    WHERE episode_id=NEW.episode_id FOR UPDATE;
    IF episode_status IS DISTINCT FROM 'active' THEN
        RAISE EXCEPTION 'episode replay bootstrap requires an active episode';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_episode_replay_bootstrap_requires_active
BEFORE INSERT ON cognition_episode_replay_provider_identity_evidence
FOR EACH ROW EXECUTE FUNCTION require_active_cognition_episode_replay_bootstrap();

CREATE FUNCTION require_exact_cognition_postseal_replay_bootstrap_audit()
RETURNS TRIGGER AS $$
DECLARE episode_row cognition_episodes%ROWTYPE;
DECLARE process_row cognition_provider_postseal_observations%ROWTYPE;
DECLARE seal_trace TEXT;
DECLARE expected_authority JSONB;
DECLARE brain JSONB;
BEGIN
    SELECT * INTO episode_row FROM cognition_episodes
    WHERE episode_id=NEW.episode_id FOR UPDATE;
    SELECT trace_sha256 INTO seal_trace
    FROM cognition_terminal_seals WHERE episode_id=NEW.episode_id;
    SELECT * INTO process_row FROM cognition_provider_postseal_observations
    WHERE observation_id=NEW.process_observation_id;
    brain := episode_row.attested_brain_json::jsonb;
    expected_authority := jsonb_build_object(
        'schema','omnidex.cognition-postseal-replay-bootstrap-audit.v1',
        'episode_id',NEW.episode_id,'job_id',NEW.job_id,'generation',NEW.generation,
        'step_id',NEW.step_id,'attempt',NEW.step_attempt,'worker_id',NEW.worker_id,
        'observation_sha256',NEW.provider_observation_sha256,'evidence_id',NEW.evidence_id,
        'terminal_trace_sha256',NEW.terminal_trace_sha256,
        'process_observation_id',NEW.process_observation_id,
        'process_chain_sha256',NEW.process_chain_sha256
    );
    IF episode_row.episode_id IS NULL OR
       episode_row.status NOT IN ('completed','failed','canceled') OR
       ROW(episode_row.job_id,episode_row.generation,episode_row.step_id) IS DISTINCT FROM
           ROW(NEW.job_id,NEW.generation,NEW.step_id) OR
       seal_trace IS NULL OR seal_trace<>NEW.terminal_trace_sha256 OR
       process_row.observation_id IS NULL OR
       ROW(process_row.episode_id,process_row.job_id,process_row.generation,
           process_row.step_id,process_row.step_attempt,process_row.worker_id,
           process_row.source_kind,process_row.terminal_trace_sha256,process_row.chain_sha256) IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,
           NEW.worker_id,'episode_replay',NEW.terminal_trace_sha256,NEW.process_chain_sha256) OR
       NOT cognition_json_has_unique_keys(NEW.authority_json::json) OR
       NEW.authority_json<>cognition_canonical_jsonb(expected_authority) OR
       NEW.audit_id<>'cognition_postseal_replay_bootstrap_'||NEW.authority_sha256 OR
       NOT cognition_json_has_unique_keys(NEW.provider_observation_json::json) OR
       NEW.provider_observation_json<>cognition_canonical_jsonb(
           NEW.provider_observation_json::jsonb
       ) OR
       NEW.provider_observation_sha256<>
           NEW.provider_observation_json::jsonb->>'observation_sha256' OR
       (NEW.provider_observation_json::jsonb->>'observed_at')::TIMESTAMPTZ<>NEW.observed_at OR
       NEW.observed_at<episode_row.created_at OR NEW.observed_at>NEW.created_at OR
       NOT cognition_provider_observed_identity_is_exact(
           brain->'provider_attestation',NEW.provider_observation_json::jsonb,brain->'brain',
           cognition_provider_bootstrap_challenge(brain->'brain'),NEW.evidence_id
       ) OR NOT EXISTS (
           SELECT 1 FROM jobs
           JOIN job_steps ON job_steps.job_id=jobs.id AND job_steps.id=NEW.step_id
           JOIN job_step_attempts attempts
             ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation
            AND attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt
            AND attempts.worker_id=NEW.worker_id
           WHERE jobs.id=NEW.job_id AND jobs.status='running'
             AND jobs.current_generation=NEW.generation AND job_steps.status='running'
             AND job_steps.generation=NEW.generation
             AND job_steps.superseded_at_generation IS NULL
             AND job_steps.current_attempt=NEW.step_attempt
             AND job_steps.worker_id=NEW.worker_id AND attempts.status='active'
             AND NEW.observed_at>=attempts.claimed_at
             AND attempts.expires_at>clock_timestamp()
       ) THEN
        RAISE EXCEPTION 'post-seal replay bootstrap audit lacks exact terminal invocation authority';
    END IF;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    IF SQLERRM LIKE 'post-seal replay bootstrap audit lacks exact terminal invocation authority%' THEN
        RAISE;
    END IF;
    RAISE EXCEPTION 'post-seal replay bootstrap audit lacks exact terminal invocation authority';
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_postseal_replay_bootstrap_audits_exact
AFTER INSERT ON cognition_episode_postseal_replay_bootstrap_audits
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_postseal_replay_bootstrap_audit();

CREATE OR REPLACE FUNCTION require_exact_cognition_provider_postseal_observation()
RETURNS TRIGGER AS $$
DECLARE expected_previous TEXT;
DECLARE expected_chain TEXT;
DECLARE row_count BIGINT;
DECLARE max_sequence BIGINT;
BEGIN
    IF NOT cognition_provider_process_receipt_is_exact(
        NEW.receipt_json,NEW.observation_id,NEW.evidence_id,NEW.episode_id,NEW.job_id,NEW.generation,
        NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.purpose,NEW.stable_brain_json,
        NEW.stable_brain_sha256,NEW.provider_observation_json,
        NEW.provider_observation_sha256,NEW.provider_attestation_sha256,
        NEW.challenge_sha256,NEW.observed_at
    ) THEN
        RAISE EXCEPTION 'post-seal provider observation receipt is inexact';
    ELSIF NOT EXISTS (
        SELECT 1 FROM cognition_terminal_seals seals
        JOIN cognition_episodes episodes ON episodes.episode_id=seals.episode_id
        WHERE seals.episode_id=NEW.episode_id AND seals.trace_sha256=NEW.terminal_trace_sha256
          AND episodes.status IN ('completed','failed','canceled')
          AND episodes.job_id=NEW.job_id AND episodes.generation=NEW.generation
          AND episodes.step_id=NEW.step_id
          AND NEW.observed_at>=episodes.created_at AND NEW.observed_at<=NEW.created_at
          AND NEW.receipt_json::jsonb->'stable_brain'->'brain'=
              episodes.attested_brain_json::jsonb->'brain'
          AND NEW.receipt_json::jsonb->'stable_brain'->'provider_attestation'=
              episodes.attested_brain_json::jsonb->'provider_attestation'
          AND NEW.receipt_json::jsonb->'stable_brain'->'host_hardware_attestation'=
              episodes.attested_brain_json::jsonb->'host_hardware_attestation'
    ) THEN
        RAISE EXCEPTION 'post-seal provider observation lacks exact terminal authority';
    ELSIF NOT EXISTS (
        SELECT 1 FROM jobs
        JOIN job_steps steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
        JOIN job_step_attempts attempts
          ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation
         AND attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt
         AND attempts.worker_id=NEW.worker_id
        WHERE jobs.id=NEW.job_id AND jobs.status='running'
          AND jobs.current_generation=NEW.generation AND steps.status='running'
          AND steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id
          AND attempts.status='active' AND NEW.observed_at>=attempts.claimed_at
          AND attempts.expires_at>clock_timestamp()
    ) THEN
        RAISE EXCEPTION 'post-seal provider observation lacks exact live actor authority';
    END IF;
    IF NEW.sequence=1 THEN
        expected_previous := NEW.terminal_trace_sha256;
    ELSE
        SELECT chain_sha256 INTO expected_previous
        FROM cognition_provider_postseal_observations
        WHERE episode_id=NEW.episode_id AND sequence=NEW.sequence-1;
    END IF;
    expected_chain := encode(digest(
        NEW.terminal_trace_sha256||':'||expected_previous||':'||NEW.sequence::TEXT||':'||
        NEW.source_kind||':'||NEW.receipt_sha256,'sha256'
    ),'hex');
    IF expected_previous IS NULL OR NEW.previous_chain_sha256<>expected_previous OR
       NEW.chain_sha256<>expected_chain THEN
        RAISE EXCEPTION 'post-seal provider observation broke the append-only chain';
    END IF;
    SELECT COUNT(*),MAX(sequence) INTO row_count,max_sequence
    FROM cognition_provider_postseal_observations WHERE episode_id=NEW.episode_id;
    IF row_count<>max_sequence THEN
        RAISE EXCEPTION 'post-seal provider observation sequence is not contiguous';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION require_cognition_postseal_replay_bootstrap_totality()
RETURNS TRIGGER AS $$
DECLARE audit_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO audit_count
    FROM cognition_episode_postseal_replay_bootstrap_audits
    WHERE process_observation_id=NEW.observation_id;
    IF (NEW.source_kind='episode_replay' AND audit_count<>1) OR
       (NEW.source_kind='direct_audit' AND audit_count<>0) THEN
        RAISE EXCEPTION 'post-seal provider observation source lacks exact bootstrap audit totality';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_postseal_replay_bootstrap_totality
AFTER INSERT ON cognition_provider_postseal_observations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_postseal_replay_bootstrap_totality();
