CREATE OR REPLACE FUNCTION cognition_exact_json_positive_integer(
    value JSONB,
    maximum BIGINT
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN jsonb_typeof(value)='number' AND value::TEXT~'^[1-9][0-9]*$' AND
           (value::TEXT)::NUMERIC<=maximum;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_exact_json_integer(value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN jsonb_typeof(value)='number' AND value::TEXT~'^-?(0|[1-9][0-9]*)$' AND
           (value::TEXT)::NUMERIC BETWEEN -9223372036854775808 AND 9223372036854775807;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_exact_identity_text(
    value JSONB,
    maximum_bytes INTEGER,
    whitespace_free BOOLEAN
) RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='string' AND value#>>'{}'<>'' AND
           octet_length(value#>>'{}')<=maximum_bytes AND
           value#>>'{}'=btrim(
               value#>>'{}',
               U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
           ) AND (NOT whitespace_free OR (value#>>'{}') !~ '[[:space:]]') AND
           (value#>>'{}') !~ '[[:cntrl:]]';
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_sampling_identity_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'schema','temperature','thinking_enabled','response_format',
           'response_schema_version','native_context_limit','context_ceiling_bytes',
           'max_output_tokens','input_protocol','input_special_token_reserve'
       ]) OR jsonb_typeof(value->'schema')<>'string' OR
       jsonb_typeof(value->'temperature')<>'string' OR
       jsonb_typeof(value->'thinking_enabled')<>'boolean' OR
       jsonb_typeof(value->'response_format')<>'string' OR
       jsonb_typeof(value->'response_schema_version')<>'string' OR
       jsonb_typeof(value->'input_protocol')<>'string' OR
       jsonb_typeof(value->'input_special_token_reserve')<>'number' OR
       value->>'schema'<>'omnidex.cognition-policy-sampling.v2' OR
       value->>'temperature'<>'0' OR (value->>'thinking_enabled')::BOOLEAN OR
       value->>'response_format'<>'json' OR
       value->>'response_schema_version'<>'omnidex.cognition-decision-schema.v1' OR
       value->>'input_protocol'<>'omnidex.ollama-raw-generate-request.v1' OR
       value->'input_special_token_reserve'<>'2'::jsonb OR
       NOT cognition_exact_json_positive_integer(value->'native_context_limit',10000000) OR
       NOT cognition_exact_json_positive_integer(value->'context_ceiling_bytes',67108864) OR
       NOT cognition_exact_json_positive_integer(value->'max_output_tokens',10000000) THEN
        RETURN FALSE;
    END IF;
    RETURN (value->>'max_output_tokens')::BIGINT<=
               (value->>'native_context_limit')::BIGINT AND
           (value->>'context_ceiling_bytes')::BIGINT+2+
               (value->>'max_output_tokens')::BIGINT<=
               (value->>'native_context_limit')::BIGINT;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_brain_ref_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE sampling_sha TEXT;
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'model','digest','quantization','sampling_sha256','sampling',
           'native_context_limit','context_ceiling_bytes','backend','backend_version',
           'hardware','hardware_provenance'
       ]) OR NOT cognition_exact_identity_text(value->'model',128,TRUE) OR
       jsonb_typeof(value->'digest')<>'string' OR
       (value->>'digest')!~'^[0-9a-f]{64}$' OR
       NOT cognition_exact_identity_text(value->'quantization',256,FALSE) OR
       jsonb_typeof(value->'sampling_sha256')<>'string' OR
       (value->>'sampling_sha256')!~'^[0-9a-f]{64}$' OR
       NOT cognition_sampling_identity_is_exact(value->'sampling') OR
       NOT cognition_exact_json_positive_integer(value->'native_context_limit',1048576) OR
       (value->>'native_context_limit')::BIGINT<8192 OR
       NOT cognition_exact_json_positive_integer(value->'context_ceiling_bytes',67108864) OR
       jsonb_typeof(value->'backend')<>'string' OR value->>'backend'<>'ollama' OR
       jsonb_typeof(value->'backend_version')<>'string' OR
       value->>'backend_version'<>'0.24.0' OR
       NOT cognition_exact_identity_text(value->'hardware',256,FALSE) OR
       jsonb_typeof(value->'hardware_provenance')<>'string' OR
       value->>'hardware_provenance'<>'configured_authority' OR
       value->'sampling'->'native_context_limit'<>value->'native_context_limit' OR
       value->'sampling'->'context_ceiling_bytes'<>value->'context_ceiling_bytes' THEN
        RETURN FALSE;
    END IF;
    sampling_sha := encode(digest(cognition_canonical_jsonb(value->'sampling'),'sha256'),'hex');
    RETURN value->>'sampling_sha256'=sampling_sha;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
