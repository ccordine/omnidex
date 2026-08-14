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
    NEW.provider_process_observation_id :=
        NEW.attempt_json::jsonb->'provider_process_activation'->>'observation_id';
    IF NEW.result_json IS NULL THEN
        NEW.provider_identity_checked := NULL;
        NEW.provider_request_disposition := NULL;
        NEW.provider_observation_sha256 := NULL;
        NEW.provider_request_sha256 := NULL;
        NEW.provider_http_status := NULL;
        NEW.provider_response_disposition := NULL;
        NEW.provider_response_complete := NULL;
        NEW.provider_content_encoding_json := NULL;
        NEW.provider_response_bytes_known := NULL;
        NEW.provider_response_sha256 := NULL;
        NEW.provider_response_bytes := NULL;
        NEW.provider_response_capture_sha256 := NULL;
        NEW.provider_response_captured_bytes := NULL;
        NEW.provider_response_model := NULL;
        NEW.provider_done_present := NULL;
        NEW.provider_done := NULL;
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
    NEW.provider_request_disposition := NEW.result_json::jsonb->>'provider_request_disposition';
    NEW.provider_observation_sha256 :=
        NULLIF(NEW.result_json::jsonb->'provider_observation'->>'observation_sha256','');
    NEW.provider_request_sha256 := NEW.result_json::jsonb->>'provider_request_sha256';
    NEW.provider_http_status := (NEW.result_json::jsonb->>'provider_http_status')::INTEGER;
    NEW.provider_response_disposition := NEW.result_json::jsonb->>'provider_response_disposition';
    NEW.provider_response_complete :=
        (NEW.result_json::jsonb->>'provider_response_complete')::BOOLEAN;
    NEW.provider_content_encoding_json := cognition_canonical_jsonb(
        NEW.result_json::jsonb->'provider_content_encoding'
    );
    NEW.provider_response_bytes_known :=
        (NEW.result_json::jsonb->>'provider_response_bytes_known')::BOOLEAN;
    NEW.provider_response_sha256 := NEW.result_json::jsonb->>'provider_response_sha256';
    NEW.provider_response_bytes := (NEW.result_json::jsonb->>'provider_response_bytes')::BIGINT;
    NEW.provider_response_capture_sha256 :=
        NEW.result_json::jsonb->>'provider_response_capture_sha256';
    NEW.provider_response_captured_bytes :=
        (NEW.result_json::jsonb->>'provider_response_captured_bytes')::BIGINT;
    NEW.provider_response_model := NEW.result_json::jsonb->>'provider_response_model';
    NEW.provider_done_present := (NEW.result_json::jsonb->>'provider_done_present')::BOOLEAN;
    NEW.provider_done := (NEW.result_json::jsonb->>'provider_done')::BOOLEAN;
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
