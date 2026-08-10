CREATE OR REPLACE FUNCTION guard_cognition_policy_call_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.call_id,NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id,NEW.snapshot_sha256,NEW.projection_id,
           NEW.working_set_id,NEW.expected_revision,NEW.expected_revision_sha256,
           NEW.obligation_node_id,NEW.runtime_budget_json,NEW.runtime_budget_sha256,
           NEW.brain_json,NEW.brain_sha256,NEW.attempt_json,NEW.attempt_sha256,
           NEW.envelope_renderer_version,NEW.envelope_token_estimator,
           NEW.envelope_estimated_tokens,NEW.envelope_bytes,NEW.envelope_sha256,
           NEW.prompt_hint_sha256,NEW.prompt_hint_bytes,NEW.model_visible_input_sha256,
           NEW.model_visible_input_bytes,NEW.model_visible_estimated_tokens,
           NEW.model_input_token_upper_bound,NEW.response_contract_sha256,
           NEW.expected_provider_request_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.call_id,OLD.episode_id,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.step_attempt,OLD.worker_id,OLD.snapshot_sha256,OLD.projection_id,
           OLD.working_set_id,OLD.expected_revision,OLD.expected_revision_sha256,
           OLD.obligation_node_id,OLD.runtime_budget_json,OLD.runtime_budget_sha256,
           OLD.brain_json,OLD.brain_sha256,OLD.attempt_json,OLD.attempt_sha256,
           OLD.envelope_renderer_version,OLD.envelope_token_estimator,
           OLD.envelope_estimated_tokens,OLD.envelope_bytes,OLD.envelope_sha256,
           OLD.prompt_hint_sha256,OLD.prompt_hint_bytes,OLD.model_visible_input_sha256,
           OLD.model_visible_input_bytes,OLD.model_visible_estimated_tokens,
           OLD.model_input_token_upper_bound,OLD.response_contract_sha256,
           OLD.expected_provider_request_sha256,OLD.created_at) OR
       OLD.status<>'started' OR NEW.status='started' THEN
        RAISE EXCEPTION 'cognition policy call transition or identity is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_exact_cognition_policy_call_authority()
