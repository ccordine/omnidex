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
    ADD COLUMN provider_identity_checked BOOLEAN,
    ADD COLUMN provider_request_dispatched BOOLEAN,
    ADD COLUMN provider_observation_sha256 TEXT,
    ADD COLUMN provider_request_sha256 TEXT,
    ADD COLUMN provider_http_status INTEGER,
    ADD COLUMN provider_response_disposition TEXT,
    ADD COLUMN provider_response_complete BOOLEAN,
    ADD COLUMN provider_response_bytes_known BOOLEAN,
    ADD COLUMN provider_response_sha256 TEXT,
    ADD COLUMN provider_response_bytes BIGINT,
    ADD COLUMN provider_response_capture_sha256 TEXT,
    ADD COLUMN provider_response_captured_bytes BIGINT,
    ADD COLUMN provider_done_reason TEXT,
    ADD COLUMN provider_usage_present BOOLEAN,
    ADD COLUMN provider_usage_valid BOOLEAN,
    ADD COLUMN prompt_eval_count BIGINT,
    ADD COLUMN eval_count BIGINT,
    ADD COLUMN total_duration_nanos BIGINT,
    ADD COLUMN load_duration_nanos BIGINT,
    ADD COLUMN prompt_eval_duration_nanos BIGINT,
    ADD COLUMN eval_duration_nanos BIGINT;

