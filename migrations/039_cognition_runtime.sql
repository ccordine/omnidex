ALTER TABLE job_step_attempts ADD CONSTRAINT job_step_attempts_exact_actor_unique
    UNIQUE (job_id,generation,step_id,attempt,worker_id);

CREATE TABLE cognition_episodes (
    episode_id TEXT PRIMARY KEY CHECK (
        task_ledger_text_is_exact(episode_id) AND octet_length(episode_id)<=256
    ),
    schema_name TEXT NOT NULL CHECK (schema_name='omnidex.cognition-episode.v1'),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    created_attempt BIGINT NOT NULL CHECK (created_attempt>0),
    created_worker_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(created_worker_id) AND octet_length(created_worker_id)<=256
    ),
    ledger_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    scenario_id TEXT NOT NULL CHECK (
        task_ledger_text_is_exact(scenario_id) AND octet_length(scenario_id)<=256
    ),
    scenario_sha256 TEXT NOT NULL CHECK (scenario_sha256~'^[0-9a-f]{64}$'),
    goal_json TEXT NOT NULL CHECK (
        jsonb_typeof(goal_json::jsonb)='object' AND octet_length(goal_json)<=262144
    ),
    goal_sha256 TEXT NOT NULL CHECK (
        goal_sha256~'^[0-9a-f]{64}$' AND goal_sha256=encode(digest(goal_json,'sha256'),'hex')
    ),
    action_catalog_json TEXT NOT NULL CHECK (
        jsonb_typeof(action_catalog_json::jsonb)='object' AND octet_length(action_catalog_json)<=1048576
    ),
    action_catalog_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_catalog_id)),
    action_catalog_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_catalog_version)),
    action_catalog_sha256 TEXT NOT NULL CHECK (action_catalog_sha256~'^[0-9a-f]{64}$'),
    runtime_budget_json TEXT NOT NULL CHECK (
        jsonb_typeof(runtime_budget_json::jsonb)='object' AND octet_length(runtime_budget_json)<=16384
    ),
    runtime_budget_sha256 TEXT NOT NULL CHECK (
        runtime_budget_sha256~'^[0-9a-f]{64}$' AND
        runtime_budget_sha256=encode(digest(runtime_budget_json,'sha256'),'hex')
    ),
    current_revision BIGINT NOT NULL CHECK (current_revision>0),
    current_revision_sha256 TEXT NOT NULL CHECK (current_revision_sha256~'^[0-9a-f]{64}$'),
    status TEXT NOT NULL CHECK (status IN ('active','completed','failed','canceled')),
    action_count BIGINT NOT NULL DEFAULT 0 CHECK (action_count>=0),
    total_cost BIGINT NOT NULL DEFAULT 0 CHECK (total_cost>=0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
    terminal_outcome TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    terminal_at TIMESTAMPTZ,
    FOREIGN KEY (job_id,generation,step_id,created_attempt,created_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,job_id) REFERENCES task_ledgers(id,job_id) ON DELETE RESTRICT,
    FOREIGN KEY (working_set_id,job_id,generation)
        REFERENCES working_sets(id,job_id,generation) ON DELETE RESTRICT,
    UNIQUE (job_id,generation,step_id),
    UNIQUE (episode_id,job_id,generation),
    CHECK ((status='active' AND terminal_outcome IS NULL AND terminal_at IS NULL) OR
           (status<>'active' AND task_ledger_text_is_exact(terminal_outcome) AND terminal_at IS NOT NULL)),
    CHECK (updated_at>=created_at)
);
CREATE INDEX idx_cognition_episodes_status
    ON cognition_episodes(status,updated_at,episode_id);

CREATE OR REPLACE FUNCTION guard_cognition_episode_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.episode_id,NEW.schema_name,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.created_attempt,NEW.created_worker_id,NEW.ledger_id,NEW.working_set_id,
           NEW.scenario_id,NEW.scenario_sha256,NEW.goal_json,NEW.goal_sha256,
           NEW.action_catalog_json,NEW.action_catalog_id,NEW.action_catalog_version,
           NEW.action_catalog_sha256,NEW.runtime_budget_json,NEW.runtime_budget_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.episode_id,OLD.schema_name,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.created_attempt,OLD.created_worker_id,OLD.ledger_id,OLD.working_set_id,
           OLD.scenario_id,OLD.scenario_sha256,OLD.goal_json,OLD.goal_sha256,
           OLD.action_catalog_json,OLD.action_catalog_id,OLD.action_catalog_version,
           OLD.action_catalog_sha256,OLD.runtime_budget_json,OLD.runtime_budget_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'cognition episode authority is immutable';
    END IF;
    IF OLD.status<>'active' THEN RAISE EXCEPTION 'terminal cognition episode is immutable'; END IF;
    IF NEW.version<>OLD.version+1 OR NOT (
        (NEW.current_revision=OLD.current_revision+1 AND
         NEW.action_count=OLD.action_count+1 AND NEW.total_cost>=OLD.total_cost) OR
        (NEW.current_revision=OLD.current_revision AND NEW.status<>'active' AND
         NEW.action_count=OLD.action_count AND NEW.total_cost=OLD.total_cost)
    ) THEN
        RAISE EXCEPTION 'cognition episode progress must be one transition or one terminal seal';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_episodes_update_guard
