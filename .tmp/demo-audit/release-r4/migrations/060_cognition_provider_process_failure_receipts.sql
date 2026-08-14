CREATE TABLE cognition_provider_activation_failures (
    record_number BIGSERIAL NOT NULL UNIQUE CHECK (record_number>0),
    record_id TEXT PRIMARY KEY CHECK (
        record_id~'^cognition_provider_failure_[0-9a-f]{64}$'
    ),
    failure_kind TEXT NOT NULL CHECK (failure_kind IN ('brain_bootstrap','provider_process')),
    failure_id TEXT NOT NULL CHECK (
        failure_id~'^(brain_bootstrap_failure|provider_process_failure)_[0-9a-f]{64}$'
    ),
    episode_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(episode_id)),
    evidence_id TEXT NOT NULL REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    bootstrap_evidence_id TEXT REFERENCES cognition_provider_identity_evidence(evidence_id)
        ON DELETE RESTRICT,
    bootstrap_brain_json TEXT CHECK (
        bootstrap_brain_json IS NULL OR (
            jsonb_typeof(bootstrap_brain_json::jsonb)='object' AND
            octet_length(bootstrap_brain_json)<=131072
        )
    ),
    bootstrap_brain_sha256 TEXT CHECK (
        bootstrap_brain_sha256 IS NULL OR (
            bootstrap_brain_sha256~'^[0-9a-f]{64}$' AND
            bootstrap_brain_sha256=encode(digest(bootstrap_brain_json,'sha256'),'hex')
        )
    ),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    receipt_json TEXT NOT NULL CHECK (
        jsonb_typeof(receipt_json::jsonb)='object' AND octet_length(receipt_json)<=262144
    ),
    receipt_sha256 TEXT NOT NULL CHECK (
        receipt_sha256~'^[0-9a-f]{64}$' AND
        receipt_sha256=encode(digest(receipt_json,'sha256'),'hex')
    ),
    authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(authority_json::jsonb)='object' AND octet_length(authority_json)<=16384
    ),
    authority_sha256 TEXT NOT NULL CHECK (
        authority_sha256~'^[0-9a-f]{64}$' AND
        authority_sha256=encode(digest(authority_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id)
        ON DELETE RESTRICT,
    UNIQUE (episode_id,job_id,generation,step_id,step_attempt,worker_id)
    ,CHECK (
        (failure_kind='brain_bootstrap' AND bootstrap_evidence_id IS NULL AND
         bootstrap_brain_json IS NULL AND bootstrap_brain_sha256 IS NULL) OR
        (failure_kind='provider_process' AND bootstrap_evidence_id IS NOT NULL AND
         bootstrap_brain_json IS NOT NULL AND bootstrap_brain_sha256 IS NOT NULL AND
         bootstrap_evidence_id<>evidence_id)
    )
);

CREATE INDEX cognition_provider_activation_failures_episode_page
ON cognition_provider_activation_failures(
    episode_id,job_id,generation,step_id,step_attempt,worker_id,record_number
);

CREATE TRIGGER cognition_provider_activation_failures_immutable
BEFORE UPDATE OR DELETE ON cognition_provider_activation_failures
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_provider_activation_failures_no_truncate
BEFORE TRUNCATE ON cognition_provider_activation_failures
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE OR REPLACE FUNCTION require_exact_cognition_provider_activation_failure()
RETURNS TRIGGER AS $$
DECLARE receipt JSON;
DECLARE authority JSON;
DECLARE brain JSONB;
DECLARE code TEXT;
DECLARE evidence_ref JSONB;
DECLARE attempt_claimed_at TIMESTAMPTZ;
BEGIN
    receipt := NEW.receipt_json::json;
    authority := NEW.authority_json::json;
    SELECT ref_json::jsonb INTO evidence_ref FROM cognition_provider_identity_evidence
    WHERE evidence_id=NEW.evidence_id;
    IF evidence_ref IS NULL OR NOT cognition_json_has_unique_keys(receipt) OR
       NEW.receipt_json<>cognition_canonical_jsonb(receipt::jsonb) OR
       NOT cognition_json_has_unique_keys(authority) OR
       NEW.authority_json<>cognition_canonical_jsonb(authority::jsonb) OR
       NOT cognition_json_object_has_exact_keys(authority,ARRAY[
           'schema','record_id','failure_kind','failure_id','episode_id','actor',
           'evidence_id','receipt_sha256','bootstrap_evidence_id','bootstrap_brain_sha256'
       ]) OR NOT cognition_json_object_has_exact_keys(
           (authority::json->'actor')::json,
           ARRAY['job_id','generation','step_id','attempt','worker_id']
       ) OR authority::jsonb->>'schema'<>'omnidex.cognition-provider-failure-authority.v1' OR
       authority::jsonb->>'record_id'<>NEW.record_id OR
       authority::jsonb->>'failure_kind'<>NEW.failure_kind OR
       authority::jsonb->>'failure_id'<>NEW.failure_id OR
       authority::jsonb->>'episode_id'<>NEW.episode_id OR
       authority::jsonb->>'evidence_id'<>NEW.evidence_id OR
       authority::jsonb->>'receipt_sha256'<>NEW.receipt_sha256 OR
       authority::jsonb->>'bootstrap_evidence_id'<>COALESCE(NEW.bootstrap_evidence_id,'') OR
       authority::jsonb->>'bootstrap_brain_sha256'<>COALESCE(NEW.bootstrap_brain_sha256,'') OR
       authority::jsonb->'actor'<>jsonb_build_object(
           'job_id',NEW.job_id,'generation',NEW.generation,'step_id',NEW.step_id,
           'attempt',NEW.step_attempt,'worker_id',NEW.worker_id
       ) OR receipt::jsonb->>'id'<>NEW.failure_id OR
       receipt::jsonb->'evidence'<>evidence_ref THEN
        RAISE EXCEPTION 'provider activation failure authority is inexact';
    END IF;
    IF NEW.failure_kind='brain_bootstrap' THEN
        IF NOT cognition_json_object_has_exact_keys(receipt,ARRAY[
            'schema','id','brain','challenge_sha256','code','provider_attestation',
            'provider_observation','evidence'
        ]) OR receipt::jsonb->>'schema'<>'omnidex.brain-bootstrap-failure.v1' OR
           NEW.failure_id !~ '^brain_bootstrap_failure_' OR
           receipt::jsonb->>'code'='host_identity_mismatch' THEN
            RAISE EXCEPTION 'Brain bootstrap failure receipt is inexact';
        END IF;
        brain := receipt::jsonb->'brain';
    ELSE
        IF NOT cognition_json_object_has_exact_keys(receipt,ARRAY[
            'schema','id','episode_id','actor','purpose','stable_brain','code',
            'provider_attestation','provider_observation',
            'live_host_hardware_attestation','evidence'
        ]) OR receipt::jsonb->>'schema'<>'omnidex.provider-process-failure.v1' OR
           NEW.failure_id !~ '^provider_process_failure_' OR
           receipt::jsonb->>'episode_id'<>NEW.episode_id OR
           receipt::jsonb->'actor'<>authority::jsonb->'actor' OR
           receipt::jsonb->>'purpose'<>'episode_invocation' OR
           NOT cognition_json_has_unique_keys(NEW.bootstrap_brain_json::json) OR
           NEW.bootstrap_brain_json<>
               cognition_canonical_jsonb(NEW.bootstrap_brain_json::jsonb) OR
           receipt::jsonb->'stable_brain'->'brain'<>
               NEW.bootstrap_brain_json::jsonb->'brain' OR
           receipt::jsonb->'stable_brain'->'provider_attestation'<>
               NEW.bootstrap_brain_json::jsonb->'provider_attestation' OR
           receipt::jsonb->'stable_brain'->'host_hardware_attestation'<>
               NEW.bootstrap_brain_json::jsonb->'host_hardware_attestation' OR
           NOT cognition_provider_identity_requests_match_brain(
               NEW.bootstrap_evidence_id,NEW.bootstrap_brain_json::jsonb->'brain'
           ) OR NOT cognition_provider_identity_evidence_matches_attempt(
               NEW.bootstrap_evidence_id,
               jsonb_build_object('brain',NEW.bootstrap_brain_json::jsonb->'brain')
           ) OR NOT cognition_provider_identity_observation_matches_evidence(
               cognition_canonical_jsonb(
                   NEW.bootstrap_brain_json::jsonb->'bootstrap_provider_observation'
               ),NEW.bootstrap_evidence_id,
               NEW.bootstrap_brain_json::jsonb->'provider_attestation'->>'attestation_sha256',
               cognition_provider_bootstrap_challenge(
                   NEW.bootstrap_brain_json::jsonb->'brain'
               )
           ) THEN
            RAISE EXCEPTION 'provider process failure receipt is inexact';
        END IF;
        brain := receipt::jsonb->'stable_brain'->'brain';
    END IF;
    code := receipt::jsonb->>'code';
    IF code NOT IN (
        'provider_identity_failed','provider_observation_invalid',
        'host_attestation_failed','host_identity_mismatch'
    ) OR NOT cognition_provider_identity_requests_match_brain(NEW.evidence_id,brain) THEN
        RAISE EXCEPTION 'provider activation failure evidence changed its exact request';
    END IF;
    IF code='provider_identity_failed' AND
       cognition_provider_identity_evidence_matches_attempt(
           NEW.evidence_id,jsonb_build_object('brain',brain)
       ) THEN
        RAISE EXCEPTION 'provider identity failure raw evidence proves success';
    ELSIF code<>'provider_identity_failed' AND NOT
       cognition_provider_identity_evidence_matches_attempt(
           NEW.evidence_id,jsonb_build_object('brain',brain)
       ) THEN
        RAISE EXCEPTION 'provider activation failure lacks successful provider evidence';
    END IF;
    IF NOT EXISTS (
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
        RAISE EXCEPTION 'provider activation failure lacks exact live attempt authority';
    END IF;
    SELECT claimed_at INTO attempt_claimed_at FROM job_step_attempts
    WHERE job_id=NEW.job_id AND generation=NEW.generation AND step_id=NEW.step_id
      AND attempt=NEW.step_attempt AND worker_id=NEW.worker_id;
    IF attempt_claimed_at IS NULL OR NEW.created_at<attempt_claimed_at OR
       NEW.created_at>clock_timestamp() THEN
        RAISE EXCEPTION 'provider activation failure timestamp is outside its attempt';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_provider_activation_failures_exact
AFTER INSERT ON cognition_provider_activation_failures DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_provider_activation_failure();
