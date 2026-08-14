CREATE OR REPLACE FUNCTION cognition_call_result_v3_shape_is_exact(result_text TEXT)
RETURNS BOOLEAN AS $$
DECLARE value JSON;
BEGIN
    value := result_text::json;
    RETURN json_typeof(value)='object' AND
       cognition_json_object_has_only_keys(value,ARRAY[
           'schema','call_id','status','provider_identity_checked','provider_attestation',
           'provider_observation','provider_identity_evidence','provider_request_disposition',
           'provider_request_sha256','provider_http_status','provider_response_disposition',
           'provider_response_complete','provider_content_encoding',
           'provider_response_bytes_known','provider_response_sha256','provider_response_bytes',
           'provider_response_capture_sha256','provider_response_captured_bytes',
           'provider_response_model','provider_done_present','provider_done','provider_done_reason',
           'provider_usage_present','provider_usage','provider_response_capture_evidence',
           'provider_generation_evidence','response_sha256','response_bytes','response_evidence',
           'action_schema','decision_sha256','failure_code','failure_message'
       ]) AND value::jsonb ?& ARRAY[
           'schema','call_id','status','provider_identity_checked','provider_attestation',
           'provider_observation','provider_identity_evidence','provider_request_disposition',
           'provider_http_status','provider_response_complete','provider_content_encoding',
           'provider_response_bytes_known','provider_response_bytes',
           'provider_response_captured_bytes','provider_response_model','provider_done_present',
           'provider_done','provider_done_reason','provider_usage_present','provider_usage',
           'provider_response_capture_evidence','provider_generation_evidence','response_bytes',
           'response_evidence','action_schema'
       ] AND cognition_json_object_has_exact_keys(value->'provider_attestation',ARRAY[
           'schema','backend','backend_version','model','digest','quantization',
           'native_context_limit','tokenizer_profile','backend_evidence','installed_evidence',
           'runner_evidence','attestation_sha256'
       ]) AND cognition_json_object_has_exact_keys(value->'provider_observation',ARRAY[
           'schema','observed_at','attestation_sha256','version_body_sha256',
           'installed_body_sha256','tokenizer_request_sha256','tokenizer_body_sha256',
           'preload_body_sha256','runner_body_sha256','preload_method','preload_endpoint',
           'preload_request_sha256','challenge_sha256','evidence','observation_sha256'
       ]) AND cognition_json_object_has_exact_keys(value->'provider_observation'->'evidence',ARRAY[
           'schema','id','sha256','bytes'
       ]) AND cognition_json_object_has_exact_keys(value->'provider_identity_evidence',ARRAY[
           'schema','id','sha256','bytes'
       ]) AND cognition_json_object_has_exact_keys(value->'provider_content_encoding',ARRAY[
           'schema','values','complete','sha256','bytes','captured_base64',
           'captured_bytes','uncompressed'
       ]) AND cognition_json_object_has_exact_keys(value->'provider_usage',ARRAY[
           'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
           'prompt_eval_duration_nanos','eval_duration_nanos'
       ]) AND cognition_json_object_has_exact_keys(
           value->'provider_response_capture_evidence',ARRAY['schema','id','sha256','bytes']
       ) AND cognition_json_object_has_exact_keys(
           value->'provider_generation_evidence',ARRAY['schema','id','sha256','bytes']
       ) AND cognition_json_object_has_exact_keys(
           value->'response_evidence',ARRAY['schema','id','sha256','bytes']
       ) AND cognition_json_object_has_exact_keys(
           value->'action_schema',ARRAY['id','version','sha256']
       );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_call_provider_challenge(
    call_identity TEXT,
    brain JSONB
) RETURNS TEXT AS $$
    SELECT encode(digest(cognition_canonical_jsonb(jsonb_build_object(
        'scope','cognition-policy-call:'||call_identity,
        'expectation',jsonb_build_object(
            'backend',brain->>'backend','backend_version',brain->>'backend_version',
            'model',brain->>'model','digest',brain->>'digest',
            'quantization',brain->>'quantization',
            'native_context_limit',(brain->>'native_context_limit')::BIGINT,
            'tokenizer_profile','ollama-0.24.0-qwen35-gpt2-boundary-v1'
        )
    )),'sha256'),'hex');
$$ LANGUAGE SQL IMMUTABLE STRICT;
