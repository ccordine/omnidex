CREATE TABLE cognition_runtime_snapshots (
    snapshot_sha256 TEXT PRIMARY KEY CHECK (snapshot_sha256~'^[0-9a-f]{64}$'),
    preparation_id TEXT NOT NULL UNIQUE CHECK (
        preparation_id~'^cognition_snapshot_[0-9a-f]{64}$'
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    call_ordinal BIGINT NOT NULL CHECK (call_ordinal>0),
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    obligation_node_id TEXT NOT NULL,
    graph_version BIGINT NOT NULL CHECK (graph_version>0),
    graph_sha256 TEXT NOT NULL CHECK (graph_sha256~'^[0-9a-f]{64}$'),
    projection_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    runtime_budget_json TEXT NOT NULL CHECK (
        jsonb_typeof(runtime_budget_json::jsonb)='object' AND octet_length(runtime_budget_json)<=16384
    ),
    runtime_budget_sha256 TEXT NOT NULL CHECK (
        runtime_budget_sha256~'^[0-9a-f]{64}$' AND
        runtime_budget_sha256=encode(digest(runtime_budget_json,'sha256'),'hex')
    ),
    evidence_refs_json TEXT NOT NULL CHECK (
        jsonb_typeof(evidence_refs_json::jsonb)='array' AND octet_length(evidence_refs_json)<=1048576
    ),
    evidence_refs_sha256 TEXT NOT NULL CHECK (
        evidence_refs_sha256~'^[0-9a-f]{64}$' AND
        evidence_refs_sha256=encode(digest(evidence_refs_json,'sha256'),'hex')
    ),
    environment_terminal BOOLEAN NOT NULL,
    public_outcome TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,obligation_node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (projection_id,working_set_id,job_id,generation)
        REFERENCES context_projections(projection_id,working_set_id,job_id,generation) ON DELETE RESTRICT,
    UNIQUE (episode_id,job_id,generation,step_id,actor_attempt,actor_worker_id,
            call_ordinal,expected_revision,obligation_node_id),
    CHECK ((environment_terminal AND task_ledger_text_is_exact(public_outcome)) OR
           (NOT environment_terminal AND (public_outcome='' OR task_ledger_text_is_exact(public_outcome))))
);

CREATE TABLE cognition_reconciliations (
    reconciliation_id TEXT PRIMARY KEY CHECK (
        reconciliation_id~'^cognition_reconciliation_[0-9a-f]{64}$'
    ),
    reconciliation_sha256 TEXT NOT NULL CHECK (reconciliation_sha256~'^[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    snapshot_sha256 TEXT NOT NULL UNIQUE,
    policy_call_id TEXT NOT NULL UNIQUE,
    decision_sha256 TEXT NOT NULL CHECK (decision_sha256~'^[0-9a-f]{64}$'),
    action_schema_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_id)),
    action_schema_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_version)),
    action_schema_sha256 TEXT NOT NULL CHECK (action_schema_sha256~'^[0-9a-f]{64}$'),
    ledger_version BIGINT NOT NULL CHECK (ledger_version>0),
    working_set_version BIGINT NOT NULL CHECK (working_set_version>0),
    command_json TEXT NOT NULL CHECK (
        jsonb_typeof(command_json::jsonb)='object' AND octet_length(command_json)<=2097152
    ),
    command_sha256 TEXT NOT NULL CHECK (
        command_sha256~'^[0-9a-f]{64}$' AND command_sha256=encode(digest(command_json,'sha256'),'hex')
    ),
    receipt_json TEXT NOT NULL CHECK (
        jsonb_typeof(receipt_json::jsonb)='object' AND octet_length(receipt_json)<=262144
    ),
    receipt_sha256 TEXT NOT NULL CHECK (
        receipt_sha256~'^[0-9a-f]{64}$' AND receipt_sha256=encode(digest(receipt_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (snapshot_sha256) REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    UNIQUE (reconciliation_id,episode_id,job_id,generation,step_id)
);

CREATE TABLE cognition_episode_progress (
    command_id TEXT PRIMARY KEY REFERENCES cognition_obligation_graphs(command_id) ON DELETE RESTRICT,
    episode_id TEXT NOT NULL,
    source_snapshot_sha256 TEXT NOT NULL REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    input_graph_version BIGINT NOT NULL CHECK (input_graph_version>0),
    output_graph_version BIGINT NOT NULL CHECK (output_graph_version=input_graph_version+1),
    state TEXT NOT NULL CHECK (state IN ('active','completed','failed')),
    command_json TEXT NOT NULL CHECK (
        jsonb_typeof(command_json::jsonb)='object' AND octet_length(command_json)<=2097152
    ),
    command_sha256 TEXT NOT NULL CHECK (
        command_sha256~'^[0-9a-f]{64}$' AND command_sha256=encode(digest(command_json,'sha256'),'hex')
    ),
    progress_json TEXT NOT NULL CHECK (
        jsonb_typeof(progress_json::jsonb)='object' AND octet_length(progress_json)<=2097152
    ),
    progress_sha256 TEXT NOT NULL CHECK (
        progress_sha256~'^[0-9a-f]{64}$' AND progress_sha256=encode(digest(progress_json,'sha256'),'hex')
    ),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,output_graph_version)
        REFERENCES cognition_obligation_graphs(episode_id,graph_version) ON DELETE RESTRICT,
    UNIQUE (episode_id,output_graph_version),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT
);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_actions) THEN
        RAISE EXCEPTION 'migration 044 requires cognition_actions to be empty; reconciliation authority cannot be backfilled';
    END IF;
