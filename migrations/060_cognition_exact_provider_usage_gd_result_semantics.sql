CREATE OR REPLACE FUNCTION cognition_policy_usage_is_successful(result JSONB)
RETURNS BOOLEAN AS $$
DECLARE usage JSONB := result->'provider_usage';
BEGIN
    RETURN (result->>'provider_usage_present')::BOOLEAN AND
           (usage->>'prompt_eval_count')::NUMERIC>0 AND
           (usage->>'eval_count')::NUMERIC>0 AND
           (usage->>'total_duration_nanos')::NUMERIC>0 AND
           (usage->>'load_duration_nanos')::NUMERIC>=0 AND
           (usage->>'prompt_eval_duration_nanos')::NUMERIC>0 AND
           (usage->>'eval_duration_nanos')::NUMERIC>0 AND
           (usage->>'total_duration_nanos')::NUMERIC>=
               (usage->>'load_duration_nanos')::NUMERIC+
               (usage->>'prompt_eval_duration_nanos')::NUMERIC+
               (usage->>'eval_duration_nanos')::NUMERIC;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_action_ref_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value::json,ARRAY['id','version','sha256']) AND
           jsonb_typeof(value->'id')='string' AND
           octet_length(value->>'id') BETWEEN 1 AND 128 AND
           value->>'id'~'^[A-Za-z0-9][A-Za-z0-9_.:/-]*$' AND
           jsonb_typeof(value->'version')='string' AND
           octet_length(value->>'version') BETWEEN 1 AND 64 AND
           value->>'version'~'^[A-Za-z0-9][A-Za-z0-9_.:/-]*$' AND
           jsonb_typeof(value->'sha256')='string' AND
           value->>'sha256'~'^[0-9a-f]{64}$';
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_response_identity_is_exact(result JSONB)
RETURNS BOOLEAN AS $$
DECLARE response_bytes BIGINT := (result->>'response_bytes')::BIGINT;
DECLARE reference JSONB := result->'response_evidence';
BEGIN
    IF response_bytes=0 THEN
        RETURN NOT (result ? 'response_sha256') AND
               cognition_policy_evidence_ref_is_zero(reference);
    END IF;
    RETURN result->>'response_sha256'~'^[0-9a-f]{64}$' AND
           reference->>'schema'='omnidex.cognition-model-response-evidence.v1' AND
           reference->>'sha256'=result->>'response_sha256' AND
           (reference->>'bytes')::BIGINT=response_bytes AND
           reference->>'id'~'^cognition_response_[0-9a-f]{64}$';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_final_provider_response(
    result JSONB,
    brain JSONB,
    expected_request_sha TEXT
) RETURNS BOOLEAN AS $$
    SELECT (result->>'provider_identity_checked')::BOOLEAN AND
           result->>'provider_request_disposition'='dispatched' AND
           result->>'provider_request_sha256'=expected_request_sha AND
           result->>'provider_response_disposition'='succeeded' AND
           (result->>'provider_response_complete')::BOOLEAN AND
           (result->>'provider_response_bytes_known')::BOOLEAN AND
           result->>'provider_response_sha256'~'^[0-9a-f]{64}$' AND
           result->>'provider_response_model'=brain->>'model' AND
           (result->>'provider_done_present')::BOOLEAN AND
           (result->>'provider_done')::BOOLEAN AND
           (result->>'response_bytes')::BIGINT>0 AND
           cognition_provider_content_encoding_is_identity(
               cognition_canonical_jsonb(result->'provider_content_encoding')
           ) AND cognition_policy_usage_is_successful(result) AND
           cognition_policy_response_identity_is_exact(result);
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_terminal_result_is_exact(
    result JSONB,
    budget JSONB,
    brain JSONB,
    expected_request_sha TEXT
) RETURNS BOOLEAN AS $$
DECLARE status_value TEXT := result->>'status';
DECLARE code_value TEXT := COALESCE(result->>'failure_code','');
DECLARE prompt_count BIGINT := (result->'provider_usage'->>'prompt_eval_count')::BIGINT;
DECLARE eval_count_value BIGINT := (result->'provider_usage'->>'eval_count')::BIGINT;
DECLARE input_limit BIGINT := (budget->>'max_input_tokens')::BIGINT;
DECLARE output_limit BIGINT := (budget->>'max_output_tokens')::BIGINT;
DECLARE output_bytes BIGINT := (budget->>'max_output_bytes')::BIGINT;
DECLARE final_response BOOLEAN;
DECLARE generation_evidence_zero BOOLEAN := cognition_policy_evidence_ref_is_zero(
    result->'provider_generation_evidence'
);
DECLARE identity_evidence_zero BOOLEAN := cognition_policy_evidence_ref_is_zero(
    result->'provider_identity_evidence'
);
DECLARE zero_provider_values BOOLEAN;
BEGIN
    IF status_value NOT IN ('accepted','rejected','failed') OR
       status_value<>'accepted' AND (
        result->'action_schema'<>'{"id":"","version":"","sha256":""}'::jsonb OR
        result ? 'decision_sha256' OR code_value='' OR
        NOT (result ? 'failure_message') OR
        octet_length(result->>'failure_message') NOT BETWEEN 1 AND 4096
    ) THEN
        RETURN FALSE;
    END IF;
    IF status_value='accepted' AND (
        result ? 'failure_code' OR result ? 'failure_message'
    ) THEN
        RETURN FALSE;
    END IF;
    IF NOT cognition_policy_response_identity_is_exact(result) THEN
        RETURN FALSE;
    END IF;
    zero_provider_values := NOT (result ? 'provider_request_sha256') AND
        (result->>'provider_http_status')::BIGINT=0 AND
        NOT (result ? 'provider_response_disposition') AND
        NOT (result->>'provider_response_complete')::BOOLEAN AND
        result->'provider_content_encoding'='{
          "schema":"","values":0,"complete":false,"sha256":"","bytes":0,
          "captured_base64":"","captured_bytes":0,"uncompressed":false
        }'::jsonb AND NOT (result->>'provider_response_bytes_known')::BOOLEAN AND
        NOT (result ? 'provider_response_sha256') AND
        (result->>'provider_response_bytes')::BIGINT=0 AND
        NOT (result ? 'provider_response_capture_sha256') AND
        (result->>'provider_response_captured_bytes')::BIGINT=0 AND
        result->>'provider_response_model'='' AND
        NOT (result->>'provider_done_present')::BOOLEAN AND
        NOT (result->>'provider_done')::BOOLEAN AND result->>'provider_done_reason'='' AND
        NOT (result->>'provider_usage_present')::BOOLEAN AND
        result->'provider_usage'='{
          "prompt_eval_count":0,"eval_count":0,"total_duration_nanos":0,
          "load_duration_nanos":0,"prompt_eval_duration_nanos":0,
          "eval_duration_nanos":0
        }'::jsonb;
    IF (result->>'provider_identity_checked')::BOOLEAN THEN
        IF NOT generation_evidence_zero OR identity_evidence_zero OR
           NOT cognition_policy_provider_receipt_is_exact(result,brain) OR
           ((result->>'provider_request_sha256'<>expected_request_sha) <>
               (status_value='failed' AND code_value='provider_request_mismatch')) THEN
            RETURN FALSE;
        END IF;
    ELSIF result->'provider_attestation'<>
          '{
            "schema":"","backend":"","backend_version":"","model":"","digest":"",
            "quantization":"","native_context_limit":0,"tokenizer_profile":"",
            "backend_evidence":"","installed_evidence":"","runner_evidence":"",
            "attestation_sha256":""
          }'::jsonb OR result->'provider_observation'<>
          '{
            "schema":"","observed_at":"0001-01-01T00:00:00Z",
            "attestation_sha256":"","version_body_sha256":"",
            "installed_body_sha256":"","tokenizer_request_sha256":"",
            "tokenizer_body_sha256":"","preload_body_sha256":"",
            "runner_body_sha256":"","preload_method":"","preload_endpoint":"",
            "preload_request_sha256":"","challenge_sha256":"",
            "evidence":{"schema":"","id":"","sha256":"","bytes":0},
            "observation_sha256":""
          }'::jsonb OR NOT zero_provider_values THEN
        RETURN FALSE;
    END IF;
    IF NOT (result->>'provider_identity_checked')::BOOLEAN AND
       generation_evidence_zero AND (
           (code_value='provider_identity_error' AND identity_evidence_zero) OR
           (code_value<>'provider_identity_error' AND NOT identity_evidence_zero) OR
           NOT cognition_policy_evidence_ref_is_zero(
               result->'provider_response_capture_evidence'
           )
       ) THEN
        RETURN FALSE;
    END IF;
    final_response := cognition_policy_final_provider_response(
        result,brain,expected_request_sha
    );
    IF status_value='accepted' THEN
        RETURN code_value='' AND NOT (result ? 'failure_message') AND
               final_response AND result->>'provider_done_reason'='stop' AND
               prompt_count<=input_limit AND eval_count_value<=output_limit AND
               (result->>'response_bytes')::BIGINT<=output_bytes AND
               cognition_policy_action_ref_is_exact(result->'action_schema') AND
               result->>'decision_sha256'~'^[0-9a-f]{64}$';
    ELSIF status_value='rejected' THEN
        IF code_value='response_limit' THEN
            RETURN final_response AND prompt_count<=input_limit AND
                eval_count_value<=output_limit AND
                ((result->>'provider_done_reason'='stop' AND
                  (result->>'response_bytes')::BIGINT>output_bytes) OR
                 (result->>'provider_done_reason'='length' AND
                  eval_count_value=output_limit));
        ELSIF code_value IN ('invalid_decision','authority_denied') THEN
            RETURN final_response AND result->>'provider_done_reason'='stop' AND
                   prompt_count<=input_limit AND eval_count_value<=output_limit;
        ELSIF code_value='provider_usage_limit' THEN
            RETURN final_response AND result->>'provider_done_reason' IN ('stop','length') AND
                   (prompt_count>input_limit OR eval_count_value>output_limit);
        ELSIF code_value='provider_usage_error' THEN
            RETURN (result->>'provider_identity_checked')::BOOLEAN AND
                   result->>'provider_response_disposition'='succeeded' AND
                   (result->>'response_bytes')::BIGINT>0 AND NOT (
                       (result->>'provider_done_present')::BOOLEAN AND
                       (result->>'provider_done')::BOOLEAN AND
                       result->>'provider_done_reason' IN ('stop','length') AND
                       cognition_policy_usage_is_successful(result) AND NOT (
                           result->>'provider_done_reason'='length' AND
                           eval_count_value<>output_limit
                       )
                   );
        END IF;
        RETURN FALSE;
    END IF;
    IF code_value='provider_identity_error' THEN
        RETURN generation_evidence_zero AND NOT identity_evidence_zero AND
               result->>'provider_request_disposition'='not_dispatched';
    ELSIF code_value='provider_evidence_invalid' THEN
        RETURN NOT generation_evidence_zero AND
               result->>'provider_request_disposition' IN (
                   '','not_dispatched','dispatched','write_indeterminate'
               );
    ELSIF code_value='provider_request_mismatch' THEN
        RETURN (NOT generation_evidence_zero AND
                result->>'provider_request_disposition'='dispatched') OR
               (generation_evidence_zero AND
                (result->>'provider_identity_checked')::BOOLEAN);
    ELSIF code_value='policy_authority_error' THEN
        RETURN (NOT generation_evidence_zero AND
                result->>'provider_request_disposition'='not_dispatched') OR
               generation_evidence_zero;
    ELSIF code_value='generation_error' THEN
        RETURN generation_evidence_zero AND
               (result->>'provider_identity_checked')::BOOLEAN AND
               result->>'provider_response_disposition'<>'succeeded';
    END IF;
    RETURN FALSE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
