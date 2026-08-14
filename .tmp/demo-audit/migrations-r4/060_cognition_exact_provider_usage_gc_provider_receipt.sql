CREATE OR REPLACE FUNCTION cognition_policy_evidence_ref_is_zero(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT value='{"schema":"","id":"","sha256":"","bytes":0}'::jsonb;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_response_capture_ref_is_exact(result JSONB)
RETURNS BOOLEAN AS $$
DECLARE reference JSONB := result->'provider_response_capture_evidence';
BEGIN
    RETURN reference->>'schema'='omnidex.provider-response-capture-evidence.v1' AND
           reference->>'sha256'=result->>'provider_response_capture_sha256' AND
           (reference->>'bytes')::BIGINT=
               (result->>'provider_response_captured_bytes')::BIGINT AND
           reference->>'id'='provider_response_capture_'||encode(digest(
               cognition_canonical_jsonb(jsonb_build_object(
                   'call_id',result->>'call_id',
                   'ref',jsonb_set(reference,'{id}','""'::jsonb,false)
               )),'sha256'
           ),'hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_policy_provider_receipt_is_exact(
    result JSONB,
    brain JSONB
)
RETURNS BOOLEAN AS $$
DECLARE request_disposition TEXT := result->>'provider_request_disposition';
DECLARE response_disposition TEXT := COALESCE(result->>'provider_response_disposition','');
DECLARE status_code BIGINT := (result->>'provider_http_status')::BIGINT;
DECLARE response_complete BOOLEAN := (result->>'provider_response_complete')::BOOLEAN;
DECLARE response_bytes_known BOOLEAN := (result->>'provider_response_bytes_known')::BOOLEAN;
DECLARE response_bytes BIGINT := (result->>'provider_response_bytes')::BIGINT;
DECLARE captured_bytes BIGINT := (result->>'provider_response_captured_bytes')::BIGINT;
DECLARE parsed_final BOOLEAN;
BEGIN
    IF request_disposition NOT IN (
        'not_dispatched','dispatched','write_indeterminate'
    ) OR COALESCE(result->>'provider_request_sha256','') !~ '^[0-9a-f]{64}$' THEN
        RETURN FALSE;
    END IF;
    IF response_disposition='transport_error' THEN
        RETURN status_code=0 AND NOT response_complete AND NOT response_bytes_known AND
               cognition_policy_evidence_ref_is_zero(
                   result->'provider_response_capture_evidence'
               ) AND
               result->'provider_content_encoding'='{
                 "schema":"","values":0,"complete":false,"sha256":"","bytes":0,
                 "captured_base64":"","captured_bytes":0,"uncompressed":false
               }'::jsonb AND NOT (result ? 'provider_response_sha256') AND
               response_bytes=0 AND NOT (result ? 'provider_response_capture_sha256') AND
               captured_bytes=0 AND result->>'provider_response_model'='' AND
               NOT (result->>'provider_done_present')::BOOLEAN AND
               NOT (result->>'provider_done')::BOOLEAN AND
               result->>'provider_done_reason'='' AND
               NOT (result->>'provider_usage_present')::BOOLEAN AND
               result->'provider_usage'='{
                 "prompt_eval_count":0,"eval_count":0,"total_duration_nanos":0,
                 "load_duration_nanos":0,"prompt_eval_duration_nanos":0,
                 "eval_duration_nanos":0
               }'::jsonb;
    END IF;
    IF request_disposition<>'dispatched' OR response_disposition NOT IN (
        'succeeded','http_error','body_limit','body_read_error','invalid_json','empty_content'
    ) OR status_code NOT BETWEEN 100 AND 599 OR captured_bytes NOT BETWEEN 0 AND 16777217 OR
       COALESCE(result->>'provider_response_capture_sha256','') !~ '^[0-9a-f]{64}$' OR
       NOT cognition_policy_response_capture_ref_is_exact(result) OR
       NOT cognition_provider_content_encoding_is_exact(
           cognition_canonical_jsonb(result->'provider_content_encoding')
       ) THEN
        RETURN FALSE;
    END IF;
    IF response_complete THEN
        IF response_disposition IN ('body_limit','body_read_error') OR
           NOT response_bytes_known OR
           COALESCE(result->>'provider_response_sha256','') !~ '^[0-9a-f]{64}$' OR
           captured_bytes>16777216 OR response_bytes<>captured_bytes OR
           result->>'provider_response_sha256'<>
               result->>'provider_response_capture_sha256' THEN
            RETURN FALSE;
        END IF;
    ELSIF response_bytes_known OR response_bytes<>0 OR
          result ? 'provider_response_sha256' OR
          response_disposition NOT IN ('body_limit','body_read_error') OR
          (response_disposition='body_limit' AND captured_bytes<>16777217) THEN
        RETURN FALSE;
    END IF;
    parsed_final := response_disposition IN ('succeeded','empty_content');
    IF parsed_final THEN
        RETURN status_code BETWEEN 200 AND 299 AND
               cognition_provider_content_encoding_is_identity(
                   cognition_canonical_jsonb(result->'provider_content_encoding')
               ) AND response_complete AND result->>'provider_response_model'=brain->>'model' AND
               ((result->>'provider_done_present')::BOOLEAN OR
                   NOT (result->>'provider_done')::BOOLEAN) AND
               result->>'provider_done_reason' IN ('','stop','length');
    END IF;
    RETURN result->>'provider_response_model'='' AND
           NOT (result->>'provider_done_present')::BOOLEAN AND
           NOT (result->>'provider_done')::BOOLEAN AND
           result->>'provider_done_reason'='' AND
           NOT (result->>'provider_usage_present')::BOOLEAN AND
           result->'provider_usage'='{
             "prompt_eval_count":0,"eval_count":0,"total_duration_nanos":0,
             "load_duration_nanos":0,"prompt_eval_duration_nanos":0,
             "eval_duration_nanos":0
           }'::jsonb;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