CREATE OR REPLACE FUNCTION project_cognition_policy_call_v3()
RETURNS TRIGGER AS $$
DECLARE usage JSONB;
BEGIN
    IF NEW.attempt_json::jsonb->>'schema'<>'omnidex.cognition-policy-call-attempt.v3' THEN
        RAISE EXCEPTION 'cognition policy call requires exact attempt schema v3';
    END IF;
    NEW.envelope_renderer_version := NEW.attempt_json::jsonb->>'envelope_renderer_version';
    NEW.envelope_token_estimator := NEW.attempt_json::jsonb->>'envelope_token_estimator';
    NEW.envelope_estimated_tokens := (NEW.attempt_json::jsonb->>'envelope_estimated_tokens')::BIGINT;
    NEW.envelope_bytes := (NEW.attempt_json::jsonb->>'envelope_bytes')::BIGINT;
    NEW.envelope_sha256 := NEW.attempt_json::jsonb->>'envelope_sha256';
    NEW.prompt_hint_sha256 := NEW.attempt_json::jsonb->>'prompt_hint_sha256';
    NEW.prompt_hint_bytes := (NEW.attempt_json::jsonb->>'prompt_hint_bytes')::BIGINT;
    NEW.model_visible_input_sha256 := NEW.attempt_json::jsonb->>'model_visible_input_sha256';
    NEW.model_visible_input_bytes := (NEW.attempt_json::jsonb->>'model_visible_input_bytes')::BIGINT;
    NEW.model_visible_estimated_tokens :=
        (NEW.attempt_json::jsonb->>'model_visible_estimated_tokens')::BIGINT;
    NEW.model_input_token_upper_bound :=
        (NEW.attempt_json::jsonb->>'model_input_token_upper_bound')::BIGINT;
    NEW.response_contract_sha256 := NEW.attempt_json::jsonb->>'response_contract_sha256';
    NEW.expected_provider_request_sha256 :=
        NEW.attempt_json::jsonb->>'expected_provider_request_sha256';
    IF NEW.result_json IS NULL THEN
        NEW.provider_identity_checked := NULL;
        NEW.provider_request_dispatched := NULL;
        NEW.provider_observation_sha256 := NULL;
        NEW.provider_request_sha256 := NULL;
        NEW.provider_http_status := NULL;
        NEW.provider_response_disposition := NULL;
        NEW.provider_response_complete := NULL;
        NEW.provider_response_bytes_known := NULL;
        NEW.provider_response_sha256 := NULL;
        NEW.provider_response_bytes := NULL;
        NEW.provider_response_capture_sha256 := NULL;
        NEW.provider_response_captured_bytes := NULL;
        NEW.provider_done_reason := NULL;
        NEW.provider_usage_present := NULL;
        NEW.provider_usage_valid := NULL;
        NEW.prompt_eval_count := NULL;
        NEW.eval_count := NULL;
        NEW.total_duration_nanos := NULL;
        NEW.load_duration_nanos := NULL;
        NEW.prompt_eval_duration_nanos := NULL;
        NEW.eval_duration_nanos := NULL;
        RETURN NEW;
    END IF;
    IF NEW.result_json::jsonb->>'schema'<>'omnidex.cognition-policy-call-result.v3' THEN
        RAISE EXCEPTION 'cognition policy call requires exact result schema v3';
    END IF;
    NEW.provider_identity_checked :=
        (NEW.result_json::jsonb->>'provider_identity_checked')::BOOLEAN;
    NEW.provider_request_dispatched := (NEW.result_json::jsonb->>'provider_request_dispatched')::BOOLEAN;
    NEW.provider_observation_sha256 :=
        NULLIF(NEW.result_json::jsonb->'provider_observation'->>'observation_sha256','');
    NEW.provider_request_sha256 := NEW.result_json::jsonb->>'provider_request_sha256';
    NEW.provider_http_status := (NEW.result_json::jsonb->>'provider_http_status')::INTEGER;
    NEW.provider_response_disposition := NEW.result_json::jsonb->>'provider_response_disposition';
    NEW.provider_response_complete :=
        (NEW.result_json::jsonb->>'provider_response_complete')::BOOLEAN;
    NEW.provider_response_bytes_known :=
        (NEW.result_json::jsonb->>'provider_response_bytes_known')::BOOLEAN;
    NEW.provider_response_sha256 := NEW.result_json::jsonb->>'provider_response_sha256';
    NEW.provider_response_bytes := (NEW.result_json::jsonb->>'provider_response_bytes')::BIGINT;
    NEW.provider_response_capture_sha256 :=
        NEW.result_json::jsonb->>'provider_response_capture_sha256';
    NEW.provider_response_captured_bytes :=
        (NEW.result_json::jsonb->>'provider_response_captured_bytes')::BIGINT;
    NEW.provider_done_reason := NEW.result_json::jsonb->>'provider_done_reason';
    NEW.provider_usage_present := (NEW.result_json::jsonb->>'provider_usage_present')::BOOLEAN;
    usage := NEW.result_json::jsonb->'provider_usage';
    NEW.prompt_eval_count := (usage->>'prompt_eval_count')::BIGINT;
    NEW.eval_count := (usage->>'eval_count')::BIGINT;
    NEW.total_duration_nanos := (usage->>'total_duration_nanos')::BIGINT;
    NEW.load_duration_nanos := (usage->>'load_duration_nanos')::BIGINT;
    NEW.prompt_eval_duration_nanos := (usage->>'prompt_eval_duration_nanos')::BIGINT;
    NEW.eval_duration_nanos := (usage->>'eval_duration_nanos')::BIGINT;
    NEW.provider_usage_valid := NEW.provider_usage_present AND
        NEW.prompt_eval_count>0 AND NEW.eval_count>0 AND
        NEW.total_duration_nanos>0 AND NEW.load_duration_nanos>=0 AND
        NEW.prompt_eval_duration_nanos>0 AND NEW.eval_duration_nanos>0 AND
        NEW.total_duration_nanos>=NEW.load_duration_nanos+
            NEW.prompt_eval_duration_nanos+NEW.eval_duration_nanos;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_policy_calls_00_v3_projection
