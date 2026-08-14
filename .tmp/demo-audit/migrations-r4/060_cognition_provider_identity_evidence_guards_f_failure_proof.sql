CREATE OR REPLACE FUNCTION cognition_attempt_ref_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value::json,ARRAY[
               'job_id','generation','step_id','attempt','worker_id'
           ]) AND cognition_exact_json_positive_integer(value->'job_id',9223372036854775807) AND
           cognition_exact_json_positive_integer(value->'generation',9223372036854775807) AND
           cognition_exact_json_positive_integer(value->'step_id',9223372036854775807) AND
           cognition_exact_json_positive_integer(value->'attempt',9223372036854775807) AND
           jsonb_typeof(value->'worker_id')='string' AND
           task_ledger_text_is_exact(value->>'worker_id');
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_empty_provider_attestation()
RETURNS JSONB AS $$
    SELECT jsonb_build_object(
        'schema','','backend','','backend_version','','model','','digest','',
        'quantization','','native_context_limit',0,'tokenizer_profile','',
        'backend_evidence','','installed_evidence','','runner_evidence','',
        'attestation_sha256',''
    );
$$ LANGUAGE SQL IMMUTABLE;

CREATE OR REPLACE FUNCTION cognition_empty_provider_evidence_ref()
RETURNS JSONB AS $$
    SELECT jsonb_build_object('schema','','id','','sha256','','bytes',0);
$$ LANGUAGE SQL IMMUTABLE;

CREATE OR REPLACE FUNCTION cognition_empty_provider_observation()
RETURNS JSONB AS $$
    SELECT jsonb_build_object(
        'schema','','observed_at','0001-01-01T00:00:00Z','attestation_sha256','',
        'version_body_sha256','','installed_body_sha256','',
        'tokenizer_request_sha256','','tokenizer_body_sha256','',
        'preload_body_sha256','','runner_body_sha256','','preload_method','',
        'preload_endpoint','','preload_request_sha256','','challenge_sha256','',
        'evidence',cognition_empty_provider_evidence_ref(),'observation_sha256',''
    );
$$ LANGUAGE SQL IMMUTABLE;

CREATE OR REPLACE FUNCTION cognition_empty_host_attestation()
RETURNS JSONB AS $$
    SELECT jsonb_build_object(
        'schema','','os','','architecture','','logical_cpus',0,
        'cpu_identity_sha256','','accelerator_identity_sha256','',
        'cpu_evidence','','accelerator_evidence','','attestation_sha256',''
    );
$$ LANGUAGE SQL IMMUTABLE;