RETURNS TRIGGER AS $$
DECLARE failure_code TEXT;
DECLARE input_limit BIGINT;
DECLARE output_limit BIGINT;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_runtime_snapshots snapshots
        JOIN cognition_episodes episodes ON episodes.episode_id=NEW.episode_id
        JOIN context_projections projections
          ON projections.projection_id=snapshots.projection_id
        WHERE snapshots.snapshot_sha256=NEW.snapshot_sha256
          AND snapshots.episode_id=NEW.episode_id
          AND snapshots.job_id=NEW.job_id AND snapshots.generation=NEW.generation
          AND snapshots.step_id=NEW.step_id AND snapshots.actor_attempt=NEW.step_attempt
          AND snapshots.actor_worker_id=NEW.worker_id
          AND snapshots.expected_revision=NEW.expected_revision
          AND snapshots.expected_revision_sha256=NEW.expected_revision_sha256
          AND snapshots.obligation_node_id=NEW.obligation_node_id
          AND snapshots.projection_id=NEW.projection_id
          AND snapshots.working_set_id=NEW.working_set_id
          AND projections.working_set_id=NEW.working_set_id
          AND projections.rendered_sha256=
              NEW.attempt_json::jsonb->'context_projection'->>'sha256'
          AND projections.working_set_version=
              (NEW.attempt_json::jsonb->'context_projection'->>'working_set_version')::BIGINT
          AND projections.renderer_version=
              NEW.attempt_json::jsonb->'context_projection'->>'renderer_version'
          AND snapshots.runtime_budget_json::jsonb=NEW.runtime_budget_json::jsonb
          AND snapshots.policy_envelope_renderer_version=NEW.envelope_renderer_version
          AND snapshots.policy_envelope_token_estimator=NEW.envelope_token_estimator
          AND snapshots.policy_envelope_estimated_tokens=NEW.envelope_estimated_tokens
          AND snapshots.policy_envelope_sha256=NEW.envelope_sha256
          AND snapshots.policy_envelope_bytes=NEW.envelope_bytes
          AND snapshots.policy_prompt_hint_sha256=NEW.prompt_hint_sha256
          AND snapshots.policy_prompt_hint_bytes=NEW.prompt_hint_bytes
          AND snapshots.policy_model_visible_input_sha256=NEW.model_visible_input_sha256
          AND snapshots.policy_model_visible_input_bytes=NEW.model_visible_input_bytes
          AND snapshots.policy_model_visible_estimated_tokens=NEW.model_visible_estimated_tokens
          AND snapshots.policy_model_input_token_upper_bound=NEW.model_input_token_upper_bound
          AND snapshots.policy_response_contract_sha256=NEW.response_contract_sha256
          AND snapshots.policy_expected_provider_request_sha256=
              NEW.expected_provider_request_sha256
          AND episodes.status='active'
          AND episodes.current_revision=NEW.expected_revision
          AND episodes.current_revision_sha256=NEW.expected_revision_sha256
          AND episodes.attested_brain_json::jsonb->'brain'=NEW.brain_json::jsonb
          AND episodes.attested_brain_json::jsonb->'provider_attestation'=
              NEW.attempt_json::jsonb->'provider_attestation'
          AND episodes.attested_brain_json::jsonb->'host_hardware_attestation'=
              NEW.attempt_json::jsonb->'host_hardware_attestation'
    ) THEN
        RAISE EXCEPTION 'cognition policy call has no exact snapshot, budget, or Brain authority';
    END IF;
    IF NEW.status IN ('started','abandoned') THEN RETURN NULL; END IF;
    IF NOT cognition_json_has_unique_keys(NEW.result_json::json) OR
       NEW.result_json<>cognition_canonical_jsonb(NEW.result_json::jsonb) OR
       NOT cognition_json_object_has_only_keys(NEW.result_json::json,ARRAY[
           'schema','call_id','status','provider_identity_checked','provider_attestation',
           'provider_observation','provider_request_dispatched','provider_request_sha256',
           'provider_http_status','provider_response_disposition','provider_response_complete',
           'provider_response_sha256','provider_response_bytes',
           'provider_response_capture_sha256','provider_response_captured_bytes',
	       'provider_done_reason',
           'provider_usage_present','provider_usage','response_stored','response_sha256',
           'response_bytes','response','action_schema','decision_sha256','failure_code',
           'failure_message'
       ]) OR NOT (NEW.result_json::jsonb ?& ARRAY[
           'schema','call_id','status','provider_identity_checked','provider_attestation',
           'provider_observation','provider_request_dispatched','provider_http_status',
           'provider_response_complete','provider_response_bytes',
	       'provider_response_captured_bytes','provider_done_reason',
	       'provider_usage_present','provider_usage',
           'response_stored','response_bytes','action_schema'
       ]) OR NOT cognition_json_object_has_exact_keys(
           (NEW.result_json::json->'provider_attestation')::json,ARRAY[
               'schema','backend','backend_version','model','digest','quantization',
               'native_context_limit','backend_evidence','installed_evidence','runner_evidence',
               'attestation_sha256'
           ]
       ) OR NOT cognition_json_object_has_exact_keys(
           (NEW.result_json::json->'provider_observation')::json,ARRAY[
               'schema','observed_at','attestation_sha256','version_body_sha256',
               'installed_body_sha256','preload_body_sha256','runner_body_sha256',
               'preload_method','preload_endpoint','preload_request_sha256',
               'challenge_sha256','observation_sha256'
           ]
       ) OR NOT cognition_json_object_has_exact_keys(
           (NEW.result_json::json->'provider_usage')::json,ARRAY[
               'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
               'prompt_eval_duration_nanos','eval_duration_nanos'
           ]
       ) OR NOT cognition_json_object_has_exact_keys(
           (NEW.result_json::json->'action_schema')::json,ARRAY['id','version','sha256']
       ) OR NEW.result_json::jsonb->>'call_id'<>NEW.call_id OR
       NEW.result_json::jsonb->>'status'<>NEW.status OR
       NEW.result_json::jsonb->>'schema'<>'omnidex.cognition-policy-call-result.v3' OR
       NEW.provider_observation_sha256 IS DISTINCT FROM NULLIF(
           NEW.result_json::jsonb->'provider_observation'->>'observation_sha256',''
       ) THEN
        RAISE EXCEPTION 'cognition policy result does not project exact v3 authority';
    END IF;
    IF NEW.provider_identity_checked THEN
        IF NEW.result_json::jsonb->'provider_attestation'<>
               NEW.attempt_json::jsonb->'provider_attestation' OR
           NEW.result_json::jsonb->'provider_observation'->>'attestation_sha256'<>
               NEW.attempt_json::jsonb->'provider_attestation'->>'attestation_sha256' OR
           NEW.provider_observation_sha256<>encode(digest(cognition_canonical_jsonb(jsonb_set(
               NEW.result_json::jsonb->'provider_observation',
               '{observation_sha256}',to_jsonb(''::TEXT)
           )),'sha256'),'hex') OR
           NEW.result_json::jsonb->'provider_observation'->>'challenge_sha256'<>
               encode(digest(cognition_canonical_jsonb(jsonb_build_object(
                   'scope','cognition-policy-call:'||NEW.call_id,
                   'expectation',jsonb_build_object(
                       'backend',NEW.brain_json::jsonb->>'backend',
                       'backend_version',NEW.brain_json::jsonb->>'backend_version',
                       'model',NEW.brain_json::jsonb->>'model',
                       'digest',NEW.brain_json::jsonb->>'digest',
                       'quantization',NEW.brain_json::jsonb->>'quantization',
                       'native_context_limit',
                           (NEW.brain_json::jsonb->>'native_context_limit')::BIGINT
                   )
               )),'sha256'),'hex') OR
           COALESCE(NEW.result_json::jsonb->'provider_observation'->>'observed_at','') !~ 'Z$' OR
           (NEW.result_json::jsonb->'provider_observation'->>'observed_at')::TIMESTAMPTZ<
               NEW.created_at OR
           (NEW.result_json::jsonb->'provider_observation'->>'observed_at')::TIMESTAMPTZ>
               NEW.finished_at OR EXISTS (
               SELECT 1 FROM jsonb_each_text(NEW.result_json::jsonb->'provider_observation') fields
               WHERE fields.key IN (
                   'attestation_sha256','version_body_sha256','installed_body_sha256',
                   'preload_body_sha256','runner_body_sha256','preload_request_sha256',
                   'challenge_sha256','observation_sha256'
               ) AND fields.value !~ '^[0-9a-f]{64}$'
           ) THEN
            RAISE EXCEPTION 'cognition policy result has forged checked provider authority';
        END IF;
    ELSIF NEW.result_json::jsonb->'provider_attestation'<>
          '{"schema":"","backend":"","backend_version":"","model":"","digest":"","quantization":"","native_context_limit":0,"backend_evidence":"","installed_evidence":"","runner_evidence":"","attestation_sha256":""}'::jsonb OR
          NEW.result_json::jsonb->'provider_observation'<>
          '{"schema":"","observed_at":"0001-01-01T00:00:00Z","attestation_sha256":"","version_body_sha256":"","installed_body_sha256":"","preload_body_sha256":"","runner_body_sha256":"","preload_method":"","preload_endpoint":"","preload_request_sha256":"","challenge_sha256":"","observation_sha256":""}'::jsonb THEN
        RAISE EXCEPTION 'unchecked cognition policy result claims provider identity evidence';
    END IF;
    IF (NEW.result_json::jsonb->>'response_stored')::BOOLEAN THEN
        IF NOT (NEW.result_json::jsonb ? 'response') OR
           (NEW.result_json::jsonb->>'response_bytes')::BIGINT<>
               octet_length(NEW.result_json::jsonb->>'response') OR
           NEW.result_json::jsonb->>'response_sha256'<>
               encode(digest(NEW.result_json::jsonb->>'response','sha256'),'hex') THEN
            RAISE EXCEPTION 'stored cognition policy response identity is invalid';
        END IF;
    ELSIF NEW.result_json::jsonb ? 'response' THEN
        RAISE EXCEPTION 'omitted cognition policy response carried content';
    END IF;
    failure_code := COALESCE(NEW.result_json::jsonb->>'failure_code','');
    input_limit := (NEW.runtime_budget_json::jsonb->>'max_input_tokens')::BIGINT;
    output_limit := (NEW.runtime_budget_json::jsonb->>'max_output_tokens')::BIGINT;
    IF NEW.status='accepted' AND NOT (
        failure_code='' AND NEW.provider_identity_checked AND NEW.provider_request_dispatched AND
        NEW.provider_usage_valid AND
        NEW.provider_response_disposition='succeeded' AND
        NEW.prompt_eval_count<=input_limit AND NEW.eval_count<=output_limit
    ) THEN
        RAISE EXCEPTION 'accepted cognition call lacks exact in-budget provider usage';
    ELSIF NEW.status='rejected' AND NOT (
        (failure_code='provider_usage_limit' AND NEW.provider_identity_checked AND
         NEW.provider_request_dispatched AND
         NEW.provider_usage_valid AND NEW.provider_response_disposition='succeeded' AND
         (NEW.prompt_eval_count>input_limit OR NEW.eval_count>output_limit)) OR
        (failure_code='provider_usage_error' AND NEW.provider_identity_checked AND
         NEW.provider_request_dispatched AND
         NOT NEW.provider_usage_valid AND NEW.provider_response_disposition='succeeded') OR
        (failure_code='response_limit' AND NEW.provider_identity_checked AND
         NEW.provider_request_dispatched AND
         NEW.provider_usage_valid AND NEW.provider_response_disposition='succeeded' AND
         NEW.prompt_eval_count<=input_limit AND NEW.eval_count<=output_limit AND
         (NEW.result_json::jsonb->>'response_bytes')::BIGINT>0 AND
         ((NEW.result_json::jsonb->>'response_bytes')::BIGINT>
              (NEW.runtime_budget_json::jsonb->>'max_output_bytes')::BIGINT OR
          ((NEW.result_json::jsonb->>'response_bytes')::BIGINT+3)/4>output_limit)) OR
        (failure_code IN ('invalid_decision','authority_denied') AND
         NEW.provider_identity_checked AND NEW.provider_request_dispatched AND
         NEW.provider_usage_valid AND
         NEW.provider_response_disposition='succeeded' AND
         NEW.prompt_eval_count<=input_limit AND NEW.eval_count<=output_limit)
    ) THEN
        RAISE EXCEPTION 'rejected cognition call has an invalid provider usage disposition';
    ELSIF NEW.status='failed' AND NOT (
        (failure_code='provider_identity_error' AND NOT NEW.provider_identity_checked AND
         NOT NEW.provider_request_dispatched AND NEW.provider_observation_sha256 IS NULL AND
         NEW.provider_request_sha256 IS NULL AND NEW.provider_http_status=0 AND
         NEW.provider_response_disposition IS NULL AND NOT NEW.provider_response_complete AND
         NEW.provider_response_sha256 IS NULL AND NEW.provider_response_bytes=0 AND
         NEW.provider_response_capture_sha256 IS NULL AND
         NEW.provider_response_captured_bytes=0 AND NOT NEW.provider_usage_present AND
         NOT NEW.provider_usage_valid AND NEW.prompt_eval_count=0 AND NEW.eval_count=0 AND
         NEW.total_duration_nanos=0 AND NEW.load_duration_nanos=0 AND
         NEW.prompt_eval_duration_nanos=0 AND NEW.eval_duration_nanos=0) OR
        (failure_code='generation_error' AND NOT NEW.provider_request_dispatched AND
         NOT NEW.provider_identity_checked AND NEW.provider_response_disposition IS NULL) OR
        (failure_code='generation_error' AND NEW.provider_identity_checked AND
         NEW.provider_request_dispatched AND
         NEW.provider_response_disposition IN (
             'transport_error','http_error','body_limit','body_read_error',
             'invalid_json','empty_content','succeeded'
         ))
    ) THEN
        RAISE EXCEPTION 'failed cognition call has an invalid provider invocation disposition';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION require_started_cognition_policy_call_insert()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status<>'started' OR NEW.result_json IS NOT NULL OR
       NEW.result_sha256 IS NOT NULL OR NEW.finished_at IS NOT NULL THEN
        RAISE EXCEPTION 'cognition policy call must be inserted in started state';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_policy_calls_10_started_insert
BEFORE INSERT ON cognition_policy_calls
FOR EACH ROW EXECUTE FUNCTION require_started_cognition_policy_call_insert();