BEFORE INSERT OR UPDATE ON cognition_policy_calls
FOR EACH ROW EXECUTE FUNCTION project_cognition_policy_call_v3();

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_v3_input CHECK (
    cognition_json_has_unique_keys(attempt_json::json) AND
    attempt_json=cognition_canonical_jsonb(attempt_json::jsonb) AND
    cognition_json_object_has_exact_keys(attempt_json::json,ARRAY[
        'schema','id','actor','snapshot_sha256','expected_revision','obligation_id',
        'runtime_budget','context_projection','brain','provider_attestation',
        'host_hardware_attestation','envelope_renderer_version','envelope_token_estimator',
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
        'native_context_limit','backend_evidence','installed_evidence','runner_evidence',
        'attestation_sha256'
    ]) AND
    cognition_json_object_has_exact_keys(
        (attempt_json::json->'host_hardware_attestation')::json,ARRAY[
            'schema','os','architecture','logical_cpus','cpu_identity_sha256',
            'accelerator_identity_sha256','cpu_evidence','accelerator_evidence',
            'attestation_sha256'
        ]
    ) AND
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
     provider_request_dispatched IS NULL AND
     provider_observation_sha256 IS NULL AND provider_usage_valid IS NULL) OR
    (status IN ('accepted','rejected','failed') AND provider_identity_checked IS NOT NULL AND
     provider_request_dispatched IS NOT NULL AND
	 provider_http_status IS NOT NULL AND provider_response_complete IS NOT NULL AND
	 provider_response_bytes_known IS NOT NULL AND
     provider_response_bytes>=0 AND provider_response_captured_bytes>=0 AND
     provider_usage_present IS NOT NULL AND provider_usage_valid IS NOT NULL AND
     prompt_eval_count>=0 AND eval_count>=0 AND total_duration_nanos>=0 AND
     load_duration_nanos>=0 AND prompt_eval_duration_nanos>=0 AND eval_duration_nanos>=0)
);

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_v3_response_capture CHECK (
    status IN ('started','abandoned') OR
    (NOT provider_request_dispatched AND NOT provider_identity_checked AND
     provider_observation_sha256 IS NULL AND provider_request_sha256 IS NULL AND
     provider_http_status=0 AND provider_response_disposition IS NULL AND
	 NOT provider_response_complete AND NOT provider_response_bytes_known AND
	 provider_response_sha256 IS NULL AND
     provider_response_bytes=0 AND provider_response_capture_sha256 IS NULL AND
     provider_response_captured_bytes=0 AND NOT provider_usage_present AND
     NOT provider_usage_valid AND prompt_eval_count=0 AND eval_count=0 AND
     total_duration_nanos=0 AND load_duration_nanos=0 AND
     prompt_eval_duration_nanos=0 AND eval_duration_nanos=0) OR
    (provider_request_dispatched AND NOT provider_identity_checked AND
     result_json::jsonb->'provider_generation_evidence'->>'id'<>'' AND
     provider_observation_sha256 IS NULL AND provider_request_sha256 IS NULL AND
     provider_http_status=0 AND provider_response_disposition IS NULL AND
     NOT provider_response_complete AND NOT provider_response_bytes_known AND
     provider_response_sha256 IS NULL AND provider_response_bytes=0 AND
     provider_response_capture_sha256 IS NULL AND provider_response_captured_bytes=0 AND
     provider_done_reason='' AND NOT provider_usage_present AND NOT provider_usage_valid AND
     prompt_eval_count=0 AND eval_count=0 AND total_duration_nanos=0 AND
     load_duration_nanos=0 AND prompt_eval_duration_nanos=0 AND eval_duration_nanos=0) OR
    (provider_request_dispatched AND provider_identity_checked AND
     provider_response_disposition='transport_error' AND provider_http_status=0 AND
	 NOT provider_response_complete AND NOT provider_response_bytes_known AND
	 provider_response_sha256 IS NULL AND
     provider_response_bytes=0 AND provider_response_capture_sha256 IS NULL AND
     provider_response_captured_bytes=0 AND provider_done_reason='') OR
    (provider_request_dispatched AND provider_identity_checked AND
     provider_response_disposition IN ('body_limit','body_read_error') AND
     provider_http_status BETWEEN 100 AND 599 AND NOT provider_response_complete AND
	 NOT provider_response_bytes_known AND provider_response_bytes=0 AND
	 provider_response_sha256 IS NULL AND provider_response_capture_sha256~'^[0-9a-f]{64}$' AND
     provider_done_reason='') OR
    (provider_request_dispatched AND provider_identity_checked AND
     provider_response_disposition IN ('succeeded','http_error','invalid_json','empty_content') AND
	 provider_http_status BETWEEN 100 AND 599 AND provider_response_complete AND
	 provider_response_bytes_known AND
     provider_response_sha256~'^[0-9a-f]{64}$' AND
     provider_response_sha256=provider_response_capture_sha256 AND
     provider_response_bytes=provider_response_captured_bytes AND
     ((provider_response_disposition='succeeded' AND provider_done_reason IN ('stop','length')) OR
      (provider_response_disposition<>'succeeded' AND provider_done_reason='')))
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
