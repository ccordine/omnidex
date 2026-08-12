CREATE OR REPLACE FUNCTION cognition_provider_attestation_shape_is_bounded(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE metadata_bytes BIGINT;
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'schema','backend','backend_version','model','digest','quantization',
           'native_context_limit','tokenizer_profile','backend_evidence',
           'installed_evidence','runner_evidence','attestation_sha256'
       ]) OR EXISTS (
           SELECT 1 FROM jsonb_each(value) field
           WHERE field.key<>'native_context_limit' AND jsonb_typeof(field.value)<>'string'
       ) OR NOT cognition_exact_json_integer(value->'native_context_limit') THEN
        RETURN FALSE;
    END IF;
    SELECT COALESCE(sum(octet_length(field.value#>>'{}')),0)
      INTO metadata_bytes FROM jsonb_each(value) field
      WHERE jsonb_typeof(field.value)='string';
    RETURN metadata_bytes<=65536;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_attestation_matches_brain(
    value JSONB,
    brain JSONB
) RETURNS BOOLEAN AS $$
BEGIN
    IF NOT cognition_provider_attestation_shape_is_bounded(value) OR
       value->>'schema'<>'omnidex.provider-identity-attestation.v2' OR
       value->>'backend'<>brain->>'backend' OR
       value->>'backend_version'<>brain->>'backend_version' OR
       value->>'model'<>brain->>'model' OR value->>'digest'<>brain->>'digest' OR
       value->>'quantization'<>brain->>'quantization' OR
       value->'native_context_limit'<>brain->'native_context_limit' OR
       value->>'tokenizer_profile'<>'ollama-0.24.0-qwen35-gpt2-boundary-v1' OR
       NOT cognition_exact_identity_text(value->'backend_evidence',256,FALSE) OR
       NOT cognition_exact_identity_text(value->'installed_evidence',256,FALSE) OR
       NOT cognition_exact_identity_text(value->'runner_evidence',256,FALSE) OR
       (value->>'attestation_sha256')!~'^[0-9a-f]{64}$' THEN
        RETURN FALSE;
    END IF;
    RETURN value->>'attestation_sha256'=encode(digest(cognition_canonical_jsonb(
        jsonb_set(value,'{attestation_sha256}',to_jsonb(''::TEXT))
    ),'sha256'),'hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_host_attestation_shape_is_bounded(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE metadata_bytes BIGINT;
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'schema','os','architecture','logical_cpus','cpu_identity_sha256',
           'accelerator_identity_sha256','cpu_evidence','accelerator_evidence',
           'attestation_sha256'
       ]) OR EXISTS (
           SELECT 1 FROM jsonb_each(value) field
           WHERE field.key<>'logical_cpus' AND jsonb_typeof(field.value)<>'string'
       ) OR NOT cognition_exact_json_integer(value->'logical_cpus') THEN
        RETURN FALSE;
    END IF;
    SELECT COALESCE(sum(octet_length(field.value#>>'{}')),0)
      INTO metadata_bytes FROM jsonb_each(value) field
      WHERE jsonb_typeof(field.value)='string';
    RETURN metadata_bytes<=65536;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_host_attestation_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    IF NOT cognition_host_attestation_shape_is_bounded(value) OR
       value->>'schema'<>'omnidex.host-hardware-attestation.v2' OR
       value->>'os'<>'linux' OR
       NOT cognition_exact_identity_text(value->'architecture',64,TRUE) OR
       NOT cognition_exact_json_positive_integer(value->'logical_cpus',9223372036854775807) OR
       (value->>'cpu_identity_sha256')!~'^[0-9a-f]{64}$' OR
       (value->>'accelerator_identity_sha256')!~'^[0-9a-f]{64}$' OR
       value->>'cpu_evidence'<>'linux:/proc/cpuinfo:selected-identity' OR
       value->>'accelerator_evidence'<>
           'linux:/sys/class/drm/card*/device/vendor+device' OR
       (value->>'attestation_sha256')!~'^[0-9a-f]{64}$' THEN
        RETURN FALSE;
    END IF;
    RETURN value->>'attestation_sha256'=encode(digest(cognition_canonical_jsonb(
        jsonb_set(value,'{attestation_sha256}',to_jsonb(''::TEXT))
    ),'sha256'),'hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_stable_brain_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'schema','brain','provider_attestation','host_hardware_attestation','sha256'
       ]) OR jsonb_typeof(value->'schema')<>'string' OR
       value->>'schema'<>'omnidex.stable-brain-authority.v1' OR
       NOT cognition_brain_ref_is_exact(value->'brain') OR
       NOT cognition_provider_attestation_matches_brain(
           value->'provider_attestation',value->'brain'
       ) OR NOT cognition_host_attestation_is_exact(value->'host_hardware_attestation') OR
       jsonb_typeof(value->'sha256')<>'string' OR
       (value->>'sha256')!~'^[0-9a-f]{64}$' THEN
        RETURN FALSE;
    END IF;
    RETURN value->>'sha256'=encode(digest(cognition_canonical_jsonb(
        jsonb_set(value,'{sha256}',to_jsonb(''::TEXT))
    ),'sha256'),'hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;
