DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_episodes) THEN
        RAISE EXCEPTION 'migration 060 cannot rewrite legacy cognition episode or provider authority evidence';
    END IF;
    IF EXISTS (SELECT 1 FROM cognition_policy_calls) THEN
        RAISE EXCEPTION 'migration 060 cannot fabricate exact provider usage for legacy cognition policy calls';
    END IF;
END;
$$;

ALTER TABLE cognition_runtime_snapshots
    ADD COLUMN policy_envelope_renderer_version TEXT NOT NULL CHECK (
        policy_envelope_renderer_version='omnidex.cognition-policy-renderer.v2'
    ),
    ADD COLUMN policy_envelope_token_estimator TEXT NOT NULL CHECK (
        policy_envelope_token_estimator='utf8-bytes-div-four.v1'
    ),
    ADD COLUMN policy_envelope_estimated_tokens BIGINT NOT NULL CHECK (
        policy_envelope_estimated_tokens>0
    ),
    ADD COLUMN policy_envelope_sha256 TEXT NOT NULL CHECK (
        policy_envelope_sha256~'^[0-9a-f]{64}$'
    ),
    ADD COLUMN policy_envelope_bytes BIGINT NOT NULL CHECK (policy_envelope_bytes>0),
    ADD COLUMN policy_prompt_hint_sha256 TEXT NOT NULL CHECK (
        policy_prompt_hint_sha256~'^[0-9a-f]{64}$'
    ),
    ADD COLUMN policy_prompt_hint_bytes BIGINT NOT NULL CHECK (policy_prompt_hint_bytes>0),
    ADD COLUMN policy_model_visible_input_sha256 TEXT NOT NULL CHECK (
        policy_model_visible_input_sha256~'^[0-9a-f]{64}$'
    ),
    ADD COLUMN policy_model_visible_input_bytes BIGINT NOT NULL CHECK (
        policy_model_visible_input_bytes=policy_envelope_bytes+1+policy_prompt_hint_bytes
    ),
    ADD COLUMN policy_model_visible_estimated_tokens BIGINT NOT NULL CHECK (
        policy_model_visible_estimated_tokens=(policy_model_visible_input_bytes+3)/4
    ),
    ADD COLUMN policy_model_input_token_upper_bound BIGINT NOT NULL CHECK (
        policy_model_input_token_upper_bound=policy_model_visible_input_bytes+2
    ),
    ADD COLUMN policy_response_contract_sha256 TEXT NOT NULL CHECK (
        policy_response_contract_sha256~'^[0-9a-f]{64}$'
    ),
    ADD COLUMN policy_expected_provider_request_sha256 TEXT NOT NULL CHECK (
        policy_expected_provider_request_sha256~'^[0-9a-f]{64}$'
    );
