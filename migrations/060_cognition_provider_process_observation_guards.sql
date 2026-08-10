CREATE OR REPLACE FUNCTION cognition_provider_process_receipt_is_exact(
    receipt_json TEXT,
    observation_id TEXT,
    episode_id TEXT,
    job_id BIGINT,
    generation BIGINT,
    step_id BIGINT,
    step_attempt BIGINT,
    worker_id TEXT,
    purpose TEXT,
    stable_brain_json TEXT,
    stable_brain_sha256 TEXT,
    provider_observation_json TEXT,
    provider_observation_sha256 TEXT,
    provider_attestation_sha256 TEXT,
    challenge_sha256 TEXT,
    observed_at TIMESTAMPTZ
) RETURNS BOOLEAN AS $$
    SELECT cognition_json_has_unique_keys(receipt_json::json) AND
           cognition_json_has_unique_keys(stable_brain_json::json) AND
           cognition_json_has_unique_keys(provider_observation_json::json) AND
           receipt_json=cognition_canonical_jsonb(receipt_json::jsonb) AND
           stable_brain_json=cognition_canonical_jsonb(stable_brain_json::jsonb) AND
           provider_observation_json=cognition_canonical_jsonb(provider_observation_json::jsonb) AND
           cognition_json_object_has_exact_keys(receipt_json::json,ARRAY[
               'schema','id','episode_id','actor','purpose','stable_brain','observation'
           ]) AND
           cognition_json_object_has_exact_keys((receipt_json::json->'actor')::json,ARRAY[
               'job_id','generation','step_id','attempt','worker_id'
           ]) AND
           cognition_json_object_has_exact_keys((receipt_json::json->'stable_brain')::json,ARRAY[
               'schema','brain','provider_attestation','host_hardware_attestation','sha256'
           ]) AND
           cognition_json_object_has_exact_keys((receipt_json::json->'observation')::json,ARRAY[
               'schema','observed_at','attestation_sha256','version_body_sha256',
               'installed_body_sha256','preload_body_sha256','runner_body_sha256',
               'preload_method','preload_endpoint','preload_request_sha256',
               'challenge_sha256','observation_sha256'
           ]) AND
           receipt_json::jsonb->>'schema'='omnidex.provider-process-observation.v1' AND
           receipt_json::jsonb->>'id'=observation_id AND
           receipt_json::jsonb->>'episode_id'=episode_id AND
           receipt_json::jsonb->>'purpose'=purpose AND
           (receipt_json::jsonb->'actor'->>'job_id')::BIGINT=job_id AND
           (receipt_json::jsonb->'actor'->>'generation')::BIGINT=generation AND
           (receipt_json::jsonb->'actor'->>'step_id')::BIGINT=step_id AND
           (receipt_json::jsonb->'actor'->>'attempt')::BIGINT=step_attempt AND
           receipt_json::jsonb->'actor'->>'worker_id'=worker_id AND
           receipt_json::jsonb->'stable_brain'=stable_brain_json::jsonb AND
           receipt_json::jsonb->'observation'=provider_observation_json::jsonb AND
           stable_brain_json::jsonb->>'sha256'=stable_brain_sha256 AND
           provider_observation_json::jsonb->>'observation_sha256'=
               provider_observation_sha256 AND
           stable_brain_json::jsonb->'provider_attestation'->>'attestation_sha256'=
               provider_attestation_sha256 AND
           provider_observation_json::jsonb->>'attestation_sha256'=
               provider_attestation_sha256 AND
           provider_observation_json::jsonb->>'challenge_sha256'=challenge_sha256 AND
           provider_observation_json::jsonb->>'observed_at' ~ 'Z$' AND
           (provider_observation_json::jsonb->>'observed_at')::TIMESTAMPTZ=observed_at AND
           NOT EXISTS (
               SELECT 1 FROM jsonb_each_text(provider_observation_json::jsonb) fields
               WHERE fields.key IN (
                   'attestation_sha256','version_body_sha256','installed_body_sha256',
                   'preload_body_sha256','runner_body_sha256','preload_request_sha256',
                   'challenge_sha256','observation_sha256'
               ) AND fields.value !~ '^[0-9a-f]{64}$'
           ) AND
           stable_brain_sha256=encode(digest(cognition_canonical_jsonb(jsonb_set(
               stable_brain_json::jsonb,'{sha256}',to_jsonb(''::TEXT)
           )),'sha256'),'hex') AND
           provider_observation_sha256=encode(digest(cognition_canonical_jsonb(jsonb_set(
               provider_observation_json::jsonb,'{observation_sha256}',to_jsonb(''::TEXT)
           )),'sha256'),'hex') AND
           challenge_sha256=encode(digest(cognition_canonical_jsonb(jsonb_build_object(
               'scope','cognition-process:'||encode(digest(cognition_canonical_jsonb(
                   jsonb_build_object(
                       'episode_id',episode_id,
                       'actor',receipt_json::jsonb->'actor',
                       'purpose',purpose,
                       'stable_brain_sha256',stable_brain_sha256
                   )
               ),'sha256'),'hex'),
               'expectation',jsonb_build_object(
                   'backend',stable_brain_json::jsonb->'brain'->>'backend',
                   'backend_version',stable_brain_json::jsonb->'brain'->>'backend_version',
                   'model',stable_brain_json::jsonb->'brain'->>'model',
                   'digest',stable_brain_json::jsonb->'brain'->>'digest',
                   'quantization',stable_brain_json::jsonb->'brain'->>'quantization',
                   'native_context_limit',
                       (stable_brain_json::jsonb->'brain'->>'native_context_limit')::BIGINT
               )
           )),'sha256'),'hex') AND
           observation_id='provider_process_observation_'||encode(digest(
               cognition_canonical_jsonb(jsonb_set(
                   receipt_json::jsonb,'{id}',to_jsonb(''::TEXT)
               )),'sha256'
           ),'hex');
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION require_exact_cognition_provider_process_observation()
RETURNS TRIGGER AS $$
DECLARE episode_brain JSONB;
DECLARE row_count BIGINT;
DECLARE max_sequence BIGINT;
BEGIN
    SELECT attested_brain_json::jsonb INTO episode_brain
    FROM cognition_episodes WHERE episode_id=NEW.episode_id;
    IF episode_brain IS NULL OR NOT cognition_provider_process_receipt_is_exact(
        NEW.receipt_json,NEW.observation_id,NEW.episode_id,NEW.job_id,NEW.generation,
        NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.purpose,NEW.stable_brain_json,
        NEW.stable_brain_sha256,NEW.provider_observation_json,
        NEW.provider_observation_sha256,NEW.provider_attestation_sha256,
        NEW.challenge_sha256,NEW.observed_at
    ) OR NOT EXISTS (
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
          AND NEW.observed_at>=episodes.created_at AND NEW.observed_at<=NEW.created_at
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
        NEW.receipt_json,NEW.observation_id,NEW.episode_id,NEW.job_id,NEW.generation,
        NEW.step_id,NEW.step_attempt,NEW.worker_id,NEW.purpose,NEW.stable_brain_json,
        NEW.stable_brain_sha256,NEW.provider_observation_json,
        NEW.provider_observation_sha256,NEW.provider_attestation_sha256,
        NEW.challenge_sha256,NEW.observed_at
    ) OR NOT EXISTS (
        SELECT 1 FROM cognition_terminal_seals seals
        JOIN cognition_episodes episodes ON episodes.episode_id=seals.episode_id
        JOIN jobs ON jobs.id=episodes.job_id
        JOIN job_steps steps ON steps.job_id=episodes.job_id AND steps.id=episodes.step_id
        JOIN job_step_attempts attempts
          ON attempts.job_id=episodes.job_id AND attempts.generation=episodes.generation
         AND attempts.step_id=episodes.step_id AND attempts.attempt=NEW.step_attempt
         AND attempts.worker_id=NEW.worker_id
        WHERE seals.episode_id=NEW.episode_id AND seals.trace_sha256=NEW.terminal_trace_sha256
          AND episodes.status IN ('completed','failed','canceled')
          AND episodes.job_id=NEW.job_id AND episodes.generation=NEW.generation
          AND episodes.step_id=NEW.step_id AND jobs.status='running'
          AND jobs.current_generation=NEW.generation AND steps.status='running'
          AND steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id
          AND attempts.status='active' AND attempts.expires_at>clock_timestamp()
          AND NEW.observed_at>=seals.created_at AND NEW.observed_at<=NEW.created_at
          AND NEW.receipt_json::jsonb->'stable_brain'->'brain'=
              episodes.attested_brain_json::jsonb->'brain'
          AND NEW.receipt_json::jsonb->'stable_brain'->'provider_attestation'=
              episodes.attested_brain_json::jsonb->'provider_attestation'
          AND NEW.receipt_json::jsonb->'stable_brain'->'host_hardware_attestation'=
              episodes.attested_brain_json::jsonb->'host_hardware_attestation'
    ) THEN
        RAISE EXCEPTION 'post-seal provider observation lacks exact terminal authority';
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