BEFORE UPDATE ON cognition_episodes FOR EACH ROW EXECUTE FUNCTION guard_cognition_episode_update();

CREATE TABLE cognition_obligations (
    episode_id TEXT NOT NULL,
    ledger_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    node_id TEXT NOT NULL,
    parent_node_id TEXT,
    created_generation BIGINT NOT NULL CHECK (created_generation>0),
    desired_json TEXT NOT NULL CHECK (
        jsonb_typeof(desired_json::jsonb)='object' AND octet_length(desired_json)<=262144
    ),
    desired_sha256 TEXT NOT NULL CHECK (
        desired_sha256~'^[0-9a-f]{64}$' AND desired_sha256=encode(digest(desired_json,'sha256'),'hex')
    ),
    completion_check_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(completion_check_id)),
    completion_check_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(completion_check_version)),
    completion_check_sha256 TEXT NOT NULL CHECK (completion_check_sha256~'^[0-9a-f]{64}$'),
    supporting_refs_json TEXT NOT NULL CHECK (
        jsonb_typeof(supporting_refs_json::jsonb)='array' AND octet_length(supporting_refs_json)<=1048576
    ),
    supporting_refs_sha256 TEXT NOT NULL CHECK (
        supporting_refs_sha256~'^[0-9a-f]{64}$' AND
        supporting_refs_sha256=encode(digest(supporting_refs_json,'sha256'),'hex')
    ),
    created_version BIGINT NOT NULL CHECK (created_version>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (episode_id,node_id),
    FOREIGN KEY (episode_id,job_id,created_generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,node_id) REFERENCES task_nodes(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,parent_node_id) REFERENCES task_nodes(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,job_id) REFERENCES task_ledgers(id,job_id) ON DELETE RESTRICT,
    UNIQUE (ledger_id,node_id),
    CHECK (parent_node_id IS NULL OR parent_node_id<>node_id)
);

CREATE TABLE cognition_policy_evidence (
    call_id TEXT PRIMARY KEY CHECK (call_id~'^cognition_call_[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL,
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    projection_id TEXT NOT NULL,
    working_set_id TEXT NOT NULL,
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    obligation_node_id TEXT NOT NULL,
    snapshot_sha256 TEXT NOT NULL CHECK (snapshot_sha256~'^[0-9a-f]{64}$'),
    decision_sha256 TEXT NOT NULL CHECK (decision_sha256~'^[0-9a-f]{64}$'),
    brain_json TEXT NOT NULL CHECK (
        jsonb_typeof(brain_json::jsonb)='object' AND octet_length(brain_json)<=65536
    ),
    evidence_json TEXT NOT NULL CHECK (
        jsonb_typeof(evidence_json::jsonb)='object' AND octet_length(evidence_json)<=2097152
    ),
    evidence_sha256 TEXT NOT NULL CHECK (
        evidence_sha256~'^[0-9a-f]{64}$' AND evidence_sha256=encode(digest(evidence_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,obligation_node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    FOREIGN KEY (projection_id,working_set_id,job_id,generation)
        REFERENCES context_projections(projection_id,working_set_id,job_id,generation) ON DELETE RESTRICT,
    UNIQUE (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
);

CREATE OR REPLACE FUNCTION validate_cognition_policy_projection()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM context_projections projections
        WHERE projections.projection_id=NEW.projection_id AND projections.job_id=NEW.job_id
          AND projections.generation=NEW.generation AND projections.step_id=NEW.step_id
          AND projections.step_attempt=NEW.step_attempt AND projections.worker_id=NEW.worker_id) THEN
        RAISE EXCEPTION 'cognition policy evidence has no exact context projection authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_policy_projection_validate BEFORE INSERT ON cognition_policy_evidence
FOR EACH ROW EXECUTE FUNCTION validate_cognition_policy_projection();

CREATE OR REPLACE FUNCTION prevent_cognition_immutable_mutation()
RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'cognition evidence and authority rows are immutable'; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_obligations_immutable BEFORE UPDATE OR DELETE ON cognition_obligations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_policy_evidence_immutable BEFORE UPDATE OR DELETE ON cognition_policy_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episodes_delete_immutable BEFORE DELETE ON cognition_episodes
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_episodes_no_truncate BEFORE TRUNCATE ON cognition_episodes
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligations_no_truncate BEFORE TRUNCATE ON cognition_obligations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_policy_evidence_no_truncate BEFORE TRUNCATE ON cognition_policy_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
