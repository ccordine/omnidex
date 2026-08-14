DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_policy_evidence) THEN
        RAISE EXCEPTION 'migration 045 refuses to discard existing cognition policy evidence';
    END IF;
END;
$$;
DROP TABLE cognition_policy_evidence CASCADE;

CREATE TABLE cognition_policy_calls (
    call_id TEXT PRIMARY KEY CHECK (call_id~'^cognition_call_[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    snapshot_sha256 TEXT NOT NULL UNIQUE,
    projection_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    obligation_node_id TEXT NOT NULL,
    runtime_budget_json TEXT NOT NULL CHECK (
        jsonb_typeof(runtime_budget_json::jsonb)='object' AND octet_length(runtime_budget_json)<=16384
    ),
    brain_json TEXT NOT NULL CHECK (
        jsonb_typeof(brain_json::jsonb)='object' AND octet_length(brain_json)<=65536
    ),
    attempt_json TEXT NOT NULL CHECK (
        jsonb_typeof(attempt_json::jsonb)='object' AND octet_length(attempt_json)<=2097152
    ),
    attempt_sha256 TEXT NOT NULL CHECK (
        attempt_sha256~'^[0-9a-f]{64}$' AND attempt_sha256=encode(digest(attempt_json,'sha256'),'hex')
    ),
    status TEXT NOT NULL CHECK (status IN ('started','accepted','rejected','failed')),
    result_json TEXT,
    result_sha256 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,obligation_node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (snapshot_sha256) REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (projection_id,working_set_id,job_id,generation)
        REFERENCES context_projections(projection_id,working_set_id,job_id,generation) ON DELETE RESTRICT,
    CHECK ((status='started' AND result_json IS NULL AND result_sha256 IS NULL AND finished_at IS NULL) OR
           (status<>'started' AND jsonb_typeof(result_json::jsonb)='object' AND
            result_sha256~'^[0-9a-f]{64}$' AND
            result_sha256=encode(digest(result_json,'sha256'),'hex') AND finished_at IS NOT NULL))
);

ALTER TABLE cognition_policy_calls ADD CONSTRAINT cognition_policy_calls_exact_actor_unique
    UNIQUE (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id);
ALTER TABLE cognition_actions ADD CONSTRAINT cognition_actions_policy_call_fk
    FOREIGN KEY (policy_call_id,episode_id,job_id,generation,step_id,origin_attempt,origin_worker_id)
    REFERENCES cognition_policy_calls(call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
    ON DELETE RESTRICT;
ALTER TABLE cognition_reconciliations ADD CONSTRAINT cognition_reconciliations_policy_call_fk
    FOREIGN KEY (policy_call_id) REFERENCES cognition_policy_calls(call_id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION guard_cognition_policy_call_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.call_id,NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id,NEW.snapshot_sha256,NEW.projection_id,
           NEW.working_set_id,NEW.expected_revision,NEW.expected_revision_sha256,
           NEW.obligation_node_id,NEW.runtime_budget_json,NEW.brain_json,
           NEW.attempt_json,NEW.attempt_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.call_id,OLD.episode_id,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.step_attempt,OLD.worker_id,OLD.snapshot_sha256,OLD.projection_id,
           OLD.working_set_id,OLD.expected_revision,OLD.expected_revision_sha256,
           OLD.obligation_node_id,OLD.runtime_budget_json,OLD.brain_json,
           OLD.attempt_json,OLD.attempt_sha256,OLD.created_at) OR
       OLD.status<>'started' OR NEW.status='started' THEN
        RAISE EXCEPTION 'cognition policy call transition or identity is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_policy_calls_update_guard
BEFORE UPDATE ON cognition_policy_calls FOR EACH ROW EXECUTE FUNCTION guard_cognition_policy_call_update();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_snapshot()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_runtime_snapshots snapshots
        WHERE snapshots.snapshot_sha256=NEW.snapshot_sha256
          AND snapshots.episode_id=NEW.episode_id
          AND snapshots.job_id=NEW.job_id
          AND snapshots.generation=NEW.generation
          AND snapshots.step_id=NEW.step_id
          AND snapshots.actor_attempt=NEW.step_attempt
          AND snapshots.actor_worker_id=NEW.worker_id
          AND snapshots.expected_revision=NEW.expected_revision
          AND snapshots.expected_revision_sha256=NEW.expected_revision_sha256
          AND snapshots.obligation_node_id=NEW.obligation_node_id
          AND snapshots.projection_id=NEW.projection_id
          AND snapshots.working_set_id=NEW.working_set_id
    ) THEN
        RAISE EXCEPTION 'cognition policy call has no exact prepared snapshot';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_snapshot
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_snapshot();

CREATE OR REPLACE FUNCTION require_cognition_action_projection()
RETURNS TRIGGER AS $$
DECLARE expected_count INT; projected_count INT; last_status TEXT;
BEGIN
    expected_count := CASE NEW.status WHEN 'prepared' THEN 1 WHEN 'dispatched' THEN 2 ELSE 3 END;
    SELECT COUNT(*),MAX(status) FILTER (WHERE sequence=expected_count)
      INTO projected_count,last_status FROM cognition_action_events events
    WHERE events.action_id=NEW.action_id AND events.job_id=NEW.job_id
      AND events.generation=NEW.generation AND events.step_id=NEW.step_id;
    IF projected_count<>expected_count OR last_status IS DISTINCT FROM NEW.status THEN
        RAISE EXCEPTION 'cognition action state and immutable events disagree';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM cognition_policy_calls calls
        WHERE calls.call_id=NEW.policy_call_id AND calls.status='accepted'
          AND calls.snapshot_sha256=NEW.snapshot_sha256
          AND calls.projection_id=NEW.projection_id
          AND calls.result_json::jsonb->>'decision_sha256'=NEW.decision_sha256) THEN
        RAISE EXCEPTION 'cognition action has no exact accepted policy call';
    END IF;
    IF NEW.status='succeeded' AND NOT EXISTS (
        SELECT 1 FROM cognition_transitions transitions
        WHERE transitions.episode_id=NEW.episode_id AND transitions.action_id=NEW.action_id
          AND transitions.revision=NEW.result_revision
          AND transitions.current_revision_sha256=NEW.result_revision_sha256
    ) THEN RAISE EXCEPTION 'successful cognition action has no exact transition'; END IF;
    IF NEW.status='failed' AND EXISTS (
        SELECT 1 FROM cognition_transitions transitions WHERE transitions.action_id=NEW.action_id
    ) THEN RAISE EXCEPTION 'failed cognition action cannot own a transition'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_policy_calls_delete_immutable
BEFORE DELETE ON cognition_policy_calls FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_policy_calls_no_truncate
BEFORE TRUNCATE ON cognition_policy_calls FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
