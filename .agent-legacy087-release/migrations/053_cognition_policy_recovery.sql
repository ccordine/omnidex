DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_policy_calls) THEN
        RAISE EXCEPTION 'migration 053 requires cognition_policy_calls to be empty; canonical call authority cannot be backfilled';
    END IF;
END;
$$;

ALTER TABLE cognition_policy_calls
    DROP CONSTRAINT cognition_policy_calls_status_check,
    DROP CONSTRAINT cognition_policy_calls_check;

ALTER TABLE cognition_policy_calls
    ADD COLUMN runtime_budget_sha256 TEXT NOT NULL CHECK (
        runtime_budget_sha256~'^[0-9a-f]{64}$' AND
        runtime_budget_sha256=encode(digest(runtime_budget_json,'sha256'),'hex')
    ),
    ADD COLUMN brain_sha256 TEXT NOT NULL CHECK (
        brain_sha256~'^[0-9a-f]{64}$' AND brain_sha256=encode(digest(brain_json,'sha256'),'hex')
    ),
    ADD CONSTRAINT cognition_policy_calls_status_check
        CHECK (status IN ('started','accepted','rejected','failed','abandoned')),
    ADD CONSTRAINT cognition_policy_calls_result_check CHECK (
        (status='started' AND result_json IS NULL AND result_sha256 IS NULL AND finished_at IS NULL) OR
        (status='abandoned' AND result_json IS NULL AND result_sha256 IS NULL AND finished_at IS NOT NULL) OR
        (status IN ('accepted','rejected','failed') AND jsonb_typeof(result_json::jsonb)='object' AND
         result_sha256~'^[0-9a-f]{64}$' AND
         result_sha256=encode(digest(result_json,'sha256'),'hex') AND finished_at IS NOT NULL)
    );

CREATE OR REPLACE FUNCTION cognition_canonical_jsonb(value JSONB)
RETURNS TEXT AS $$
DECLARE
    rendered TEXT;
BEGIN
    CASE jsonb_typeof(value)
    WHEN 'object' THEN
        SELECT '{'||COALESCE(string_agg(
            to_jsonb(object_key)::TEXT||':'||cognition_canonical_jsonb(value->object_key),
            ',' ORDER BY convert_to(object_key,'UTF8')
        ),'')||'}'
        INTO rendered
        FROM jsonb_object_keys(value) AS object_key;
    WHEN 'array' THEN
        SELECT '['||COALESCE(string_agg(
            cognition_canonical_jsonb(element),',' ORDER BY ordinal
        ),'')||']'
        INTO rendered
        FROM jsonb_array_elements(value) WITH ORDINALITY AS items(element,ordinal);
    ELSE
        rendered := value::TEXT;
    END CASE;
    RETURN rendered;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_exact_attempt_check CHECK (
    call_id='cognition_call_'||encode(digest(
        cognition_canonical_jsonb(attempt_json::jsonb-'id'),'sha256'
    ),'hex') AND
    attempt_json::jsonb->>'schema'='omnidex.cognition-policy-call-attempt.v2' AND
    attempt_json::jsonb->>'id'=call_id AND
    attempt_json::jsonb->'actor'->>'job_id'=job_id::TEXT AND
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

CREATE TABLE cognition_policy_call_abandonments (
    abandonment_id TEXT PRIMARY KEY CHECK (
        abandonment_id~'^cognition_call_abandonment_[0-9a-f]{64}$'
    ),
    abandonment_sha256 TEXT NOT NULL CHECK (
        abandonment_sha256~'^[0-9a-f]{64}$' AND
        abandonment_id='cognition_call_abandonment_'||abandonment_sha256
    ),
    source_call_id TEXT NOT NULL UNIQUE,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    source_attempt BIGINT NOT NULL CHECK (source_attempt>0),
    source_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(source_worker_id)),
    source_attempt_sha256 TEXT NOT NULL CHECK (source_attempt_sha256~'^[0-9a-f]{64}$'),
    source_snapshot_sha256 TEXT NOT NULL CHECK (source_snapshot_sha256~'^[0-9a-f]{64}$'),
    source_disposition TEXT NOT NULL CHECK (source_disposition IN ('expired','superseded')),
    recovery_attempt BIGINT NOT NULL CHECK (recovery_attempt>source_attempt),
    recovery_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(recovery_worker_id)),
    call_ordinal BIGINT NOT NULL CHECK (call_ordinal>0),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND octet_length(descriptor_json)<=262144
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256~'^[0-9a-f]{64}$' AND
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (source_call_id,episode_id,job_id,generation,step_id,source_attempt,source_worker_id)
        REFERENCES cognition_policy_calls(
            call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (source_snapshot_sha256)
        REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,source_attempt,source_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,recovery_attempt,recovery_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    UNIQUE (source_call_id,recovery_attempt,recovery_worker_id)
);

