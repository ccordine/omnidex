CREATE OR REPLACE FUNCTION cognition_provider_identity_model_shape_is_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE document JSONB;
DECLARE details JSON;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_only_keys(value,ARRAY[
           'name','model','size','digest','details','modified_at','expires_at',
           'size_vram','context_length'
       ]) THEN
        RETURN FALSE;
    END IF;
    document := value::jsonb;
    IF EXISTS (
        SELECT 1 FROM json_each(value) field
        WHERE field.key IN ('name','model','digest') AND
              json_typeof(field.value) NOT IN ('string','null')
    ) OR EXISTS (
        SELECT 1 FROM json_each(value) field
        WHERE field.key IN ('size','size_vram','context_length') AND
          NOT (json_typeof(field.value)='null' OR (
              json_typeof(field.value)='number' AND
              field.value::TEXT~'^-?(0|[1-9][0-9]*)$' AND
              (field.value::TEXT)::NUMERIC BETWEEN -9223372036854775808 AND 9223372036854775807
          ))
    ) OR EXISTS (
        SELECT 1 FROM json_each(value) field
        WHERE field.key IN ('modified_at','expires_at') AND
              cognition_provider_identity_json_time_is_decodable(field.value::jsonb) IS NOT TRUE
    ) THEN
        RETURN FALSE;
    END IF;
    IF NOT (document ? 'details') OR document->'details'='null'::jsonb THEN
        RETURN TRUE;
    ELSIF jsonb_typeof(document->'details')<>'object' THEN
        RETURN FALSE;
    END IF;
    details := (document->'details')::TEXT::json;
    IF NOT cognition_json_object_has_only_keys(details,ARRAY[
           'parent_model','format','family','families','parameter_size','quantization_level'
       ]) OR EXISTS (
           SELECT 1 FROM json_each(details) field
           WHERE field.key<>'families' AND json_typeof(field.value) NOT IN ('string','null')
       ) THEN
        RETURN FALSE;
    END IF;
    IF document->'details' ? 'families' AND
       document->'details'->'families'<>'null'::jsonb AND (
          jsonb_typeof(document->'details'->'families')<>'array' OR EXISTS (
              SELECT 1 FROM json_array_elements(details->'families') member
              WHERE json_typeof(member) NOT IN ('string','null')
          )
       ) THEN
        RETURN FALSE;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_models_shape_is_exact(value JSON)
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='array' AND NOT EXISTS (
        SELECT 1 FROM json_array_elements(value) AS item
        WHERE NOT cognition_provider_identity_model_shape_is_exact(item)
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_requests_match_brain(
    evidence_identity TEXT,
    brain JSONB
) RETURNS BOOLEAN AS $$
    SELECT NOT EXISTS (
        SELECT 1 FROM cognition_provider_identity_evidence_operations operations
        WHERE operations.evidence_id=evidence_identity AND (
            (operations.operation_index IN (0,1,4) AND operations.request_bytes<>0) OR
            (operations.operation_index=2 AND convert_from(operations.request_body,'UTF8')<>
                cognition_canonical_jsonb(jsonb_build_object(
                    'model',brain->>'model','verbose',FALSE
                ))) OR
            (operations.operation_index=3 AND convert_from(operations.request_body,'UTF8')<>
                cognition_canonical_jsonb(jsonb_build_object(
                    'model',brain->>'model','stream',FALSE,'keep_alive','5m',
                    'options',jsonb_build_object(
                        'num_ctx',(brain->>'native_context_limit')::BIGINT
                    )
                )))
        )
    );
$$ LANGUAGE SQL STABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_observation_matches_evidence(
    observation_text TEXT,
    evidence_identity TEXT,
    attestation_sha TEXT,
    challenge_sha TEXT
) RETURNS BOOLEAN AS $$
DECLARE observation JSON;
DECLARE evidence_ref JSONB;
BEGIN
    observation := observation_text::json;
    SELECT ref_json::jsonb INTO evidence_ref
    FROM cognition_provider_identity_evidence WHERE evidence_id=evidence_identity;
    IF evidence_ref IS NULL THEN
        RAISE EXCEPTION 'provider observation raw evidence is unavailable';
    ELSIF NOT cognition_json_has_unique_keys(observation) OR
       observation_text<>cognition_canonical_jsonb(observation::jsonb) OR
       NOT cognition_json_object_has_exact_keys(observation,ARRAY[
           'schema','observed_at','attestation_sha256','version_body_sha256',
           'installed_body_sha256','tokenizer_request_sha256','tokenizer_body_sha256',
           'preload_body_sha256','runner_body_sha256','preload_method','preload_endpoint',
           'preload_request_sha256','challenge_sha256','evidence','observation_sha256'
       ]) OR observation::jsonb->>'schema'<>'omnidex.provider-identity-observation.v2' THEN
        RAISE EXCEPTION 'provider observation JSON shape is inexact';
    ELSIF observation::jsonb->>'attestation_sha256'<>attestation_sha OR
       observation::jsonb->>'challenge_sha256'<>challenge_sha OR
       observation::jsonb->'evidence'<>evidence_ref THEN
        RAISE EXCEPTION 'provider observation authority differs from its request or evidence';
    ELSIF observation::jsonb->>'observed_at' !~ 'Z$' OR
       observation::jsonb->>'observation_sha256'<>
           encode(digest(cognition_canonical_jsonb(jsonb_set(
               observation::jsonb,'{observation_sha256}',to_jsonb(''::TEXT)
           )),'sha256'),'hex') THEN
        RAISE EXCEPTION 'provider observation time or self-hash is invalid';
    ELSIF EXISTS (
               SELECT 1 FROM jsonb_each_text(observation::jsonb) field
               WHERE field.key IN (
                   'attestation_sha256','version_body_sha256','installed_body_sha256',
                   'tokenizer_request_sha256','tokenizer_body_sha256','preload_body_sha256',
                   'runner_body_sha256','preload_request_sha256','challenge_sha256',
                   'observation_sha256'
               ) AND field.value !~ '^[0-9a-f]{64}$'
           ) THEN
        RAISE EXCEPTION 'provider observation contains an invalid digest';
    END IF;
    RETURN EXISTS (
        SELECT 1 FROM cognition_provider_identity_evidence_operations operations
        WHERE operations.evidence_id=evidence_identity
        GROUP BY operations.evidence_id
        HAVING COUNT(*)=5 AND bool_and(operations.disposition='succeeded') AND
          max(operations.response_sha256) FILTER (WHERE operation_index=0)=
              observation::jsonb->>'version_body_sha256' AND
          max(operations.response_sha256) FILTER (WHERE operation_index=1)=
              observation::jsonb->>'installed_body_sha256' AND
          max(operations.request_sha256) FILTER (WHERE operation_index=2)=
              observation::jsonb->>'tokenizer_request_sha256' AND
          max(operations.response_sha256) FILTER (WHERE operation_index=2)=
              observation::jsonb->>'tokenizer_body_sha256' AND
          max(operations.method) FILTER (WHERE operation_index=3)=
              observation::jsonb->>'preload_method' AND
          max(operations.endpoint) FILTER (WHERE operation_index=3)=
              observation::jsonb->>'preload_endpoint' AND
          max(operations.request_sha256) FILTER (WHERE operation_index=3)=
              observation::jsonb->>'preload_request_sha256' AND
          max(operations.response_sha256) FILTER (WHERE operation_index=3)=
              observation::jsonb->>'preload_body_sha256' AND
          max(operations.response_sha256) FILTER (WHERE operation_index=4)=
              observation::jsonb->>'runner_body_sha256'
    );
END;
$$ LANGUAGE plpgsql STABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_evidence_matches_attempt(
    evidence_identity TEXT,
    attempt JSONB
) RETURNS BOOLEAN AS $$
DECLARE version_doc JSON;
DECLARE installed_doc JSON;
DECLARE tokenizer_doc JSON;
DECLARE preload_doc JSON;
DECLARE runner_doc JSON;
DECLARE installed_model JSONB;
DECLARE runner_model JSONB;
DECLARE model_name TEXT := attempt->'brain'->>'model';
DECLARE model_digest TEXT := attempt->'brain'->>'digest';
DECLARE quantization TEXT := attempt->'brain'->>'quantization';
DECLARE backend_version TEXT := attempt->'brain'->>'backend_version';
DECLARE native_context BIGINT := (attempt->'brain'->>'native_context_limit')::BIGINT;
DECLARE tokenizer_info JSONB;
BEGIN
    IF attempt->'brain'->>'backend'<>'ollama' OR
       attempt->'brain'->>'backend_version'<>'0.24.0' OR
       attempt->'brain'->'sampling'->>'input_protocol'<>
           'omnidex.ollama-raw-generate-request.v1' OR NOT EXISTS (
        SELECT 1 FROM cognition_provider_identity_evidence_operations
        WHERE evidence_id=evidence_identity GROUP BY evidence_id
        HAVING COUNT(*)=5 AND bool_and(disposition='succeeded')
    ) THEN
        RETURN FALSE;
    END IF;
    SELECT convert_from(response_body,'UTF8')::json INTO version_doc
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND operation_index=0;
    SELECT convert_from(response_body,'UTF8')::json INTO installed_doc
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND operation_index=1;
    SELECT convert_from(response_body,'UTF8')::json INTO tokenizer_doc
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND operation_index=2;
    SELECT convert_from(response_body,'UTF8')::json INTO preload_doc
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND operation_index=3;
    SELECT convert_from(response_body,'UTF8')::json INTO runner_doc
    FROM cognition_provider_identity_evidence_operations
    WHERE evidence_id=evidence_identity AND operation_index=4;
    IF NOT cognition_json_has_unique_keys(version_doc) OR
       NOT cognition_json_object_has_exact_keys(version_doc,ARRAY['version']) OR
       json_typeof(version_doc->'version')<>'string' OR
       version_doc::jsonb->>'version' IS DISTINCT FROM backend_version OR
       NOT cognition_json_has_unique_keys(installed_doc) OR
       NOT cognition_json_object_has_exact_keys(installed_doc,ARRAY['models']) OR
       cognition_provider_identity_models_shape_is_exact(installed_doc->'models') IS NOT TRUE OR
       NOT cognition_json_has_unique_keys(tokenizer_doc) OR
       json_typeof(tokenizer_doc)<>'object' OR
       jsonb_typeof(tokenizer_doc::jsonb->'model_info') IS DISTINCT FROM 'object' OR
       NOT cognition_json_has_unique_keys(preload_doc) OR
       json_typeof(preload_doc)<>'object' OR
       preload_doc::jsonb->'done' IS DISTINCT FROM 'true'::jsonb OR
       NOT cognition_json_has_unique_keys(runner_doc) OR
       NOT cognition_json_object_has_exact_keys(runner_doc,ARRAY['models']) OR
       cognition_provider_identity_models_shape_is_exact(runner_doc->'models') IS NOT TRUE THEN
        RETURN FALSE;
    END IF;
    SELECT item INTO installed_model FROM jsonb_array_elements(installed_doc::jsonb->'models') item
    WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name);
    IF NOT FOUND OR (SELECT COUNT(*) FROM jsonb_array_elements(installed_doc::jsonb->'models') item
        WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name))<>1 OR
       installed_model->>'digest' IS DISTINCT FROM model_digest OR
       installed_model->'details'->>'quantization_level' IS DISTINCT FROM quantization THEN
        RETURN FALSE;
    END IF;
    tokenizer_info := tokenizer_doc::jsonb->'model_info';
    IF tokenizer_info->>'general.architecture' IS DISTINCT FROM 'qwen35' OR
       tokenizer_info->>'tokenizer.ggml.model' IS DISTINCT FROM 'gpt2' OR
       tokenizer_info->>'tokenizer.ggml.pre' IS DISTINCT FROM 'qwen35' OR
       tokenizer_info->'tokenizer.ggml.add_eos_token' IS DISTINCT FROM 'false'::jsonb OR
       tokenizer_info->'tokenizer.ggml.add_padding_token' IS DISTINCT FROM 'false'::jsonb OR
       tokenizer_info ? 'tokenizer.ggml.add_bos_token' OR
       tokenizer_info->'tokenizer.ggml.tokens' IS DISTINCT FROM 'null'::jsonb OR
       tokenizer_info->'tokenizer.ggml.token_type' IS DISTINCT FROM 'null'::jsonb OR
       tokenizer_info->'tokenizer.ggml.merges' IS DISTINCT FROM 'null'::jsonb OR EXISTS (
           SELECT 1 FROM jsonb_object_keys(tokenizer_info) key
           WHERE key LIKE 'tokenizer.ggml.add\_%' ESCAPE '\' AND
                 key NOT IN ('tokenizer.ggml.add_eos_token','tokenizer.ggml.add_padding_token')
       ) THEN
        RETURN FALSE;
    END IF;
    SELECT item INTO runner_model FROM jsonb_array_elements(runner_doc::jsonb->'models') item
    WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name);
    IF NOT FOUND OR (SELECT COUNT(*) FROM jsonb_array_elements(runner_doc::jsonb->'models') item
        WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name))<>1 OR
       runner_model->>'digest' IS DISTINCT FROM model_digest OR
       runner_model->'details'->>'quantization_level' IS DISTINCT FROM quantization OR
       (runner_model->>'context_length')::BIGINT IS DISTINCT FROM native_context THEN
        RETURN FALSE;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;
