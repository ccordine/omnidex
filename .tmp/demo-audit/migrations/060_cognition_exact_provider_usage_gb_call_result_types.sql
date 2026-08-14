CREATE OR REPLACE FUNCTION cognition_policy_evidence_ref_types_are_exact(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value::json,ARRAY[
               'schema','id','sha256','bytes'
           ]) AND jsonb_typeof(value->'schema')='string' AND
           jsonb_typeof(value->'id')='string' AND
           jsonb_typeof(value->'sha256')='string' AND
           cognition_exact_json_nonnegative_integer(value->'bytes',121527588);
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_content_encoding_types_are_exact(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value::json,ARRAY[
               'schema','values','complete','sha256','bytes','captured_base64',
               'captured_bytes','uncompressed'
           ]) AND jsonb_typeof(value->'schema')='string' AND
           cognition_exact_json_nonnegative_integer(value->'values',9223372036854775807) AND
           jsonb_typeof(value->'complete')='boolean' AND
           jsonb_typeof(value->'sha256')='string' AND
           cognition_exact_json_nonnegative_integer(value->'bytes',9223372036854775807) AND
           jsonb_typeof(value->'captured_base64')='string' AND
           cognition_exact_json_nonnegative_integer(value->'captured_bytes',9223372036854775807) AND
           jsonb_typeof(value->'uncompressed')='boolean';
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_call_result_v3_types_are_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE document JSONB := value::jsonb;
DECLARE key_name TEXT;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_call_result_v3_shape_is_exact(value::TEXT) OR
       json_typeof(value->'schema')<>'string' OR json_typeof(value->'call_id')<>'string' OR
       json_typeof(value->'status')<>'string' OR
       json_typeof(value->'provider_identity_checked')<>'boolean' OR
       json_typeof(value->'provider_request_disposition')<>'string' OR
       NOT cognition_exact_json_nonnegative_integer(document->'provider_http_status',2147483647) OR
       json_typeof(value->'provider_response_complete')<>'boolean' OR
       NOT cognition_provider_content_encoding_types_are_exact(
           document->'provider_content_encoding'
       ) OR json_typeof(value->'provider_response_bytes_known')<>'boolean' OR
       NOT cognition_exact_json_nonnegative_integer(
           document->'provider_response_bytes',9223372036854775807
       ) OR NOT cognition_exact_json_nonnegative_integer(
           document->'provider_response_captured_bytes',9223372036854775807
       ) OR json_typeof(value->'provider_response_model')<>'string' OR
       json_typeof(value->'provider_done_present')<>'boolean' OR
       json_typeof(value->'provider_done')<>'boolean' OR
       json_typeof(value->'provider_done_reason')<>'string' OR
       json_typeof(value->'provider_usage_present')<>'boolean' OR
       NOT cognition_exact_json_nonnegative_integer(document->'response_bytes',9223372036854775807) THEN
        RETURN FALSE;
    END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'provider_request_sha256','provider_response_disposition','provider_response_sha256',
        'provider_response_capture_sha256','response_sha256','decision_sha256',
        'failure_code','failure_message'
    ] LOOP
        IF document ? key_name AND json_typeof(value->key_name)<>'string' THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    IF NOT cognition_provider_attestation_shape_is_bounded(document->'provider_attestation') OR
       NOT cognition_provider_observation_shape_is_bounded(document->'provider_observation') OR
       NOT cognition_policy_evidence_ref_types_are_exact(
           document->'provider_identity_evidence'
       ) OR NOT cognition_policy_evidence_ref_types_are_exact(
           document->'provider_response_capture_evidence'
       ) OR NOT cognition_policy_evidence_ref_types_are_exact(
           document->'provider_generation_evidence'
       ) OR NOT cognition_policy_evidence_ref_types_are_exact(document->'response_evidence') THEN
        RETURN FALSE;
    END IF;
    IF jsonb_typeof(document->'provider_usage')<>'object' OR
       NOT cognition_json_object_has_exact_keys((document->'provider_usage')::json,ARRAY[
           'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
           'prompt_eval_duration_nanos','eval_duration_nanos'
       ]) THEN
        RETURN FALSE;
    END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
        'prompt_eval_duration_nanos','eval_duration_nanos'
    ] LOOP
        IF NOT cognition_exact_json_nonnegative_integer(
            document->'provider_usage'->key_name,9223372036854775807
        ) THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    RETURN jsonb_typeof(document->'action_schema')='object' AND
           cognition_json_object_has_exact_keys((document->'action_schema')::json,ARRAY[
               'id','version','sha256'
           ]) AND jsonb_typeof(document->'action_schema'->'id')='string' AND
           jsonb_typeof(document->'action_schema'->'version')='string' AND
           jsonb_typeof(document->'action_schema'->'sha256')='string';
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

ALTER TABLE cognition_policy_calls
ADD CONSTRAINT cognition_policy_calls_result_v3_types_exact CHECK (
    status IN ('started','abandoned') OR
    cognition_call_result_v3_types_are_exact(result_json::json)
);