END;
$$;
ALTER TABLE cognition_actions
    ADD COLUMN reconciliation_id TEXT NOT NULL REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    ADD COLUMN reconciliation_sha256 TEXT NOT NULL CHECK (reconciliation_sha256~'^[0-9a-f]{64}$');

CREATE OR REPLACE FUNCTION guard_cognition_action_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.action_id,NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.origin_attempt,NEW.origin_worker_id,NEW.obligation_node_id,NEW.policy_call_id,
           NEW.expected_revision,NEW.expected_revision_sha256,NEW.snapshot_sha256,
           NEW.projection_id,NEW.action_schema_id,NEW.action_schema_version,
           NEW.action_schema_sha256,NEW.decision_json,NEW.decision_sha256,
           NEW.registered_action_json,NEW.registered_action_sha256,
           NEW.reconciliation_id,NEW.reconciliation_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.action_id,OLD.episode_id,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.origin_attempt,OLD.origin_worker_id,OLD.obligation_node_id,OLD.policy_call_id,
           OLD.expected_revision,OLD.expected_revision_sha256,OLD.snapshot_sha256,
           OLD.projection_id,OLD.action_schema_id,OLD.action_schema_version,
           OLD.action_schema_sha256,OLD.decision_json,OLD.decision_sha256,
           OLD.registered_action_json,OLD.registered_action_sha256,
           OLD.reconciliation_id,OLD.reconciliation_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'cognition action identity is immutable';
    END IF;
    IF NOT ((OLD.status='prepared' AND NEW.status='dispatched') OR
            (OLD.status='dispatched' AND NEW.status IN ('succeeded','failed'))) THEN
        RAISE EXCEPTION 'cognition action status transition is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_action_reconciliation()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_reconciliations reconciliations
        WHERE reconciliations.reconciliation_id=NEW.reconciliation_id
          AND reconciliations.reconciliation_sha256=NEW.reconciliation_sha256
          AND reconciliations.episode_id=NEW.episode_id
          AND reconciliations.job_id=NEW.job_id
          AND reconciliations.generation=NEW.generation
          AND reconciliations.step_id=NEW.step_id
          AND reconciliations.policy_call_id=NEW.policy_call_id
          AND reconciliations.snapshot_sha256=NEW.snapshot_sha256
          AND reconciliations.decision_sha256=NEW.decision_sha256
          AND reconciliations.action_schema_id=NEW.action_schema_id
          AND reconciliations.action_schema_version=NEW.action_schema_version
          AND reconciliations.action_schema_sha256=NEW.action_schema_sha256
    ) THEN
        RAISE EXCEPTION 'cognition action has no exact reconciliation authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_actions_require_reconciliation
AFTER INSERT ON cognition_actions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_action_reconciliation();

CREATE TRIGGER cognition_runtime_snapshots_immutable
BEFORE UPDATE OR DELETE ON cognition_runtime_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_reconciliations_immutable
BEFORE UPDATE OR DELETE ON cognition_reconciliations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_progress_immutable
BEFORE UPDATE OR DELETE ON cognition_episode_progress
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_runtime_snapshots_no_truncate
BEFORE TRUNCATE ON cognition_runtime_snapshots
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_reconciliations_no_truncate
BEFORE TRUNCATE ON cognition_reconciliations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_progress_no_truncate
BEFORE TRUNCATE ON cognition_episode_progress
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
