CREATE OR REPLACE FUNCTION require_exact_cognition_provider_process_observation()
RETURNS TRIGGER AS $$
DECLARE episode_brain JSONB;
DECLARE row_count BIGINT;
DECLARE max_sequence BIGINT;
BEGIN
    SELECT attested_brain_json::jsonb INTO episode_brain
    FROM cognition_episodes WHERE episode_id=NEW.episode_id;
    IF episode_brain IS NULL THEN
        RAISE EXCEPTION 'provider process observation has no persisted episode Brain';
    ELSIF NOT cognition_provider_process_receipt_is_exact(
		NEW.receipt_json,NEW.observation_id,NEW.evidence_id,NEW.episode_id,NEW.job_id,NEW.generation,
        NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.purpose,NEW.stable_brain_json,
        NEW.stable_brain_sha256,NEW.provider_observation_json,
        NEW.provider_observation_sha256,NEW.provider_attestation_sha256,
        NEW.challenge_sha256,NEW.observed_at
	) THEN
        RAISE EXCEPTION 'provider process observation receipt is inexact';
    ELSIF NOT EXISTS (
        SELECT 1 FROM cognition_episodes episodes
        JOIN jobs ON jobs.id=episodes.job_id
        JOIN job_steps steps ON steps.job_id=episodes.job_id AND steps.id=episodes.step_id
        JOIN job_step_attempts attempts
          ON attempts.job_id=episodes.job_id AND attempts.generation=episodes.generation
         AND attempts.step_id=episodes.step_id AND attempts.attempt=NEW.step_attempt
         AND attempts.worker_id=NEW.worker_id
        WHERE episodes.episode_id=NEW.episode_id AND episodes.status='active'
          AND episodes.job_id=NEW.job_id AND episodes.generation=NEW.generation
          AND episodes.step_id=NEW.step_id AND jobs.status='running'
          AND jobs.current_generation=NEW.generation AND steps.status='running'
          AND steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id
          AND attempts.status='active' AND attempts.expires_at>clock_timestamp()
          AND NEW.observed_at>=attempts.claimed_at AND NEW.observed_at<=NEW.created_at
          AND NEW.receipt_json::jsonb->'stable_brain'->'brain'=episode_brain->'brain'
          AND NEW.receipt_json::jsonb->'stable_brain'->'provider_attestation'=
              episode_brain->'provider_attestation'
          AND NEW.receipt_json::jsonb->'stable_brain'->'host_hardware_attestation'=
              episode_brain->'host_hardware_attestation'
    ) THEN
        RAISE EXCEPTION 'provider process observation lacks exact active authority';
    END IF;
    SELECT COUNT(*),MAX(sequence) INTO row_count,max_sequence
    FROM cognition_provider_process_observations WHERE episode_id=NEW.episode_id;
    IF row_count<>max_sequence THEN
        RAISE EXCEPTION 'provider process observation sequence is not contiguous';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_process_observations_exact
AFTER INSERT ON cognition_provider_process_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_provider_process_observation();

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
          AND NEW.observed_at>=seals.created_at AND NEW.observed_at<=NEW.created_at
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
          AND attempts.status='active' AND attempts.expires_at>clock_timestamp()
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
        NEW.receipt_sha256,'sha256'
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

CREATE CONSTRAINT TRIGGER cognition_provider_postseal_observations_exact
AFTER INSERT ON cognition_provider_postseal_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_provider_postseal_observation();

CREATE OR REPLACE FUNCTION require_cognition_provider_observation_cross_table_unique()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_TABLE_NAME='cognition_provider_process_observations' AND EXISTS (
        SELECT 1 FROM cognition_provider_postseal_observations
        WHERE observation_id=NEW.observation_id
    ) THEN
        RAISE EXCEPTION 'provider process observation identity already exists post-seal';
    ELSIF TG_TABLE_NAME='cognition_provider_postseal_observations' AND EXISTS (
        SELECT 1 FROM cognition_provider_process_observations
        WHERE observation_id=NEW.observation_id
    ) THEN
        RAISE EXCEPTION 'provider process observation identity already exists pre-seal';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_process_observation_cross_table_unique
AFTER INSERT ON cognition_provider_process_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_observation_cross_table_unique();

CREATE CONSTRAINT TRIGGER cognition_provider_postseal_observation_cross_table_unique
AFTER INSERT ON cognition_provider_postseal_observations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_provider_observation_cross_table_unique();

CREATE OR REPLACE FUNCTION cognition_episode_has_sealed_provider_identity_evidence(
    wanted_episode TEXT,
    wanted_evidence TEXT
) RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM cognition_terminal_seals seals
        WHERE seals.episode_id=wanted_episode AND (
            EXISTS (SELECT 1 FROM cognition_episode_provider_identity_evidence value
                WHERE value.episode_id=wanted_episode AND value.evidence_id=wanted_evidence) OR
            EXISTS (SELECT 1 FROM cognition_episode_replay_provider_identity_evidence value
                WHERE value.episode_id=wanted_episode AND value.evidence_id=wanted_evidence) OR
            EXISTS (SELECT 1 FROM cognition_policy_call_provider_identity_evidence value
                WHERE value.episode_id=wanted_episode AND value.evidence_id=wanted_evidence) OR
            EXISTS (SELECT 1 FROM cognition_provider_process_observations value
                WHERE value.episode_id=wanted_episode AND value.evidence_id=wanted_evidence) OR
            EXISTS (SELECT 1 FROM cognition_provider_postseal_observations value
                WHERE value.episode_id=wanted_episode AND value.evidence_id=wanted_evidence)
            OR EXISTS (SELECT 1 FROM cognition_provider_activation_failures value
                WHERE value.episode_id=wanted_episode AND
                      wanted_evidence IN (value.evidence_id,value.bootstrap_evidence_id))
        )
    );
$$ LANGUAGE SQL STABLE STRICT;
