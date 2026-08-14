CREATE OR REPLACE FUNCTION cognition_call_attempt_v3_types_are_exact(value JSON)
RETURNS BOOLEAN AS $$
DECLARE document JSONB := value::jsonb;
BEGIN
    IF json_typeof(value)<>'object' OR
       NOT cognition_json_object_has_exact_keys(value,ARRAY[
           'schema','id','actor','snapshot_sha256','expected_revision','obligation_id',
           'runtime_budget','context_projection','brain','provider_attestation',
           'host_hardware_attestation','provider_process_activation',
           'envelope_renderer_version','envelope_token_estimator','envelope_estimated_tokens',
           'envelope_sha256','envelope_bytes','envelope','prompt_hint','prompt_hint_sha256',
           'prompt_hint_bytes','model_visible_input_sha256','model_visible_input_bytes',
           'model_visible_estimated_tokens','model_input_token_upper_bound',
           'response_contract_sha256','expected_provider_request_sha256'
       ]) OR json_typeof(value->'schema')<>'string' OR
       json_typeof(value->'id')<>'string' OR json_typeof(value->'snapshot_sha256')<>'string' OR
       json_typeof(value->'obligation_id')<>'string' OR
       json_typeof(value->'envelope_renderer_version')<>'string' OR
       json_typeof(value->'envelope_token_estimator')<>'string' OR
       json_typeof(value->'envelope_sha256')<>'string' OR
       json_typeof(value->'envelope')<>'string' OR json_typeof(value->'prompt_hint')<>'string' OR
       json_typeof(value->'prompt_hint_sha256')<>'string' OR
       json_typeof(value->'model_visible_input_sha256')<>'string' OR
       json_typeof(value->'response_contract_sha256')<>'string' OR
       json_typeof(value->'expected_provider_request_sha256')<>'string' OR
       NOT cognition_exact_json_positive_integer(document->'envelope_estimated_tokens',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'envelope_bytes',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'prompt_hint_bytes',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'model_visible_input_bytes',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'model_visible_estimated_tokens',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'model_input_token_upper_bound',9223372036854775807) THEN
        RETURN FALSE;
    END IF;
    IF json_typeof(value->'actor')<>'object' OR
       NOT cognition_json_object_has_exact_keys(value->'actor',ARRAY[
           'job_id','generation','step_id','attempt','worker_id'
       ]) OR NOT cognition_exact_json_positive_integer(document->'actor'->'job_id',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'actor'->'generation',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'actor'->'step_id',9223372036854775807) OR
       NOT cognition_exact_json_positive_integer(document->'actor'->'attempt',9223372036854775807) OR
       json_typeof(value->'actor'->'worker_id')<>'string' THEN
        RETURN FALSE;
    END IF;
    IF json_typeof(value->'expected_revision')<>'object' OR
       NOT cognition_json_object_has_exact_keys(value->'expected_revision',ARRAY[
           'episode_id','number','sha256'
       ]) OR json_typeof(value->'expected_revision'->'episode_id')<>'string' OR
       NOT cognition_exact_json_positive_integer(
           document->'expected_revision'->'number',9223372036854775807
       ) OR json_typeof(value->'expected_revision'->'sha256')<>'string' THEN
        RETURN FALSE;
    END IF;
    IF NOT cognition_runtime_budget_matches_brain(document->'runtime_budget',document->'brain') OR
       (document->'runtime_budget'->>'remaining_policy_calls')::BIGINT=0 OR
       NOT cognition_provider_attestation_matches_brain(
           document->'provider_attestation',document->'brain'
       ) OR NOT cognition_host_attestation_is_exact(document->'host_hardware_attestation') THEN
        RETURN FALSE;
    END IF;
    IF json_typeof(value->'context_projection')<>'object' OR
       NOT cognition_json_object_has_exact_keys(value->'context_projection',ARRAY[
           'id','sha256','working_set_id','working_set_version','renderer_version'
       ]) OR json_typeof(value->'context_projection'->'id')<>'string' OR
       json_typeof(value->'context_projection'->'sha256')<>'string' OR
       json_typeof(value->'context_projection'->'working_set_id')<>'string' OR
       NOT cognition_exact_json_positive_integer(
           document->'context_projection'->'working_set_version',9223372036854775807
       ) OR json_typeof(value->'context_projection'->'renderer_version')<>'string' THEN
        RETURN FALSE;
    END IF;
    IF json_typeof(value->'provider_process_activation')<>'object' OR
       NOT cognition_json_object_has_exact_keys(value->'provider_process_activation',ARRAY[
           'schema','observation_id','episode_id','actor','stable_brain_sha256',
           'provider_observation_sha256','evidence'
       ]) OR json_typeof(value->'provider_process_activation'->'schema')<>'string' OR
       json_typeof(value->'provider_process_activation'->'observation_id')<>'string' OR
       json_typeof(value->'provider_process_activation'->'episode_id')<>'string' OR
       json_typeof(value->'provider_process_activation'->'stable_brain_sha256')<>'string' OR
       json_typeof(value->'provider_process_activation'->'provider_observation_sha256')<>'string' OR
       document->'provider_process_activation'->'actor'<>document->'actor' OR
       NOT cognition_provider_evidence_ref_is_exact(
           document->'provider_process_activation'->'evidence'
       ) THEN
        RETURN FALSE;
    END IF;
    RETURN TRUE;
EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

ALTER TABLE cognition_policy_calls
ADD CONSTRAINT cognition_policy_calls_attempt_v3_types_exact CHECK (
    cognition_call_attempt_v3_types_are_exact(attempt_json::json)
);
