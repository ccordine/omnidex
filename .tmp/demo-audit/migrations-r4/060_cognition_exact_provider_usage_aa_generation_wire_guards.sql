CREATE OR REPLACE FUNCTION cognition_provider_wire_int64_is_exact(
    value JSON,
    nonnegative BOOLEAN
) RETURNS BOOLEAN AS $$
DECLARE numeric_text TEXT;
DECLARE numeric_value NUMERIC;
BEGIN
    IF json_typeof(value)<>'number' THEN RETURN FALSE; END IF;
    numeric_text := value::TEXT;
    IF numeric_text !~ '^-?(0|[1-9][0-9]*)$' THEN RETURN FALSE; END IF;
    numeric_value := numeric_text::NUMERIC;
    RETURN numeric_value BETWEEN -9223372036854775808 AND 9223372036854775807 AND
           (NOT nonnegative OR numeric_value>=0);
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_wire_bytes_is_exact(
    value JSON,
    byte_limit BIGINT
) RETURNS BOOLEAN AS $$
DECLARE decoded BYTEA;
DECLARE original_bytes BIGINT;
DECLARE is_complete BOOLEAN;
DECLARE capture_limit BIGINT := ((byte_limit+1+2)/3)*4;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'original_bytes','original_sha256','complete','capture'
       ]) OR NOT cognition_provider_wire_int64_is_exact(
           value->'original_bytes',TRUE
       ) OR
       json_typeof(value->'original_sha256')<>'string' OR
       json_typeof(value->'complete')<>'boolean' OR
       json_typeof(value->'capture')<>'string' OR byte_limit<0 OR
       value::jsonb->>'original_sha256' !~ '^[0-9a-f]{64}$' OR
       octet_length(value::jsonb->>'capture')>capture_limit THEN RETURN FALSE;
    END IF;
    original_bytes := (value::jsonb->>'original_bytes')::BIGINT;
    is_complete := (value::jsonb->>'complete')::BOOLEAN;
    decoded := decode(value::jsonb->>'capture','base64');
    IF original_bytes<0 OR translate(encode(decoded,'base64'),E'\n\r','')<>
       value::jsonb->>'capture' THEN RETURN FALSE; END IF;
    IF is_complete THEN
        RETURN original_bytes=octet_length(decoded) AND original_bytes<=byte_limit AND
               value::jsonb->>'original_sha256'=encode(digest(decoded,'sha256'),'hex');
    END IF;
    RETURN original_bytes>byte_limit AND octet_length(decoded)=byte_limit+1;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_content_encoding_wire_is_exact(value JSON)
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value,ARRAY[
               'schema','values','complete','sha256','bytes','captured_base64',
               'captured_bytes','uncompressed'
           ]) AND cognition_provider_wire_bytes_is_exact(value->'schema',4096) AND
           cognition_provider_wire_int64_is_exact(value->'values',FALSE) AND
           json_typeof(value->'complete')='boolean' AND
           cognition_provider_wire_bytes_is_exact(value->'sha256',4096) AND
           cognition_provider_wire_int64_is_exact(value->'bytes',FALSE) AND
           cognition_provider_wire_bytes_is_exact(value->'captured_base64',87384) AND
           cognition_provider_wire_int64_is_exact(value->'captured_bytes',FALSE) AND
           json_typeof(value->'uncompressed')='boolean';
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_observation_wire_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE key_name TEXT;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'schema','observed_year','observed_month','observed_day','observed_hour',
           'observed_minute','observed_second','observed_nanosecond','observed_at',
           'observed_location','observed_offset_seconds','attestation_sha256',
           'version_body_sha256','installed_body_sha256','tokenizer_request_sha256',
           'tokenizer_body_sha256','preload_body_sha256','runner_body_sha256',
           'preload_method','preload_endpoint','preload_request_sha256','challenge_sha256',
           'evidence_schema','evidence_id','evidence_sha256','evidence_bytes','observation_sha256'
       ]) THEN RETURN FALSE; END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'schema','observed_at','observed_location','attestation_sha256',
        'version_body_sha256','installed_body_sha256','tokenizer_request_sha256',
        'tokenizer_body_sha256','preload_body_sha256','runner_body_sha256',
        'preload_method','preload_endpoint','preload_request_sha256','challenge_sha256',
        'evidence_schema','evidence_id','evidence_sha256','observation_sha256'
    ] LOOP
        IF NOT cognition_provider_wire_bytes_is_exact(value->key_name,4096) THEN RETURN FALSE; END IF;
    END LOOP;
    FOREACH key_name IN ARRAY ARRAY[
        'observed_year','observed_month','observed_day','observed_hour','observed_minute',
        'observed_second','observed_nanosecond','observed_offset_seconds','evidence_bytes'
    ] LOOP
        IF NOT cognition_provider_wire_int64_is_exact(
            value->key_name,FALSE
        ) THEN RETURN FALSE; END IF;
    END LOOP;
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_operation_wire_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE key_name TEXT;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'operation','method','endpoint','request_disposition','request_sha256',
           'request_bytes','request','http_status','disposition','response_complete',
           'content_encoding','response_sha256','response_bytes','response_capture'
       ]) THEN RETURN FALSE; END IF;
    FOREACH key_name IN ARRAY ARRAY[
        'operation','method','endpoint','request_disposition','request_sha256',
        'disposition','response_sha256'
    ] LOOP
        IF NOT cognition_provider_wire_bytes_is_exact(value->key_name,4096) THEN RETURN FALSE; END IF;
    END LOOP;
    RETURN cognition_provider_wire_bytes_is_exact(value->'request',4194304) AND
           cognition_provider_wire_bytes_is_exact(value->'response_capture',4194305) AND
           cognition_provider_content_encoding_wire_is_exact(value->'content_encoding') AND
           cognition_provider_wire_int64_is_exact(value->'request_bytes',FALSE) AND
           cognition_provider_wire_int64_is_exact(value->'http_status',FALSE) AND
           json_typeof(value->'response_complete')='boolean' AND
           cognition_provider_wire_int64_is_exact(value->'response_bytes',FALSE);
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_wire_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE key_name TEXT;
DECLARE operation JSON;
DECLARE original_operations BIGINT;
DECLARE captured_operations BIGINT;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'schema','ref_schema','ref_id','ref_sha256','ref_bytes',
           'original_operations','operations'
       ]) OR NOT cognition_provider_wire_int64_is_exact(value->'ref_bytes',FALSE) OR
       NOT cognition_provider_wire_int64_is_exact(value->'original_operations',TRUE) OR
       json_typeof(value->'operations')<>'array' THEN RETURN FALSE; END IF;
    FOREACH key_name IN ARRAY ARRAY['schema','ref_schema','ref_id','ref_sha256'] LOOP
        IF NOT cognition_provider_wire_bytes_is_exact(value->key_name,4096) THEN RETURN FALSE; END IF;
    END LOOP;
    original_operations := (value::jsonb->>'original_operations')::BIGINT;
    captured_operations := json_array_length(value->'operations');
    IF original_operations<0 OR captured_operations>6 OR
       (original_operations<=6 AND original_operations<>captured_operations) OR
       (original_operations>6 AND captured_operations<>6) THEN RETURN FALSE; END IF;
    FOR operation IN SELECT * FROM json_array_elements(value->'operations') LOOP
        IF NOT cognition_provider_identity_operation_wire_is_exact(operation) THEN RETURN FALSE; END IF;
    END LOOP;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
