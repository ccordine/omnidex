CREATE TABLE cognition_accepted_decision_recoveries (
    recovery_id TEXT PRIMARY KEY CHECK (
        recovery_id~'^cognition_recovery_[0-9a-f]{64}$' AND
        recovery_id='cognition_recovery_'||recovery_sha256
    ),
    recovery_sha256 TEXT NOT NULL CHECK (recovery_sha256~'^[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    source_policy_call_id TEXT NOT NULL,
    source_attempt BIGINT NOT NULL CHECK (source_attempt>0),
    source_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(source_worker_id)),
    recovery_attempt BIGINT NOT NULL CHECK (recovery_attempt>0),
    recovery_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(recovery_worker_id)),
    snapshot_sha256 TEXT NOT NULL,
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    graph_version BIGINT NOT NULL CHECK (graph_version>0),
    graph_sha256 TEXT NOT NULL CHECK (graph_sha256~'^[0-9a-f]{64}$'),
    projection_id TEXT NOT NULL,
    obligation_node_id TEXT NOT NULL,
    decision_sha256 TEXT NOT NULL CHECK (decision_sha256~'^[0-9a-f]{64}$'),
    action_schema_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_id)),
    action_schema_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_version)),
    action_schema_sha256 TEXT NOT NULL CHECK (action_schema_sha256~'^[0-9a-f]{64}$'),
    authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(authority_json::jsonb)='object' AND octet_length(authority_json)<=2097152
    ),
    authority_json_sha256 TEXT NOT NULL CHECK (
        authority_json_sha256~'^[0-9a-f]{64}$' AND
        authority_json_sha256=encode(digest(authority_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,source_attempt,source_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,recovery_attempt,recovery_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_policy_call_id,episode_id,job_id,generation,step_id,source_attempt,source_worker_id)
        REFERENCES cognition_policy_calls(call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (snapshot_sha256) REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,graph_version)
        REFERENCES cognition_obligation_graphs(episode_id,graph_version) ON DELETE RESTRICT,
	CHECK (
		recovery_attempt>source_attempt OR
		(recovery_attempt=source_attempt AND recovery_worker_id=source_worker_id)
	),
	CHECK (authority_json::jsonb->>'Schema' IS NOT DISTINCT FROM
	       'omnidex.cognition-accepted-decision-recovery.v1'),
	CHECK (authority_json::jsonb->>'ID' IS NOT DISTINCT FROM recovery_id),
	CHECK (authority_json::jsonb->>'SHA256' IS NOT DISTINCT FROM recovery_sha256),
	CHECK (authority_json::jsonb->>'PolicyCallID' IS NOT DISTINCT FROM source_policy_call_id),
	CHECK (authority_json::jsonb->>'SnapshotSHA256' IS NOT DISTINCT FROM snapshot_sha256),
	CHECK (authority_json::jsonb->>'GraphSHA256' IS NOT DISTINCT FROM graph_sha256),
	CHECK (authority_json::jsonb->>'DecisionSHA256' IS NOT DISTINCT FROM decision_sha256),
	CHECK ((authority_json::jsonb->>'GraphVersion')::BIGINT IS NOT DISTINCT FROM graph_version),
	CHECK (authority_json::jsonb->'Binding'->'episode'->>'id' IS NOT DISTINCT FROM episode_id),
	CHECK ((authority_json::jsonb->'Binding'->'attempt'->>'job_id')::BIGINT IS NOT DISTINCT FROM job_id),
	CHECK ((authority_json::jsonb->'Binding'->'attempt'->>'generation')::BIGINT IS NOT DISTINCT FROM generation),
	CHECK ((authority_json::jsonb->'Binding'->'attempt'->>'step_id')::BIGINT IS NOT DISTINCT FROM step_id),
	CHECK ((authority_json::jsonb->'Binding'->'attempt'->>'attempt')::BIGINT IS NOT DISTINCT FROM recovery_attempt),
	CHECK (authority_json::jsonb->'Binding'->'attempt'->>'worker_id' IS NOT DISTINCT FROM recovery_worker_id),
	CHECK ((authority_json::jsonb->'SourceActor'->>'job_id')::BIGINT IS NOT DISTINCT FROM job_id),
	CHECK ((authority_json::jsonb->'SourceActor'->>'generation')::BIGINT IS NOT DISTINCT FROM generation),
	CHECK ((authority_json::jsonb->'SourceActor'->>'step_id')::BIGINT IS NOT DISTINCT FROM step_id),
	CHECK ((authority_json::jsonb->'SourceActor'->>'attempt')::BIGINT IS NOT DISTINCT FROM source_attempt),
	CHECK (authority_json::jsonb->'SourceActor'->>'worker_id' IS NOT DISTINCT FROM source_worker_id),
	CHECK (authority_json::jsonb->'Projection'->>'id' IS NOT DISTINCT FROM projection_id),
	CHECK (authority_json::jsonb->'ActionSchema'->>'id' IS NOT DISTINCT FROM action_schema_id),
	CHECK (authority_json::jsonb->'ActionSchema'->>'version' IS NOT DISTINCT FROM action_schema_version),
	CHECK (authority_json::jsonb->'ActionSchema'->>'sha256' IS NOT DISTINCT FROM action_schema_sha256),
    UNIQUE (source_policy_call_id,recovery_attempt,recovery_worker_id)
);

CREATE OR REPLACE FUNCTION require_exact_accepted_decision_recovery()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_policy_calls calls
        JOIN cognition_runtime_snapshots snapshots
          ON snapshots.snapshot_sha256=calls.snapshot_sha256
        JOIN cognition_obligation_graphs graphs
          ON graphs.episode_id=snapshots.episode_id
         AND graphs.graph_version=snapshots.graph_version
        JOIN cognition_episodes episodes ON episodes.episode_id=calls.episode_id
        JOIN jobs jobs ON jobs.id=NEW.job_id
        JOIN job_steps steps ON steps.job_id=NEW.job_id AND steps.id=NEW.step_id
        JOIN job_step_attempts recovery_attempt
          ON recovery_attempt.job_id=NEW.job_id
         AND recovery_attempt.generation=NEW.generation
         AND recovery_attempt.step_id=NEW.step_id
         AND recovery_attempt.attempt=NEW.recovery_attempt
         AND recovery_attempt.worker_id=NEW.recovery_worker_id
        WHERE calls.call_id=NEW.source_policy_call_id
          AND calls.status='accepted'
          AND calls.episode_id=NEW.episode_id
          AND calls.job_id=NEW.job_id
          AND calls.generation=NEW.generation
          AND calls.step_id=NEW.step_id
          AND calls.step_attempt=NEW.source_attempt
          AND calls.worker_id=NEW.source_worker_id
          AND calls.snapshot_sha256=NEW.snapshot_sha256
          AND calls.projection_id=NEW.projection_id
          AND calls.expected_revision=NEW.expected_revision
          AND calls.expected_revision_sha256=NEW.expected_revision_sha256
          AND calls.obligation_node_id=NEW.obligation_node_id
          AND calls.result_json::jsonb->>'decision_sha256'=NEW.decision_sha256
          AND calls.result_json::jsonb->'action_schema'->>'id'=NEW.action_schema_id
          AND calls.result_json::jsonb->'action_schema'->>'version'=NEW.action_schema_version
          AND calls.result_json::jsonb->'action_schema'->>'sha256'=NEW.action_schema_sha256
          AND NEW.authority_json::jsonb->'Projection'=calls.attempt_json::jsonb->'context_projection'
          AND NEW.authority_json::jsonb->'ActionSchema'=calls.result_json::jsonb->'action_schema'
          AND snapshots.graph_version=NEW.graph_version
          AND snapshots.graph_sha256=NEW.graph_sha256
          AND graphs.graph_sha256=NEW.graph_sha256
          AND episodes.status='active'
          AND episodes.current_revision=NEW.expected_revision
          AND episodes.current_revision_sha256=NEW.expected_revision_sha256
          AND jobs.status='running' AND jobs.current_generation=NEW.generation
          AND steps.status='running' AND steps.generation=NEW.generation
          AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=NEW.recovery_attempt
          AND steps.worker_id=NEW.recovery_worker_id
          AND recovery_attempt.status='active'
          AND recovery_attempt.expires_at>clock_timestamp()
          AND NOT EXISTS (
              SELECT 1 FROM cognition_actions actions
              WHERE actions.policy_call_id=NEW.source_policy_call_id
          )
    ) THEN
        RAISE EXCEPTION 'accepted decision recovery has no exact stranded policy call';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_accepted_decision_recoveries_exact
AFTER INSERT ON cognition_accepted_decision_recoveries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_accepted_decision_recovery();

ALTER TABLE cognition_actions DROP CONSTRAINT cognition_actions_policy_call_fk;
ALTER TABLE cognition_actions ADD CONSTRAINT cognition_actions_policy_call_fk
    FOREIGN KEY (policy_call_id) REFERENCES cognition_policy_calls(call_id) ON DELETE RESTRICT;

CREATE TRIGGER cognition_accepted_decision_recoveries_immutable
BEFORE UPDATE OR DELETE ON cognition_accepted_decision_recoveries
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_decision_recoveries_no_truncate
BEFORE TRUNCATE ON cognition_accepted_decision_recoveries
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
