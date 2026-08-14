DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_episodes) THEN
        RAISE EXCEPTION 'migration 050 requires cognition_episodes to be empty; completion and prepared-evidence authority cannot be backfilled';
    END IF;
END;
$$;

ALTER TABLE cognition_runtime_snapshots
    ADD COLUMN completion_evidence_refs_json TEXT NOT NULL CHECK (
        jsonb_typeof(completion_evidence_refs_json::jsonb)='array' AND
        octet_length(completion_evidence_refs_json)<=1048576
    ),
    ADD COLUMN completion_evidence_refs_sha256 TEXT NOT NULL CHECK (
        completion_evidence_refs_sha256~'^[0-9a-f]{64}$' AND
        completion_evidence_refs_sha256=encode(digest(completion_evidence_refs_json,'sha256'),'hex')
    );

ALTER TABLE cognition_episodes
    ADD COLUMN completion_authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(completion_authority_json::jsonb)='object' AND
        octet_length(completion_authority_json)<=262144
    ),
    ADD COLUMN completion_authority_sha256 TEXT NOT NULL CHECK (
        completion_authority_sha256~'^[0-9a-f]{64}$' AND
        completion_authority_sha256=encode(digest(completion_authority_json,'sha256'),'hex')
    );

CREATE OR REPLACE FUNCTION guard_cognition_episode_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.episode_id,NEW.schema_name,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.created_attempt,NEW.created_worker_id,NEW.ledger_id,NEW.working_set_id,
           NEW.scenario_id,NEW.scenario_sha256,NEW.goal_json,NEW.goal_sha256,
           NEW.completion_authority_json,NEW.completion_authority_sha256,
           NEW.action_catalog_json,NEW.action_catalog_id,NEW.action_catalog_version,
           NEW.action_catalog_sha256,NEW.runtime_budget_json,NEW.runtime_budget_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.episode_id,OLD.schema_name,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.created_attempt,OLD.created_worker_id,OLD.ledger_id,OLD.working_set_id,
           OLD.scenario_id,OLD.scenario_sha256,OLD.goal_json,OLD.goal_sha256,
           OLD.completion_authority_json,OLD.completion_authority_sha256,
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

ALTER TABLE cognition_obligation_graphs
    DROP CONSTRAINT cognition_obligation_graphs_command_id_check,
    DROP CONSTRAINT cognition_obligation_graphs_command_kind_check;
ALTER TABLE cognition_obligation_graphs
    ADD CONSTRAINT cognition_obligation_graphs_command_id_check CHECK (
        command_id~'^cognition_graph_command_[0-9a-f]{64}$' OR
        command_id~'^cognition_obligation_materialization_[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT cognition_obligation_graphs_command_kind_check CHECK (command_kind IN (
        'initial','fail','satisfy','materialize'
    ));

CREATE TABLE cognition_obligation_materializations (
    materialization_id TEXT PRIMARY KEY CHECK (
        materialization_id~'^cognition_obligation_materialization_[0-9a-f]{64}$'
    ),
    materialization_sha256 TEXT NOT NULL CHECK (
        materialization_sha256~'^[0-9a-f]{64}$' AND
        materialization_id='cognition_obligation_materialization_'||materialization_sha256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    reconciliation_id TEXT NOT NULL UNIQUE REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    source_snapshot_sha256 TEXT NOT NULL UNIQUE REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    source_decision_sha256 TEXT NOT NULL CHECK (source_decision_sha256~'^[0-9a-f]{64}$'),
    source_proposal_sha256 TEXT NOT NULL CHECK (source_proposal_sha256~'^[0-9a-f]{64}$'),
    proposal_index INTEGER NOT NULL CHECK (proposal_index>=0 AND proposal_index<32),
    expected_graph_version BIGINT NOT NULL CHECK (expected_graph_version>0),
    expected_graph_sha256 TEXT NOT NULL CHECK (expected_graph_sha256~'^[0-9a-f]{64}$'),
    active_obligation_id TEXT NOT NULL,
    child_obligation_id TEXT NOT NULL,
    result_graph_sha256 TEXT NOT NULL CHECK (result_graph_sha256~'^[0-9a-f]{64}$'),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND octet_length(descriptor_json)<=2097152
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256~'^[0-9a-f]{64}$' AND
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,active_obligation_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,expected_graph_version)
        REFERENCES cognition_obligation_graphs(episode_id,graph_version) ON DELETE RESTRICT,
    CHECK (active_obligation_id<>child_obligation_id)
);

