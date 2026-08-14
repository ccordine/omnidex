CREATE OR REPLACE FUNCTION require_exact_cognition_policy_call_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
DECLARE identity cognition_provider_identity_evidence%ROWTYPE;
DECLARE matches_attempt BOOLEAN;
BEGIN
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    SELECT * INTO identity FROM cognition_provider_identity_evidence evidence
    WHERE evidence.evidence_id=NEW.evidence_id;
    IF call_row.call_id IS NULL OR identity.evidence_id IS NULL OR
       call_row.status NOT IN ('accepted','rejected','failed') OR
       call_row.result_json::jsonb->'provider_identity_evidence'<>identity.ref_json::jsonb OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id) IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) OR
       NOT cognition_provider_identity_requests_match_brain(
           NEW.evidence_id,call_row.attempt_json::jsonb->'brain'
       ) THEN
        RAISE EXCEPTION 'provider identity evidence lacks its exact terminal call';
    END IF;
    matches_attempt := cognition_provider_identity_evidence_matches_attempt(
        NEW.evidence_id,call_row.attempt_json::jsonb
    );
    IF call_row.provider_identity_checked AND NOT cognition_provider_observed_identity_is_exact(
       call_row.result_json::jsonb->'provider_attestation',
       call_row.result_json::jsonb->'provider_observation',
       call_row.attempt_json::jsonb->'brain',
       cognition_call_provider_challenge(call_row.call_id,call_row.attempt_json::jsonb->'brain'),
       NEW.evidence_id
    ) THEN
        RAISE EXCEPTION 'provider observation differs from its exact raw call evidence';
    END IF;
    IF call_row.result_json::jsonb->>'failure_code'='provider_identity_error' THEN
        IF matches_attempt THEN
            RAISE EXCEPTION 'provider identity error raw evidence matches the frozen provider';
        END IF;
    ELSIF NOT matches_attempt THEN
        RAISE EXCEPTION 'provider generation call raw identity differs from frozen provider';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_call_identity_evidence_exact
AFTER INSERT ON cognition_policy_call_provider_identity_evidence
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_policy_call_identity_evidence();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE ref JSONB;
BEGIN
    IF NEW.status NOT IN ('accepted','rejected','failed') THEN RETURN NULL; END IF;
    ref := NEW.result_json::jsonb->'provider_identity_evidence';
    IF ref='{"schema":"","id":"","sha256":"","bytes":0}'::jsonb THEN
        IF EXISTS (SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
                   WHERE association.call_id=NEW.call_id) THEN
            RAISE EXCEPTION 'terminal call has extraneous provider identity evidence';
        END IF;
    ELSIF NOT EXISTS (
        SELECT 1 FROM cognition_policy_call_provider_identity_evidence association
        JOIN cognition_provider_identity_evidence evidence
          ON evidence.evidence_id=association.evidence_id
        WHERE association.call_id=NEW.call_id AND evidence.ref_json::jsonb=ref
    ) THEN
        RAISE EXCEPTION 'terminal call lacks exact provider identity evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_identity_evidence
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_identity_evidence();