CREATE OR REPLACE FUNCTION guard_cognition_policy_call_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.call_id,NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id,NEW.snapshot_sha256,NEW.projection_id,
           NEW.working_set_id,NEW.expected_revision,NEW.expected_revision_sha256,
           NEW.obligation_node_id,NEW.runtime_budget_json,NEW.runtime_budget_sha256,
           NEW.brain_json,NEW.brain_sha256,NEW.attempt_json,NEW.attempt_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.call_id,OLD.episode_id,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.step_attempt,OLD.worker_id,OLD.snapshot_sha256,OLD.projection_id,
           OLD.working_set_id,OLD.expected_revision,OLD.expected_revision_sha256,
           OLD.obligation_node_id,OLD.runtime_budget_json,OLD.runtime_budget_sha256,
           OLD.brain_json,OLD.brain_sha256,OLD.attempt_json,OLD.attempt_sha256,OLD.created_at) OR
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
          AND snapshots.runtime_budget_json::jsonb=NEW.runtime_budget_json::jsonb
          AND episodes.status='active'
          AND episodes.current_revision=NEW.expected_revision
          AND episodes.current_revision_sha256=NEW.expected_revision_sha256
          AND episodes.attested_brain_json::jsonb->'brain'=NEW.brain_json::jsonb
          AND episodes.attested_brain_json::jsonb->'provider_attestation'=NEW.attempt_json::jsonb->'provider_attestation'
          AND episodes.attested_brain_json::jsonb->'host_hardware_attestation'=NEW.attempt_json::jsonb->'host_hardware_attestation'
    ) THEN
        RAISE EXCEPTION 'cognition policy call has no exact snapshot, budget, or Brain authority';
    END IF;
    IF NEW.status IN ('accepted','rejected','failed') AND NOT (
        NEW.result_json::jsonb->>'schema'='omnidex.cognition-policy-call-result.v2' AND
        NEW.result_json::jsonb->>'call_id'=NEW.call_id AND
        NEW.result_json::jsonb->>'status'=NEW.status AND
        ((NEW.status='accepted' AND NEW.result_json::jsonb->>'failure_code' IS NULL) OR
         (NEW.status<>'accepted' AND task_ledger_text_is_exact(NEW.result_json::jsonb->>'failure_code')))
    ) THEN
        RAISE EXCEPTION 'cognition policy result does not project its exact call status';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_exact_authority
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_policy_call_authority();

