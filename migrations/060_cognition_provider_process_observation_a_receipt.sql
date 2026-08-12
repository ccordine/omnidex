CREATE OR REPLACE FUNCTION cognition_provider_process_receipt_is_exact(
    receipt_json TEXT,
    observation_id TEXT,
    evidence_id TEXT,
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
DECLARE receipt JSONB := receipt_json::jsonb;
DECLARE stable JSONB := stable_brain_json::jsonb;
DECLARE observation JSONB := provider_observation_json::jsonb;
DECLARE evidence_ref JSONB;
BEGIN
    SELECT ref_json::jsonb INTO evidence_ref
    FROM cognition_provider_identity_evidence WHERE cognition_provider_identity_evidence.evidence_id=$3;
    IF NOT cognition_json_has_unique_keys(receipt_json::json) OR
       NOT cognition_json_has_unique_keys(stable_brain_json::json) OR
       NOT cognition_json_has_unique_keys(provider_observation_json::json) OR
       receipt_json<>cognition_canonical_jsonb(receipt) OR
       stable_brain_json<>cognition_canonical_jsonb(stable) OR
       provider_observation_json<>cognition_canonical_jsonb(observation) OR
       NOT cognition_json_object_has_exact_keys(receipt_json::json,ARRAY[
           'schema','id','episode_id','actor','purpose','stable_brain','observation'
       ]) OR NOT cognition_json_object_has_exact_keys(
           (receipt_json::json->'actor')::json,
           ARRAY['job_id','generation','step_id','attempt','worker_id']
       ) OR NOT cognition_json_object_has_exact_keys(
           (receipt_json::json->'stable_brain')::json,
           ARRAY['schema','brain','provider_attestation','host_hardware_attestation','sha256']
       ) OR NOT cognition_json_object_has_exact_keys(
           (receipt_json::json->'observation')::json,
           ARRAY['schema','observed_at','attestation_sha256','version_body_sha256',
                 'installed_body_sha256','tokenizer_request_sha256','tokenizer_body_sha256',
                 'preload_body_sha256','runner_body_sha256','preload_method','preload_endpoint',
                 'preload_request_sha256','challenge_sha256','evidence','observation_sha256']
       ) THEN
        RAISE EXCEPTION 'provider process receipt JSON shape is inexact';
    END IF;
    IF receipt->>'schema'<>'omnidex.provider-process-observation.v1' OR
       receipt->>'id'<>observation_id OR receipt->>'episode_id'<>episode_id OR
       receipt->>'purpose'<>purpose OR
       (receipt->'actor'->>'job_id')::BIGINT<>job_id OR
       (receipt->'actor'->>'generation')::BIGINT<>generation OR
       (receipt->'actor'->>'step_id')::BIGINT<>step_id OR
       (receipt->'actor'->>'attempt')::BIGINT<>step_attempt OR
       receipt->'actor'->>'worker_id'<>worker_id OR receipt->'stable_brain'<>stable OR
       receipt->'observation'<>observation THEN
        RAISE EXCEPTION 'provider process receipt projection is inexact';
    END IF;
    IF evidence_ref IS NULL OR NOT cognition_stable_brain_is_exact(stable) OR
       NOT cognition_provider_observed_identity_is_exact(
           stable->'provider_attestation',observation,stable->'brain',
           challenge_sha256,evidence_id
       ) OR observation->'evidence'<>evidence_ref OR
       stable->>'sha256'<>stable_brain_sha256 OR
       observation->>'observation_sha256'<>provider_observation_sha256 OR
       stable->'provider_attestation'->>'attestation_sha256'<>provider_attestation_sha256 OR
       observation->>'attestation_sha256'<>provider_attestation_sha256 OR
       observation->>'challenge_sha256'<>challenge_sha256 OR
       observation->>'observed_at' !~ 'Z$' OR
       (observation->>'observed_at')::TIMESTAMPTZ<>observed_at THEN
        RAISE EXCEPTION 'provider process receipt evidence projection is inexact';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_each_text(observation) fields
        WHERE fields.key IN (
            'attestation_sha256','version_body_sha256','installed_body_sha256',
            'tokenizer_request_sha256','tokenizer_body_sha256','preload_body_sha256',
            'runner_body_sha256','preload_request_sha256','challenge_sha256','observation_sha256'
        ) AND fields.value !~ '^[0-9a-f]{64}$'
    ) THEN
        RAISE EXCEPTION 'provider process receipt contains an invalid digest';
    END IF;
    IF stable_brain_sha256<>encode(digest(cognition_canonical_jsonb(jsonb_set(
           stable,'{sha256}',to_jsonb(''::TEXT)
       )),'sha256'),'hex') OR provider_observation_sha256<>encode(digest(
           cognition_canonical_jsonb(jsonb_set(
               observation,'{observation_sha256}',to_jsonb(''::TEXT)
           )),'sha256'
       ),'hex') THEN
        RAISE EXCEPTION 'provider process receipt contains an invalid self-hash';
    END IF;
    IF challenge_sha256<>encode(digest(cognition_canonical_jsonb(jsonb_build_object(
       'scope','cognition-process:'||encode(digest(cognition_canonical_jsonb(jsonb_build_object(
           'episode_id',episode_id,'actor',receipt->'actor','purpose',purpose,
           'stable_brain_sha256',stable_brain_sha256
       )),'sha256'),'hex'),
       'expectation',jsonb_build_object(
           'backend',stable->'brain'->>'backend',
           'backend_version',stable->'brain'->>'backend_version',
           'model',stable->'brain'->>'model','digest',stable->'brain'->>'digest',
           'quantization',stable->'brain'->>'quantization',
           'native_context_limit',(stable->'brain'->>'native_context_limit')::BIGINT,
           'tokenizer_profile','ollama-0.24.0-qwen35-gpt2-boundary-v1'
       )
    )),'sha256'),'hex') THEN
        RAISE EXCEPTION 'provider process receipt challenge is inexact';
    END IF;
    IF observation_id<>'provider_process_observation_'||encode(digest(
       cognition_canonical_jsonb(jsonb_set(receipt,'{id}',to_jsonb(''::TEXT))),
       'sha256'
    ),'hex') THEN
        RAISE EXCEPTION 'provider process receipt identity is inexact';
    END IF;
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;