CREATE OR REPLACE FUNCTION cognition_provider_failure_proof_is_bounded(
    attestation JSONB,
    observation JSONB
) RETURNS BOOLEAN AS $$
DECLARE total_bytes BIGINT;
BEGIN
    IF NOT cognition_provider_attestation_shape_is_bounded(attestation) OR
       NOT cognition_provider_observation_shape_is_bounded(observation) THEN
        RETURN FALSE;
    END IF;
    SELECT COALESCE(sum(octet_length(field.value#>>'{}')),0)
      INTO total_bytes FROM jsonb_each(attestation) field
      WHERE jsonb_typeof(field.value)='string';
    SELECT total_bytes+COALESCE(sum(octet_length(field.value#>>'{}')),0)
      INTO total_bytes FROM jsonb_each(observation) field
      WHERE jsonb_typeof(field.value)='string';
    total_bytes := total_bytes+
        octet_length(observation->'evidence'->>'schema')+
        octet_length(observation->'evidence'->>'id')+
        octet_length(observation->'evidence'->>'sha256');
    RETURN total_bytes<=65536;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_observed_identity_is_exact(
    attestation JSONB,
    observation JSONB,
    brain JSONB,
    challenge_sha TEXT,
    evidence_identity TEXT
) RETURNS BOOLEAN AS $$
DECLARE evidence_ref JSONB;
BEGIN
    SELECT ref_json::jsonb INTO evidence_ref
    FROM cognition_provider_identity_evidence WHERE evidence_id=evidence_identity;
    RETURN evidence_ref IS NOT NULL AND
           cognition_provider_attestation_matches_brain(attestation,brain) AND
           cognition_provider_observation_is_exact(
               observation,attestation->>'attestation_sha256',challenge_sha
           ) AND observation->'evidence'=evidence_ref AND
           cognition_provider_identity_requests_match_brain(evidence_identity,brain) AND
           cognition_provider_identity_evidence_matches_attempt(
               evidence_identity,jsonb_build_object('brain',brain)
           ) AND cognition_provider_identity_observation_matches_evidence(
               cognition_canonical_jsonb(observation),evidence_identity,
               attestation->>'attestation_sha256',challenge_sha
           );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_evidence_proves_failure(
    evidence_identity TEXT,
    brain JSONB
) RETURNS BOOLEAN AS $$
DECLARE failed cognition_provider_identity_evidence_operations%ROWTYPE;
DECLARE response_document JSON;
BEGIN
    IF NOT cognition_provider_identity_requests_match_brain(evidence_identity,brain) THEN
        RETURN FALSE;
    END IF;
    SELECT * INTO failed FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND disposition<>'succeeded'
    ORDER BY operation_index LIMIT 1;
    IF failed.evidence_id IS NULL THEN
        RETURN NOT cognition_provider_identity_evidence_matches_attempt(
            evidence_identity,jsonb_build_object('brain',brain)
        );
    END IF;
    IF failed.disposition='invalid_json' AND
       cognition_provider_content_encoding_is_identity(failed.content_encoding_json) THEN
        BEGIN
            response_document := convert_from(failed.response_body,'UTF8')::json;
            IF json_typeof(response_document)='object' AND
               cognition_json_has_unique_keys(response_document) THEN
                RETURN FALSE;
            END IF;
        EXCEPTION WHEN OTHERS THEN
            RETURN TRUE;
        END;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_failure_code_is_exact(
    code TEXT,
    attestation JSONB,
    observation JSONB,
    brain JSONB,
    challenge_sha TEXT,
    evidence_identity TEXT
) RETURNS BOOLEAN AS $$
DECLARE empty_proof BOOLEAN;
DECLARE successful BOOLEAN;
DECLARE observed_exact BOOLEAN;
BEGIN
    empty_proof := attestation=cognition_empty_provider_attestation() AND
                   observation=cognition_empty_provider_observation();
    successful := cognition_provider_identity_requests_match_brain(
                      evidence_identity,brain
                  ) AND cognition_provider_identity_evidence_matches_attempt(
                      evidence_identity,jsonb_build_object('brain',brain)
                  );
    observed_exact := cognition_provider_observed_identity_is_exact(
        attestation,observation,brain,challenge_sha,evidence_identity
    );
    CASE code
    WHEN 'provider_identity_failed' THEN
        RETURN empty_proof AND cognition_provider_identity_evidence_proves_failure(
            evidence_identity,brain
        );
    WHEN 'provider_observation_invalid' THEN
        RETURN NOT empty_proof AND successful AND
               cognition_provider_failure_proof_is_bounded(attestation,observation) AND
               NOT observed_exact;
    WHEN 'provider_attestation_mismatch','host_attestation_failed','host_identity_mismatch' THEN
        RETURN observed_exact;
    ELSE
        RETURN FALSE;
    END CASE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_process_challenge(
    stable JSONB,
    episode_id TEXT,
    actor JSONB,
    purpose TEXT
) RETURNS TEXT AS $$
    SELECT encode(digest(cognition_canonical_jsonb(jsonb_build_object(
        'scope','cognition-process:'||encode(digest(cognition_canonical_jsonb(
            jsonb_build_object(
                'episode_id',episode_id,'actor',actor,'purpose',purpose,
                'stable_brain_sha256',stable->>'sha256'
            )
        ),'sha256'),'hex'),
        'expectation',jsonb_build_object(
            'backend',stable->'brain'->>'backend',
            'backend_version',stable->'brain'->>'backend_version',
            'model',stable->'brain'->>'model','digest',stable->'brain'->>'digest',
            'quantization',stable->'brain'->>'quantization',
            'native_context_limit',(stable->'brain'->>'native_context_limit')::BIGINT,
            'tokenizer_profile','ollama-0.24.0-qwen35-gpt2-boundary-v1'
        )
    )),'sha256'),'hex');
$$ LANGUAGE SQL IMMUTABLE STRICT;