CREATE OR REPLACE FUNCTION require_exact_cognition_episode_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE episode_row cognition_episodes%ROWTYPE;
DECLARE brain JSONB;
BEGIN
    SELECT * INTO episode_row
    FROM cognition_episodes WHERE episode_id=NEW.episode_id;
    brain := episode_row.attested_brain_json::jsonb;
    IF brain IS NULL OR NOT cognition_provider_observed_identity_is_exact(
       brain->'provider_attestation',brain->'bootstrap_provider_observation',brain->'brain',
       cognition_provider_bootstrap_challenge(brain->'brain'),NEW.evidence_id
    ) OR NOT EXISTS (
        SELECT 1 FROM job_step_attempts attempts
        WHERE attempts.job_id=episode_row.job_id
          AND attempts.generation=episode_row.generation
          AND attempts.step_id=episode_row.step_id
          AND attempts.attempt=episode_row.created_attempt
          AND attempts.worker_id=episode_row.created_worker_id
          AND (brain#>>'{bootstrap_provider_observation,observed_at}')::TIMESTAMPTZ>=
              attempts.claimed_at
    ) THEN
        RAISE EXCEPTION 'episode bootstrap differs from its exact raw evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episode_identity_evidence_exact
AFTER INSERT ON cognition_episode_provider_identity_evidence
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_episode_identity_evidence();

CREATE OR REPLACE FUNCTION require_exact_cognition_episode_replay_identity_evidence()
RETURNS TRIGGER AS $$
DECLARE episode_row cognition_episodes%ROWTYPE;
DECLARE expected_authority JSONB;
DECLARE brain JSONB;
DECLARE process_row RECORD;
BEGIN
    SELECT * INTO episode_row FROM cognition_episodes WHERE episode_id=NEW.episode_id;
    SELECT * INTO process_row FROM cognition_provider_process_observations
    WHERE observation_id=NEW.process_observation_id;
    brain := episode_row.attested_brain_json::jsonb;
    expected_authority := jsonb_build_object(
        'schema','omnidex.cognition-episode-replay-bootstrap.v1',
        'episode_id',NEW.episode_id,'job_id',NEW.job_id,'generation',NEW.generation,
        'step_id',NEW.step_id,'attempt',NEW.step_attempt,'worker_id',NEW.worker_id,
        'observation_sha256',NEW.provider_observation_sha256,'evidence_id',NEW.evidence_id,
        'process_observation_id',NEW.process_observation_id,
        'process_receipt_sha256',NEW.process_receipt_sha256,
        'process_evidence_id',NEW.process_evidence_id
    );
    IF episode_row.episode_id IS NULL OR
       NOT cognition_json_has_unique_keys(NEW.authority_json::json) OR
       NEW.authority_json<>cognition_canonical_jsonb(expected_authority) OR
       NEW.replay_id<>'cognition_replay_bootstrap_'||NEW.authority_sha256 OR
       NOT cognition_json_has_unique_keys(NEW.provider_observation_json::json) OR
       NEW.provider_observation_json<>cognition_canonical_jsonb(
           NEW.provider_observation_json::jsonb
       ) OR
       NEW.provider_observation_sha256<>
           NEW.provider_observation_json::jsonb->>'observation_sha256' OR
       (NEW.provider_observation_json::jsonb->>'observed_at')::TIMESTAMPTZ<>NEW.observed_at OR
       NEW.observed_at<episode_row.created_at OR NEW.observed_at>NEW.created_at OR
       process_row.observation_id IS NULL OR
       ROW(process_row.episode_id,process_row.job_id,process_row.generation,
           process_row.step_id,process_row.step_attempt,process_row.worker_id,
           process_row.purpose,process_row.evidence_id,process_row.receipt_sha256) IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,
           NEW.worker_id,'episode_invocation',NEW.process_evidence_id,
           NEW.process_receipt_sha256) OR
       process_row.observed_at<NEW.observed_at OR process_row.created_at<NEW.created_at OR
       NOT cognition_stable_brain_is_exact(process_row.stable_brain_json::jsonb) OR
       process_row.stable_brain_json::jsonb->'brain'<>brain->'brain' OR
       process_row.stable_brain_json::jsonb->'provider_attestation'<>
           brain->'provider_attestation' OR
       process_row.stable_brain_json::jsonb->'host_hardware_attestation'<>
           brain->'host_hardware_attestation' OR
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
             AND attempts.expires_at>clock_timestamp()
             AND NEW.observed_at>=attempts.claimed_at
       ) THEN
        RAISE EXCEPTION 'episode replay bootstrap lacks exact current authority';
    END IF;
    RETURN NULL;
EXCEPTION WHEN OTHERS THEN
    IF SQLERRM LIKE 'episode replay bootstrap lacks exact current authority%' THEN RAISE; END IF;
    RAISE EXCEPTION 'episode replay bootstrap lacks exact current authority';
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episode_replay_identity_evidence_exact
AFTER INSERT ON cognition_episode_replay_provider_identity_evidence
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_episode_replay_identity_evidence();