CREATE OR REPLACE FUNCTION require_exact_cognition_policy_call_abandonment()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_policy_calls calls
        JOIN cognition_runtime_snapshots snapshots
          ON snapshots.snapshot_sha256=calls.snapshot_sha256
        JOIN job_step_attempts source_attempt
          ON source_attempt.job_id=calls.job_id
         AND source_attempt.generation=calls.generation
         AND source_attempt.step_id=calls.step_id
         AND source_attempt.attempt=calls.step_attempt
         AND source_attempt.worker_id=calls.worker_id
        JOIN job_step_attempts recovery_attempt
          ON recovery_attempt.job_id=NEW.job_id
         AND recovery_attempt.generation=NEW.generation
         AND recovery_attempt.step_id=NEW.step_id
         AND recovery_attempt.attempt=NEW.recovery_attempt
         AND recovery_attempt.worker_id=NEW.recovery_worker_id
        JOIN jobs ON jobs.id=NEW.job_id
        JOIN job_steps steps ON steps.job_id=NEW.job_id AND steps.id=NEW.step_id
        JOIN cognition_episodes episodes ON episodes.episode_id=NEW.episode_id
        WHERE calls.call_id=NEW.source_call_id
          AND calls.status='abandoned' AND calls.result_json IS NULL
          AND calls.episode_id=NEW.episode_id AND calls.job_id=NEW.job_id
          AND calls.generation=NEW.generation AND calls.step_id=NEW.step_id
          AND calls.step_attempt=NEW.source_attempt AND calls.worker_id=NEW.source_worker_id
          AND calls.attempt_sha256=NEW.source_attempt_sha256
          AND calls.snapshot_sha256=NEW.source_snapshot_sha256
          AND snapshots.call_ordinal=NEW.call_ordinal
          AND source_attempt.status=NEW.source_disposition
          AND recovery_attempt.status='active' AND recovery_attempt.expires_at>clock_timestamp()
          AND jobs.status='running' AND jobs.current_generation=NEW.generation
          AND steps.status='running' AND steps.generation=NEW.generation
          AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.recovery_attempt
          AND steps.worker_id=NEW.recovery_worker_id
          AND episodes.status='active'
          AND episodes.current_revision=calls.expected_revision
          AND episodes.current_revision_sha256=calls.expected_revision_sha256
          AND NEW.descriptor_json::jsonb->>'schema'='omnidex.cognition-policy-call-abandonment.v1'
          AND NEW.descriptor_json::jsonb->>'id'=NEW.abandonment_id
          AND NEW.descriptor_json::jsonb->>'sha256'=NEW.abandonment_sha256
          AND NEW.descriptor_json::jsonb->>'call_id'=NEW.source_call_id
          AND NEW.descriptor_json::jsonb->>'source_attempt_sha256'=NEW.source_attempt_sha256
          AND NEW.descriptor_json::jsonb->>'source_snapshot_sha256'=NEW.source_snapshot_sha256
          AND NEW.descriptor_json::jsonb->>'source_disposition'=NEW.source_disposition
          AND (NEW.descriptor_json::jsonb->>'call_ordinal')::BIGINT=NEW.call_ordinal
          AND NOT EXISTS (
              SELECT 1 FROM cognition_reconciliations reconciliations
              WHERE reconciliations.policy_call_id=NEW.source_call_id
          )
          AND NOT EXISTS (
              SELECT 1 FROM cognition_actions actions
              WHERE actions.policy_call_id=NEW.source_call_id
          )
    ) THEN
        RAISE EXCEPTION 'cognition policy abandonment has no exact replacement authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_call_abandonments_exact
AFTER INSERT ON cognition_policy_call_abandonments DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_policy_call_abandonment();

CREATE OR REPLACE FUNCTION require_abandoned_cognition_policy_call_disposition()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status='abandoned' AND NOT EXISTS (
        SELECT 1 FROM cognition_policy_call_abandonments abandonments
        WHERE abandonments.source_call_id=NEW.call_id
          AND abandonments.source_attempt_sha256=NEW.attempt_sha256
          AND abandonments.source_snapshot_sha256=NEW.snapshot_sha256
    ) THEN
        RAISE EXCEPTION 'abandoned cognition policy call has no typed disposition';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_abandonment
AFTER UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_abandoned_cognition_policy_call_disposition();

CREATE TRIGGER cognition_policy_call_abandonments_immutable
BEFORE UPDATE OR DELETE ON cognition_policy_call_abandonments
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_policy_call_abandonments_no_truncate
BEFORE TRUNCATE ON cognition_policy_call_abandonments
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
