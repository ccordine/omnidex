CREATE OR REPLACE FUNCTION require_exact_cognition_provider_activation_failure()
RETURNS TRIGGER AS $$
DECLARE receipt JSONB := NEW.receipt_json::jsonb;
DECLARE authority JSONB := NEW.authority_json::jsonb;
DECLARE brain JSONB;
DECLARE challenge TEXT;
DECLARE code TEXT;
DECLARE evidence_ref JSONB;
DECLARE attempt_claimed_at TIMESTAMPTZ;
BEGIN
    SELECT ref_json::jsonb INTO evidence_ref FROM cognition_provider_identity_evidence
    WHERE evidence_id=NEW.evidence_id;
    IF evidence_ref IS NULL OR NOT cognition_json_has_unique_keys(NEW.receipt_json::json) OR
       NEW.receipt_json<>cognition_canonical_jsonb(receipt) OR
       NOT cognition_json_has_unique_keys(NEW.authority_json::json) OR
       NEW.authority_json<>cognition_canonical_jsonb(authority) OR
       NOT cognition_json_object_has_exact_keys(authority::json,ARRAY[
           'schema','record_id','failure_kind','failure_id','episode_id','actor',
           'evidence_id','receipt_sha256','bootstrap_evidence_id','bootstrap_brain_sha256'
       ]) OR NOT cognition_attempt_ref_is_exact(authority->'actor') OR
       jsonb_typeof(authority->'schema')<>'string' OR
       authority->>'schema'<>'omnidex.cognition-provider-failure-authority.v1' OR
       authority->>'record_id'<>NEW.record_id OR
       authority->>'failure_kind'<>NEW.failure_kind OR
       authority->>'failure_id'<>NEW.failure_id OR authority->>'episode_id'<>NEW.episode_id OR
       authority->>'evidence_id'<>NEW.evidence_id OR
       authority->>'receipt_sha256'<>NEW.receipt_sha256 OR
       authority->>'bootstrap_evidence_id'<>COALESCE(NEW.bootstrap_evidence_id,'') OR
       authority->>'bootstrap_brain_sha256'<>COALESCE(NEW.bootstrap_brain_sha256,'') OR
       authority->'actor'<>jsonb_build_object(
           'job_id',NEW.job_id,'generation',NEW.generation,'step_id',NEW.step_id,
           'attempt',NEW.step_attempt,'worker_id',NEW.worker_id
       ) OR NEW.record_id<>'cognition_provider_failure_'||encode(digest(
           cognition_canonical_jsonb(jsonb_set(
               authority,'{record_id}',to_jsonb(''::TEXT)
           )),'sha256'
       ),'hex') OR receipt->>'id'<>NEW.failure_id OR receipt->'evidence'<>evidence_ref OR
       NEW.failure_id<>(CASE NEW.failure_kind
           WHEN 'brain_bootstrap' THEN 'brain_bootstrap_failure_'
           ELSE 'provider_process_failure_'
       END||encode(digest(cognition_canonical_jsonb(jsonb_set(
           receipt,'{id}',to_jsonb(''::TEXT)
       )),'sha256'),'hex')) THEN
        RAISE EXCEPTION 'provider activation failure authority is inexact';
    END IF;

    IF NEW.failure_kind='brain_bootstrap' THEN
        IF NOT cognition_json_object_has_exact_keys(receipt::json,ARRAY[
            'schema','id','brain','challenge_sha256','code','provider_attestation',
            'provider_observation','evidence'
        ]) OR jsonb_typeof(receipt->'schema')<>'string' OR
           receipt->>'schema'<>'omnidex.brain-bootstrap-failure.v1' OR
           jsonb_typeof(receipt->'id')<>'string' OR
           jsonb_typeof(receipt->'challenge_sha256')<>'string' OR
           jsonb_typeof(receipt->'code')<>'string' OR
           NOT cognition_brain_ref_is_exact(receipt->'brain') OR
           NOT cognition_provider_attestation_shape_is_bounded(
               receipt->'provider_attestation'
           ) OR NOT cognition_provider_observation_shape_is_bounded(
               receipt->'provider_observation'
           ) OR NOT cognition_provider_evidence_ref_is_exact(receipt->'evidence') THEN
            RAISE EXCEPTION 'Brain bootstrap failure receipt is inexact';
        END IF;
        brain := receipt->'brain';
        challenge := cognition_provider_bootstrap_challenge(brain);
        IF receipt->>'challenge_sha256'<>challenge OR
           receipt->>'code' IN ('provider_attestation_mismatch','host_identity_mismatch') THEN
            RAISE EXCEPTION 'Brain bootstrap failure proof is inexact';
        END IF;
    ELSE
        IF NOT cognition_json_object_has_exact_keys(receipt::json,ARRAY[
            'schema','id','episode_id','actor','purpose','stable_brain','code',
            'provider_attestation','provider_observation',
            'live_host_hardware_attestation','evidence'
        ]) OR jsonb_typeof(receipt->'schema')<>'string' OR
           receipt->>'schema'<>'omnidex.provider-process-failure.v1' OR
           jsonb_typeof(receipt->'id')<>'string' OR
           jsonb_typeof(receipt->'episode_id')<>'string' OR
           receipt->>'episode_id'<>NEW.episode_id OR
           NOT cognition_attempt_ref_is_exact(receipt->'actor') OR
           receipt->'actor'<>authority->'actor' OR
           jsonb_typeof(receipt->'purpose')<>'string' OR
           receipt->>'purpose'<>'episode_invocation' OR
           jsonb_typeof(receipt->'code')<>'string' OR
           NOT cognition_stable_brain_is_exact(receipt->'stable_brain') OR
           NOT cognition_provider_attestation_shape_is_bounded(
               receipt->'provider_attestation'
           ) OR NOT cognition_provider_observation_shape_is_bounded(
               receipt->'provider_observation'
           ) OR NOT cognition_host_attestation_shape_is_bounded(
               receipt->'live_host_hardware_attestation'
           ) OR NOT cognition_provider_evidence_ref_is_exact(receipt->'evidence') OR
           NOT cognition_json_has_unique_keys(NEW.bootstrap_brain_json::json) OR
           NEW.bootstrap_brain_json<>cognition_canonical_jsonb(
               NEW.bootstrap_brain_json::jsonb
           ) OR NOT cognition_attested_brain_is_exact(NEW.bootstrap_brain_json::jsonb) OR
           receipt->'stable_brain'->'brain'<>NEW.bootstrap_brain_json::jsonb->'brain' OR
           receipt->'stable_brain'->'provider_attestation'<>
               NEW.bootstrap_brain_json::jsonb->'provider_attestation' OR
           receipt->'stable_brain'->'host_hardware_attestation'<>
               NEW.bootstrap_brain_json::jsonb->'host_hardware_attestation' OR
           NOT cognition_provider_observed_identity_is_exact(
               NEW.bootstrap_brain_json::jsonb->'provider_attestation',
               NEW.bootstrap_brain_json::jsonb->'bootstrap_provider_observation',
               NEW.bootstrap_brain_json::jsonb->'brain',
               cognition_provider_bootstrap_challenge(
                   NEW.bootstrap_brain_json::jsonb->'brain'
               ),NEW.bootstrap_evidence_id
           ) THEN
            RAISE EXCEPTION 'provider process failure receipt is inexact';
        END IF;
        brain := receipt->'stable_brain'->'brain';
        challenge := cognition_provider_process_challenge(
            receipt->'stable_brain',NEW.episode_id,receipt->'actor',receipt->>'purpose'
        );
    END IF;

    code := receipt->>'code';
    IF NOT cognition_provider_failure_code_is_exact(
        code,receipt->'provider_attestation',receipt->'provider_observation',
        brain,challenge,NEW.evidence_id
    ) THEN
        RAISE EXCEPTION 'provider activation failure code is not proven';
    END IF;
    IF NEW.failure_kind='provider_process' AND (
       (code='provider_attestation_mismatch' AND (
          receipt->'provider_attestation'=receipt->'stable_brain'->'provider_attestation' OR
          receipt->'live_host_hardware_attestation'<>cognition_empty_host_attestation()
       )) OR (code='host_attestation_failed' AND (
          receipt->'provider_attestation'<>receipt->'stable_brain'->'provider_attestation' OR
          receipt->'live_host_hardware_attestation'<>cognition_empty_host_attestation()
       )) OR (code='host_identity_mismatch' AND (
          receipt->'provider_attestation'<>receipt->'stable_brain'->'provider_attestation' OR
          NOT cognition_host_attestation_is_exact(
              receipt->'live_host_hardware_attestation'
          ) OR receipt->'live_host_hardware_attestation'=
              receipt->'stable_brain'->'host_hardware_attestation'
       )) OR (code NOT IN (
          'provider_attestation_mismatch','host_attestation_failed','host_identity_mismatch'
       ) AND receipt->'live_host_hardware_attestation'<>cognition_empty_host_attestation())
    ) THEN
        RAISE EXCEPTION 'provider process failure live identity proof is inexact';
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