CREATE TABLE cognition_obligation_materialization_applications (
    materialization_id TEXT PRIMARY KEY REFERENCES cognition_obligation_materializations(materialization_id) ON DELETE RESTRICT,
    episode_id TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE REFERENCES cognition_actions(action_id) ON DELETE RESTRICT,
    input_graph_version BIGINT NOT NULL CHECK (input_graph_version>0),
    output_graph_version BIGINT NOT NULL CHECK (output_graph_version=input_graph_version+1),
    transition_revision BIGINT NOT NULL CHECK (transition_revision>1),
    result_graph_sha256 TEXT NOT NULL CHECK (result_graph_sha256~'^[0-9a-f]{64}$'),
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,output_graph_version)
        REFERENCES cognition_obligation_graphs(episode_id,graph_version) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION require_exact_cognition_obligation_materialization()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_reconciliations reconciliations
        JOIN cognition_runtime_snapshots snapshots
          ON snapshots.snapshot_sha256=reconciliations.snapshot_sha256
        JOIN cognition_episodes episodes ON episodes.episode_id=reconciliations.episode_id
        WHERE reconciliations.reconciliation_id=NEW.reconciliation_id
          AND reconciliations.episode_id=NEW.episode_id
          AND reconciliations.job_id=NEW.job_id
          AND reconciliations.generation=NEW.generation
          AND reconciliations.step_id=NEW.step_id
          AND reconciliations.snapshot_sha256=NEW.source_snapshot_sha256
          AND reconciliations.decision_sha256=NEW.source_decision_sha256
          AND snapshots.graph_version=NEW.expected_graph_version
          AND snapshots.graph_sha256=NEW.expected_graph_sha256
          AND snapshots.obligation_node_id=NEW.active_obligation_id
		  AND episodes.completion_authority_json::jsonb=
		      NEW.descriptor_json::jsonb->'completion_authority'
    ) THEN
        RAISE EXCEPTION 'obligation materialization lacks exact reconciliation authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_obligation_materializations_exact
AFTER INSERT ON cognition_obligation_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_materialization();

CREATE OR REPLACE FUNCTION require_exact_cognition_materialization_application()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_obligation_materializations materializations
        JOIN cognition_actions actions
          ON actions.reconciliation_id=materializations.reconciliation_id
         AND actions.snapshot_sha256=materializations.source_snapshot_sha256
         AND actions.decision_sha256=materializations.source_decision_sha256
        JOIN cognition_obligation_graphs graphs
          ON graphs.episode_id=NEW.episode_id
         AND graphs.graph_version=NEW.output_graph_version
        WHERE materializations.materialization_id=NEW.materialization_id
          AND materializations.episode_id=NEW.episode_id
          AND materializations.expected_graph_version=NEW.input_graph_version
          AND materializations.result_graph_sha256=NEW.result_graph_sha256
          AND actions.action_id=NEW.action_id
          AND actions.status='succeeded'
          AND actions.result_revision=NEW.transition_revision
          AND graphs.command_id=NEW.materialization_id
          AND graphs.command_kind='materialize'
          AND graphs.graph_sha256=NEW.result_graph_sha256
    ) THEN
        RAISE EXCEPTION 'obligation materialization application lacks exact successful action and graph';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_obligation_materialization_applications_exact
AFTER INSERT ON cognition_obligation_materialization_applications DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_materialization_application();

CREATE OR REPLACE FUNCTION require_cognition_action_materialization_application()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status='succeeded' AND EXISTS (
        SELECT 1 FROM cognition_obligation_materializations materializations
        WHERE materializations.reconciliation_id=NEW.reconciliation_id
    ) AND NOT EXISTS (
        SELECT 1 FROM cognition_obligation_materialization_applications applications
        JOIN cognition_obligation_materializations materializations
          ON materializations.materialization_id=applications.materialization_id
        WHERE materializations.reconciliation_id=NEW.reconciliation_id
          AND applications.action_id=NEW.action_id
          AND applications.transition_revision=NEW.result_revision
    ) THEN
        RAISE EXCEPTION 'successful cognition action omitted its obligation materialization';
    END IF;
    IF NEW.status='failed' AND EXISTS (
        SELECT 1 FROM cognition_obligation_materialization_applications applications
        WHERE applications.action_id=NEW.action_id
    ) THEN
        RAISE EXCEPTION 'failed cognition action cannot apply an obligation materialization';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_actions_require_materialization_application
AFTER UPDATE ON cognition_actions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_action_materialization_application();

CREATE TRIGGER cognition_obligation_materializations_immutable
BEFORE UPDATE OR DELETE ON cognition_obligation_materializations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_materializations_no_truncate
BEFORE TRUNCATE ON cognition_obligation_materializations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_materialization_applications_immutable
BEFORE UPDATE OR DELETE ON cognition_obligation_materialization_applications
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_materialization_applications_no_truncate
BEFORE TRUNCATE ON cognition_obligation_materialization_applications
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
