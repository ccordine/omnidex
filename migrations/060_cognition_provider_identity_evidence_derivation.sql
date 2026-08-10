CREATE OR REPLACE FUNCTION cognition_provider_identity_model_shape_is_exact(value JSON)
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='object' AND
           cognition_json_object_has_only_keys(value,ARRAY[
               'name','model','size','digest','details','modified_at','expires_at',
               'size_vram','context_length'
           ]) AND
           ((value::jsonb ? 'details') IS FALSE OR (
               jsonb_typeof(value::jsonb->'details')='object' AND
               cognition_json_object_has_only_keys((value::jsonb->'details')::json,ARRAY[
                   'parent_model','format','family','families','parameter_size',
                   'quantization_level'
               ])
           ));
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_provider_identity_models_shape_is_exact(value JSON)
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='array' AND NOT EXISTS (
        SELECT 1 FROM json_array_elements(value) AS item
        WHERE NOT cognition_provider_identity_model_shape_is_exact(item)
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

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
       version_doc::jsonb->>'version'<>backend_version OR
       NOT cognition_json_has_unique_keys(installed_doc) OR
       NOT cognition_json_object_has_exact_keys(installed_doc,ARRAY['models']) OR
       NOT cognition_provider_identity_models_shape_is_exact(
           (installed_doc::jsonb->'models')::TEXT::json
       ) OR
       NOT cognition_json_has_unique_keys(tokenizer_doc) OR
       json_typeof(tokenizer_doc)<>'object' OR
       jsonb_typeof(tokenizer_doc::jsonb->'model_info')<>'object' OR
       NOT cognition_json_has_unique_keys(preload_doc) OR
       json_typeof(preload_doc)<>'object' OR
       (preload_doc::jsonb->>'done')::BOOLEAN IS NOT TRUE OR
       NOT cognition_json_has_unique_keys(runner_doc) OR
       NOT cognition_json_object_has_exact_keys(runner_doc,ARRAY['models']) OR
       NOT cognition_provider_identity_models_shape_is_exact(
           (runner_doc::jsonb->'models')::TEXT::json
       ) THEN
        RETURN FALSE;
    END IF;
    SELECT item INTO installed_model FROM jsonb_array_elements(installed_doc::jsonb->'models') item
    WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name);
    IF NOT FOUND OR (SELECT COUNT(*) FROM jsonb_array_elements(installed_doc::jsonb->'models') item
        WHERE item->>'name'=model_name AND COALESCE(item->>'model','') IN ('',model_name))<>1 OR
       installed_model->>'digest'<>model_digest OR
       installed_model->'details'->>'quantization_level'<>quantization THEN
        RETURN FALSE;
    END IF;
    tokenizer_info := tokenizer_doc::jsonb->'model_info';
    IF tokenizer_info->>'general.architecture'<>'qwen35' OR
       tokenizer_info->>'tokenizer.ggml.model'<>'gpt2' OR
       tokenizer_info->>'tokenizer.ggml.pre'<>'qwen35' OR
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
       runner_model->>'digest'<>model_digest OR
       runner_model->'details'->>'quantization_level'<>quantization OR
       (runner_model->>'context_length')::BIGINT<>native_context THEN
        RETURN FALSE;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql STABLE STRICT;
