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
           NEW.expected_provider_request_sha256,NEW.provider_process_observation_id,
           NEW.created_at)
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
           OLD.expected_provider_request_sha256,OLD.provider_process_observation_id,
           OLD.created_at) OR
       OLD.status<>'started' OR NEW.status='started' THEN
        RAISE EXCEPTION 'cognition policy call transition or identity is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_exact_cognition_policy_call_authority()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_runtime_snapshots snapshots
        JOIN cognition_episodes episodes ON episodes.episode_id=NEW.episode_id
        JOIN context_projections projections
          ON projections.projection_id=snapshots.projection_id
        JOIN cognition_provider_process_observations activation
          ON activation.observation_id=NEW.provider_process_observation_id
         AND activation.episode_id=NEW.episode_id
	        JOIN cognition_provider_identity_evidence activation_evidence
	          ON activation_evidence.evidence_id=activation.evidence_id
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
          AND activation.job_id=NEW.job_id AND activation.generation=NEW.generation
          AND activation.step_id=NEW.step_id AND activation.step_attempt=NEW.step_attempt
          AND activation.worker_id=NEW.worker_id AND activation.created_at<=NEW.created_at
          AND activation.stable_brain_sha256=
              NEW.attempt_json::jsonb->'provider_process_activation'->>'stable_brain_sha256'
          AND activation.provider_observation_sha256=
              NEW.attempt_json::jsonb->'provider_process_activation'->>
                  'provider_observation_sha256'
          AND activation.evidence_id=
              NEW.attempt_json::jsonb->'provider_process_activation'->'evidence'->>'id'
	          AND activation_evidence.ref_json::jsonb=
	              NEW.attempt_json::jsonb->'provider_process_activation'->'evidence'
	    ) THEN
        RAISE EXCEPTION 'cognition policy call has no exact snapshot, Brain, or process activation authority';
    END IF;
    IF NEW.status='abandoned' THEN
        IF NOT EXISTS (
            SELECT 1 FROM cognition_policy_call_abandonments abandonments
            JOIN job_step_attempts source
              ON source.job_id=NEW.job_id AND source.generation=NEW.generation
             AND source.step_id=NEW.step_id AND source.attempt=NEW.step_attempt
             AND source.worker_id=NEW.worker_id
            JOIN job_step_attempts recovery
              ON recovery.job_id=NEW.job_id AND recovery.generation=NEW.generation
             AND recovery.step_id=NEW.step_id
             AND recovery.attempt=abandonments.recovery_attempt
             AND recovery.worker_id=abandonments.recovery_worker_id
            JOIN jobs ON jobs.id=NEW.job_id
            JOIN job_steps steps ON steps.job_id=NEW.job_id AND steps.id=NEW.step_id
            WHERE abandonments.source_call_id=NEW.call_id
              AND abandonments.episode_id=NEW.episode_id
              AND abandonments.source_attempt=NEW.step_attempt
              AND abandonments.source_worker_id=NEW.worker_id
              AND abandonments.source_attempt_sha256=NEW.attempt_sha256
              AND abandonments.source_snapshot_sha256=NEW.snapshot_sha256
              AND abandonments.source_disposition IN ('expired','superseded')
              AND source.status=abandonments.source_disposition
              AND ROW(abandonments.recovery_attempt,abandonments.recovery_worker_id)
                  IS DISTINCT FROM ROW(NEW.step_attempt,NEW.worker_id)
              AND recovery.status='active' AND recovery.expires_at>clock_timestamp()
              AND jobs.status='running' AND jobs.current_generation=NEW.generation
              AND steps.status='running' AND steps.generation=NEW.generation
              AND steps.superseded_at_generation IS NULL
              AND steps.current_attempt=abandonments.recovery_attempt
              AND steps.worker_id=abandonments.recovery_worker_id
              AND abandonments.descriptor_json::jsonb->>'schema'=
                  'omnidex.cognition-policy-call-abandonment.v1'
              AND abandonments.descriptor_json::jsonb->>'call_id'=NEW.call_id
              AND abandonments.descriptor_json::jsonb->>'source_disposition'=
                  abandonments.source_disposition
        ) THEN
            RAISE EXCEPTION 'abandoned cognition policy call lacks exact replacement authority';
        END IF;
        RETURN NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM jobs
        JOIN job_steps steps ON steps.job_id=NEW.job_id AND steps.id=NEW.step_id
        JOIN job_step_attempts actor
          ON actor.job_id=NEW.job_id AND actor.generation=NEW.generation
         AND actor.step_id=NEW.step_id AND actor.attempt=NEW.step_attempt
         AND actor.worker_id=NEW.worker_id
        WHERE jobs.id=NEW.job_id AND jobs.status='running'
          AND jobs.current_generation=NEW.generation AND steps.status='running'
          AND steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id
          AND actor.status='active' AND actor.expires_at>clock_timestamp()
    ) THEN
        RAISE EXCEPTION 'cognition policy call actor is no longer authoritative';
    END IF;
    IF NEW.status='started' THEN RETURN NULL; END IF;
	IF NOT cognition_json_has_unique_keys(NEW.result_json::json) OR
	   NEW.result_json<>cognition_canonical_jsonb(NEW.result_json::jsonb) OR
	   NOT cognition_call_result_v3_shape_is_exact(NEW.result_json) OR
	   NEW.result_json::jsonb->>'call_id'<>NEW.call_id OR
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
	       NEW.result_json::jsonb->'provider_observation'->'evidence'<>
	           NEW.result_json::jsonb->'provider_identity_evidence' OR
           NEW.provider_observation_sha256<>encode(digest(cognition_canonical_jsonb(jsonb_set(
               NEW.result_json::jsonb->'provider_observation',
               '{observation_sha256}',to_jsonb(''::TEXT)
           )),'sha256'),'hex') OR
	       NEW.result_json::jsonb->'provider_observation'->>'challenge_sha256'<>
	           cognition_call_provider_challenge(NEW.call_id,NEW.brain_json::jsonb) OR
           COALESCE(NEW.result_json::jsonb->'provider_observation'->>'observed_at','') !~ 'Z$' OR
           (NEW.result_json::jsonb->'provider_observation'->>'observed_at')::TIMESTAMPTZ<
               NEW.created_at OR
           (NEW.result_json::jsonb->'provider_observation'->>'observed_at')::TIMESTAMPTZ>
               NEW.finished_at OR EXISTS (
               SELECT 1 FROM jsonb_each_text(NEW.result_json::jsonb->'provider_observation') fields
               WHERE fields.key IN (
                   'attestation_sha256','version_body_sha256','installed_body_sha256',
	                   'tokenizer_request_sha256','tokenizer_body_sha256','preload_body_sha256',
	                   'runner_body_sha256','preload_request_sha256',
                   'challenge_sha256','observation_sha256'
               ) AND fields.value !~ '^[0-9a-f]{64}$'
           ) THEN
            RAISE EXCEPTION 'cognition policy result has forged checked provider authority';
        END IF;
	ELSIF NEW.result_json::jsonb->'provider_attestation'<>
	      '{"schema":"","backend":"","backend_version":"","model":"","digest":"","quantization":"","native_context_limit":0,"tokenizer_profile":"","backend_evidence":"","installed_evidence":"","runner_evidence":"","attestation_sha256":""}'::jsonb OR
	      NEW.result_json::jsonb->'provider_observation'<>
	      '{"schema":"","observed_at":"0001-01-01T00:00:00Z","attestation_sha256":"","version_body_sha256":"","installed_body_sha256":"","tokenizer_request_sha256":"","tokenizer_body_sha256":"","preload_body_sha256":"","runner_body_sha256":"","preload_method":"","preload_endpoint":"","preload_request_sha256":"","challenge_sha256":"","evidence":{"schema":"","id":"","sha256":"","bytes":0},"observation_sha256":""}'::jsonb THEN
		RAISE EXCEPTION 'unchecked cognition policy result claims provider identity evidence';
	END IF;
	    IF NOT cognition_policy_terminal_result_is_exact(
	        NEW.result_json::jsonb,NEW.runtime_budget_json::jsonb,
	        NEW.brain_json::jsonb,NEW.expected_provider_request_sha256
	    ) THEN
	        RAISE EXCEPTION 'terminal cognition call result lacks exact registered semantics';
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
