CREATE OR REPLACE FUNCTION cognition_json_has_unique_keys(value JSON)
RETURNS BOOLEAN AS $$
DECLARE child JSON;
BEGIN
    IF json_typeof(value)='object' THEN
        IF (SELECT COUNT(*) FROM json_each(value))<>
           (SELECT COUNT(DISTINCT key) FROM json_each(value)) THEN
            RETURN FALSE;
        END IF;
        FOR child IN SELECT nested FROM json_each(value) AS fields(key,nested) LOOP
            IF NOT cognition_json_has_unique_keys(child) THEN RETURN FALSE; END IF;
        END LOOP;
    ELSIF json_typeof(value)='array' THEN
        FOR child IN SELECT nested FROM json_array_elements(value) AS items(nested) LOOP
            IF NOT cognition_json_has_unique_keys(child) THEN RETURN FALSE; END IF;
        END LOOP;
    END IF;
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_json_object_has_only_keys(value JSON, allowed TEXT[])
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='object' AND NOT EXISTS (
        SELECT 1 FROM json_each(value) fields WHERE NOT fields.key=ANY(allowed)
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE cognition_policy_calls
    DROP CONSTRAINT cognition_policy_calls_exact_attempt_check;

ALTER TABLE cognition_policy_calls
    ADD COLUMN envelope_renderer_version TEXT NOT NULL,
    ADD COLUMN envelope_token_estimator TEXT NOT NULL,
    ADD COLUMN envelope_estimated_tokens BIGINT NOT NULL,
    ADD COLUMN envelope_bytes BIGINT NOT NULL,
    ADD COLUMN envelope_sha256 TEXT NOT NULL,
    ADD COLUMN prompt_hint_sha256 TEXT NOT NULL,
    ADD COLUMN prompt_hint_bytes BIGINT NOT NULL,
    ADD COLUMN model_visible_input_sha256 TEXT NOT NULL,
    ADD COLUMN model_visible_input_bytes BIGINT NOT NULL,
    ADD COLUMN model_visible_estimated_tokens BIGINT NOT NULL,
    ADD COLUMN model_input_token_upper_bound BIGINT NOT NULL,
    ADD COLUMN response_contract_sha256 TEXT NOT NULL,
    ADD COLUMN expected_provider_request_sha256 TEXT NOT NULL,
    ADD COLUMN provider_process_observation_id TEXT NOT NULL,
    ADD COLUMN provider_identity_checked BOOLEAN,
    ADD COLUMN provider_request_disposition TEXT,
    ADD COLUMN provider_observation_sha256 TEXT,
    ADD COLUMN provider_request_sha256 TEXT,
    ADD COLUMN provider_http_status INTEGER,
    ADD COLUMN provider_response_disposition TEXT,
    ADD COLUMN provider_response_complete BOOLEAN,
	ADD COLUMN provider_content_encoding_json TEXT,
    ADD COLUMN provider_response_bytes_known BOOLEAN,
    ADD COLUMN provider_response_sha256 TEXT,
    ADD COLUMN provider_response_bytes BIGINT,
    ADD COLUMN provider_response_capture_sha256 TEXT,
    ADD COLUMN provider_response_captured_bytes BIGINT,
	ADD COLUMN provider_response_model TEXT,
	ADD COLUMN provider_done_present BOOLEAN,
	ADD COLUMN provider_done BOOLEAN,
    ADD COLUMN provider_done_reason TEXT,
    ADD COLUMN provider_usage_present BOOLEAN,
    ADD COLUMN provider_usage_valid BOOLEAN,
    ADD COLUMN prompt_eval_count BIGINT,
    ADD COLUMN eval_count BIGINT,
    ADD COLUMN total_duration_nanos BIGINT,
    ADD COLUMN load_duration_nanos BIGINT,
    ADD COLUMN prompt_eval_duration_nanos BIGINT,
    ADD COLUMN eval_duration_nanos BIGINT;

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_v3_input CHECK (
    cognition_json_has_unique_keys(attempt_json::json) AND
    attempt_json=cognition_canonical_jsonb(attempt_json::jsonb) AND
    cognition_json_object_has_exact_keys(attempt_json::json,ARRAY[
        'schema','id','actor','snapshot_sha256','expected_revision','obligation_id',
        'runtime_budget','context_projection','brain','provider_attestation',
        'host_hardware_attestation','provider_process_activation',
        'envelope_renderer_version','envelope_token_estimator',
        'envelope_estimated_tokens','envelope_sha256','envelope_bytes','envelope','prompt_hint',
        'prompt_hint_sha256','prompt_hint_bytes','model_visible_input_sha256',
        'model_visible_input_bytes','model_visible_estimated_tokens','model_input_token_upper_bound',
        'response_contract_sha256',
        'expected_provider_request_sha256'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'actor')::json,ARRAY[
        'job_id','generation','step_id','attempt','worker_id'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'expected_revision')::json,ARRAY[
        'episode_id','number','sha256'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'runtime_budget')::json,ARRAY[
        'remaining_policy_calls','max_input_bytes','max_input_tokens','max_output_bytes',
        'max_output_tokens','max_evidence_refs','max_action_arguments','max_ledger_proposals',
        'max_attention_requests','max_expected_effect_bytes'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'context_projection')::json,ARRAY[
        'id','sha256','working_set_id','working_set_version','renderer_version'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'brain')::json,ARRAY[
        'model','digest','quantization','sampling_sha256','sampling','native_context_limit',
        'context_ceiling_bytes','backend','backend_version','hardware','hardware_provenance'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'brain'->'sampling')::json,ARRAY[
        'schema','temperature','thinking_enabled','response_format','response_schema_version',
        'native_context_limit','context_ceiling_bytes','max_output_tokens','input_protocol',
        'input_special_token_reserve'
    ]) AND
    cognition_json_object_has_exact_keys((attempt_json::json->'provider_attestation')::json,ARRAY[
        'schema','backend','backend_version','model','digest','quantization',
		'native_context_limit','tokenizer_profile','backend_evidence','installed_evidence','runner_evidence',
        'attestation_sha256'
    ]) AND
    cognition_json_object_has_exact_keys(
        (attempt_json::json->'host_hardware_attestation')::json,ARRAY[
            'schema','os','architecture','logical_cpus','cpu_identity_sha256',
            'accelerator_identity_sha256','cpu_evidence','accelerator_evidence',
            'attestation_sha256'
        ]
    ) AND
    cognition_json_object_has_exact_keys(
        (attempt_json::json->'provider_process_activation')::json,ARRAY[
            'schema','observation_id','episode_id','actor','stable_brain_sha256',
            'provider_observation_sha256','evidence'
        ]
    ) AND
    cognition_json_object_has_exact_keys(
        (attempt_json::json->'provider_process_activation'->'actor')::json,ARRAY[
            'job_id','generation','step_id','attempt','worker_id'
        ]
    ) AND
    cognition_json_object_has_exact_keys(
        (attempt_json::json->'provider_process_activation'->'evidence')::json,ARRAY[
            'schema','id','sha256','bytes'
        ]
    ) AND
    attempt_json::jsonb->'provider_process_activation'->>'schema'=
        'omnidex.provider-process-activation-authority.v1' AND
    provider_process_observation_id~'^provider_process_observation_[0-9a-f]{64}$' AND
    attempt_json::jsonb->'provider_process_activation'->>'episode_id'=episode_id AND
    attempt_json::jsonb->'provider_process_activation'->'actor'=
        attempt_json::jsonb->'actor' AND
    attempt_json::jsonb->'provider_process_activation'->>'stable_brain_sha256'~
        '^[0-9a-f]{64}$' AND
    attempt_json::jsonb->'provider_process_activation'->>'provider_observation_sha256'~
        '^[0-9a-f]{64}$' AND
    envelope_renderer_version='omnidex.cognition-policy-renderer.v2' AND
    envelope_token_estimator='utf8-bytes-div-four.v1' AND
    envelope_estimated_tokens>0 AND envelope_bytes>0 AND prompt_hint_bytes>0 AND
    model_visible_input_bytes=envelope_bytes+prompt_hint_bytes+1 AND
    model_visible_estimated_tokens>0 AND
    model_input_token_upper_bound=model_visible_input_bytes+
        (attempt_json::jsonb->'brain'->'sampling'->>'input_special_token_reserve')::BIGINT AND
    envelope_sha256~'^[0-9a-f]{64}$' AND prompt_hint_sha256~'^[0-9a-f]{64}$' AND
    model_visible_input_sha256~'^[0-9a-f]{64}$' AND
    response_contract_sha256~'^[0-9a-f]{64}$' AND
    expected_provider_request_sha256~'^[0-9a-f]{64}$' AND
    envelope_bytes=octet_length(attempt_json::jsonb->>'envelope') AND
    envelope_sha256=encode(digest(attempt_json::jsonb->>'envelope','sha256'),'hex') AND
    envelope_estimated_tokens=(envelope_bytes+3)/4 AND
    prompt_hint_bytes=octet_length(attempt_json::jsonb->>'prompt_hint') AND
    prompt_hint_sha256=encode(digest(attempt_json::jsonb->>'prompt_hint','sha256'),'hex') AND
    model_visible_estimated_tokens=(model_visible_input_bytes+3)/4 AND
    model_visible_input_bytes<=(attempt_json::jsonb->'runtime_budget'->>'max_input_bytes')::BIGINT AND
    model_input_token_upper_bound<=
        (attempt_json::jsonb->'runtime_budget'->>'max_input_tokens')::BIGINT AND
    attempt_json::jsonb->'brain'->'sampling'->>'schema'=
        'omnidex.cognition-policy-sampling.v2' AND
    attempt_json::jsonb->'brain'->'sampling'->>'input_protocol'=
        'omnidex.ollama-raw-generate-request.v1' AND
    (attempt_json::jsonb->'brain'->'sampling'->>'input_special_token_reserve')::BIGINT=2 AND
    model_visible_input_sha256=encode(digest(
        (attempt_json::jsonb->>'envelope')||E'\n'||(attempt_json::jsonb->>'prompt_hint'),
        'sha256'
    ),'hex')
);

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_v3_result CHECK (
    (status IN ('started','abandoned') AND provider_identity_checked IS NULL AND
     provider_request_disposition IS NULL AND
     provider_observation_sha256 IS NULL AND provider_usage_valid IS NULL) OR
    (status IN ('accepted','rejected','failed') AND provider_identity_checked IS NOT NULL AND
     provider_request_disposition IS NOT NULL AND
	 provider_http_status IS NOT NULL AND provider_response_complete IS NOT NULL AND
	 provider_content_encoding_json IS NOT NULL AND
	 provider_response_bytes_known IS NOT NULL AND
     provider_response_bytes>=0 AND provider_response_captured_bytes>=0 AND
	 provider_response_model IS NOT NULL AND provider_done_present IS NOT NULL AND
	 provider_done IS NOT NULL AND provider_usage_present IS NOT NULL AND provider_usage_valid IS NOT NULL AND
     prompt_eval_count>=0 AND eval_count>=0 AND total_duration_nanos>=0 AND
     load_duration_nanos>=0 AND prompt_eval_duration_nanos>=0 AND eval_duration_nanos>=0)
);

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_v3_response_capture CHECK (
    status IN ('started','abandoned') OR
    (provider_request_disposition='not_dispatched' AND NOT provider_identity_checked AND
     provider_observation_sha256 IS NULL AND provider_request_sha256 IS NULL AND
     provider_http_status=0 AND provider_response_disposition IS NULL AND
	 NOT provider_response_complete AND NOT provider_response_bytes_known AND
	 provider_content_encoding_json='{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}' AND
	 provider_response_sha256 IS NULL AND
     provider_response_bytes=0 AND provider_response_capture_sha256 IS NULL AND
	 provider_response_captured_bytes=0 AND provider_response_model='' AND
	 NOT provider_done_present AND NOT provider_done AND provider_done_reason='' AND
	 NOT provider_usage_present AND
     NOT provider_usage_valid AND prompt_eval_count=0 AND eval_count=0 AND
     total_duration_nanos=0 AND load_duration_nanos=0 AND
     prompt_eval_duration_nanos=0 AND eval_duration_nanos=0) OR
    (provider_request_disposition IN ('','not_dispatched','dispatched','write_indeterminate') AND
     NOT provider_identity_checked AND
     result_json::jsonb->'provider_generation_evidence'->>'id'<>'' AND
     provider_observation_sha256 IS NULL AND provider_request_sha256 IS NULL AND
     provider_http_status=0 AND provider_response_disposition IS NULL AND
     NOT provider_response_complete AND NOT provider_response_bytes_known AND
	 provider_content_encoding_json='{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}' AND
     provider_response_sha256 IS NULL AND provider_response_bytes=0 AND
     provider_response_capture_sha256 IS NULL AND provider_response_captured_bytes=0 AND
	 provider_response_model='' AND NOT provider_done_present AND NOT provider_done AND
	 provider_done_reason='' AND NOT provider_usage_present AND NOT provider_usage_valid AND
     prompt_eval_count=0 AND eval_count=0 AND total_duration_nanos=0 AND
     load_duration_nanos=0 AND prompt_eval_duration_nanos=0 AND eval_duration_nanos=0) OR
    (provider_request_disposition IN ('not_dispatched','dispatched','write_indeterminate') AND
     provider_identity_checked AND
     provider_response_disposition='transport_error' AND provider_http_status=0 AND
	 NOT provider_response_complete AND NOT provider_response_bytes_known AND
	 provider_content_encoding_json='{"bytes":0,"captured_base64":"","captured_bytes":0,"complete":false,"schema":"","sha256":"","uncompressed":false,"values":0}' AND
	 provider_response_sha256 IS NULL AND
     provider_response_bytes=0 AND provider_response_capture_sha256 IS NULL AND
	 provider_response_captured_bytes=0 AND provider_response_model='' AND
	 NOT provider_done_present AND NOT provider_done AND provider_done_reason='') OR
    (provider_request_disposition='dispatched' AND provider_identity_checked AND
     provider_response_disposition IN ('body_limit','body_read_error') AND
	 provider_http_status BETWEEN 100 AND 599 AND NOT provider_response_complete AND
	 NOT provider_response_bytes_known AND provider_response_bytes=0 AND
	 cognition_provider_content_encoding_is_exact(provider_content_encoding_json) AND
	 provider_response_sha256 IS NULL AND provider_response_capture_sha256~'^[0-9a-f]{64}$' AND
	 ((provider_response_disposition='body_limit' AND provider_response_captured_bytes=16777217) OR
	  (provider_response_disposition='body_read_error' AND
	   provider_response_captured_bytes BETWEEN 0 AND 16777217)) AND
	 provider_response_model='' AND NOT provider_done_present AND NOT provider_done AND
	 provider_done_reason='') OR
    (provider_request_disposition='dispatched' AND provider_identity_checked AND
     provider_response_disposition IN ('succeeded','http_error','invalid_json','empty_content') AND
	 provider_http_status BETWEEN 100 AND 599 AND provider_response_complete AND
	 provider_response_bytes_known AND
	 cognition_provider_content_encoding_is_exact(provider_content_encoding_json) AND
	 provider_response_sha256~'^[0-9a-f]{64}$' AND
	 provider_response_sha256=provider_response_capture_sha256 AND
	 provider_response_bytes=provider_response_captured_bytes AND
	 provider_response_captured_bytes BETWEEN 0 AND 16777216 AND
	 ((provider_response_disposition IN ('succeeded','empty_content') AND
	   cognition_provider_content_encoding_is_identity(provider_content_encoding_json) AND
	   provider_response_model<>'' AND (NOT provider_done_present OR provider_done) AND
	   provider_done_reason IN ('','stop','length')) OR
	  (provider_response_disposition NOT IN ('succeeded','empty_content') AND
	   provider_response_model='' AND NOT provider_done_present AND NOT provider_done AND
	   provider_done_reason='')))
);

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_exact_attempt_check CHECK (
    call_id='cognition_call_'||encode(digest(
        cognition_canonical_jsonb(attempt_json::jsonb-'id'),'sha256'
    ),'hex') AND attempt_json::jsonb->>'schema'='omnidex.cognition-policy-call-attempt.v3' AND
    attempt_json::jsonb->>'id'=call_id AND attempt_json::jsonb->'actor'->>'job_id'=job_id::TEXT AND
    attempt_json::jsonb->'actor'->>'generation'=generation::TEXT AND
    attempt_json::jsonb->'actor'->>'step_id'=step_id::TEXT AND
    attempt_json::jsonb->'actor'->>'attempt'=step_attempt::TEXT AND
    attempt_json::jsonb->'actor'->>'worker_id'=worker_id AND
    attempt_json::jsonb->>'snapshot_sha256'=snapshot_sha256 AND
    attempt_json::jsonb->'expected_revision'->>'episode_id'=episode_id AND
    attempt_json::jsonb->'expected_revision'->>'number'=expected_revision::TEXT AND
    attempt_json::jsonb->'expected_revision'->>'sha256'=expected_revision_sha256 AND
    attempt_json::jsonb->>'obligation_id'=obligation_node_id AND
    attempt_json::jsonb->'runtime_budget'=runtime_budget_json::jsonb AND
    attempt_json::jsonb->'context_projection'->>'id'=projection_id AND
    attempt_json::jsonb->'context_projection'->>'working_set_id'=working_set_id AND
    attempt_json::jsonb->'brain'=brain_json::jsonb
);
