CREATE OR REPLACE FUNCTION cognition_provider_evidence_ref_shape_is_bounded(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
           cognition_json_object_has_exact_keys(value::json,ARRAY[
               'schema','id','sha256','bytes'
           ]) AND jsonb_typeof(value->'schema')='string' AND
           jsonb_typeof(value->'id')='string' AND
           jsonb_typeof(value->'sha256')='string' AND
           cognition_exact_json_integer(value->'bytes') AND
           octet_length(value->>'schema')+octet_length(value->>'id')+
               octet_length(value->>'sha256')<=65536;
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_evidence_ref_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
    SELECT cognition_provider_evidence_ref_shape_is_bounded(value) AND
           value->>'schema'='omnidex.provider-identity-evidence-ref.v1' AND
           value->>'sha256'~'^[0-9a-f]{64}$' AND
           value->>'id'='provider_identity_'||(value->>'sha256') AND
           cognition_exact_json_positive_integer(value->'bytes',29360135);
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_observation_shape_is_bounded(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE metadata_bytes BIGINT;
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'schema','observed_at','attestation_sha256','version_body_sha256',
           'installed_body_sha256','tokenizer_request_sha256','tokenizer_body_sha256',
           'preload_body_sha256','runner_body_sha256','preload_method','preload_endpoint',
           'preload_request_sha256','challenge_sha256','evidence','observation_sha256'
       ]) OR EXISTS (
           SELECT 1 FROM jsonb_each(value) field
           WHERE field.key<>'evidence' AND jsonb_typeof(field.value)<>'string'
       ) OR NOT cognition_provider_evidence_ref_shape_is_bounded(value->'evidence') THEN
        RETURN FALSE;
    END IF;
    SELECT COALESCE(sum(octet_length(field.value#>>'{}')),0)
      INTO metadata_bytes FROM jsonb_each(value) field
      WHERE jsonb_typeof(field.value)='string';
    metadata_bytes := metadata_bytes+
        octet_length(cognition_canonical_jsonb(value->'evidence'));
    RETURN metadata_bytes<=65536;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_observation_is_exact(
    value JSONB,
    attestation_sha TEXT,
    challenge_sha TEXT
) RETURNS BOOLEAN AS $$
BEGIN
    IF NOT cognition_provider_observation_shape_is_bounded(value) OR
       value->>'schema'<>'omnidex.provider-identity-observation.v2' OR
       NOT cognition_provider_timestamp_is_exact(value->'observed_at',6) OR
       value->>'observed_at'='0001-01-01T00:00:00Z' OR
       NOT cognition_provider_evidence_ref_is_exact(value->'evidence') OR
       value->>'attestation_sha256'<>attestation_sha OR
       value->>'challenge_sha256'<>challenge_sha OR
       NOT cognition_exact_identity_text(value->'preload_method',16,FALSE) OR
       NOT cognition_exact_identity_text(value->'preload_endpoint',256,FALSE) OR EXISTS (
           SELECT 1 FROM jsonb_each(value) field
           WHERE field.key IN (
               'attestation_sha256','version_body_sha256','installed_body_sha256',
               'tokenizer_request_sha256','tokenizer_body_sha256','preload_body_sha256',
               'runner_body_sha256','preload_request_sha256','challenge_sha256',
               'observation_sha256'
           ) AND field.value#>>'{}' !~ '^[0-9a-f]{64}$'
       ) THEN
        RETURN FALSE;
    END IF;
    RETURN value->>'observation_sha256'=encode(digest(cognition_canonical_jsonb(
        jsonb_set(value,'{observation_sha256}',to_jsonb(''::TEXT))
    ),'sha256'),'hex');
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_attested_brain_is_exact(value JSONB)
RETURNS BOOLEAN AS $$
DECLARE challenge TEXT;
BEGIN
    IF jsonb_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value::json,ARRAY[
           'brain','provider_attestation','bootstrap_provider_observation',
           'host_hardware_attestation'
       ]) OR NOT cognition_brain_ref_is_exact(value->'brain') OR
       NOT cognition_provider_attestation_matches_brain(
           value->'provider_attestation',value->'brain'
       ) OR NOT cognition_host_attestation_is_exact(value->'host_hardware_attestation') THEN
        RETURN FALSE;
    END IF;
    challenge := cognition_provider_bootstrap_challenge(value->'brain');
    RETURN cognition_provider_observation_is_exact(
        value->'bootstrap_provider_observation',
        value->'provider_attestation'->>'attestation_sha256',challenge
    );
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_episodes WHERE
          NOT cognition_json_has_unique_keys(attested_brain_json::json) OR
          NOT cognition_attested_brain_is_exact(attested_brain_json::jsonb)
    ) THEN
        RAISE EXCEPTION 'migration 060 cannot prove an existing cognition episode Brain';
    END IF;
END;
$$;

ALTER TABLE cognition_episodes ADD CONSTRAINT cognition_episodes_attested_brain_exact CHECK (
    cognition_json_has_unique_keys(attested_brain_json::json) AND
    cognition_attested_brain_is_exact(attested_brain_json::jsonb)
);
