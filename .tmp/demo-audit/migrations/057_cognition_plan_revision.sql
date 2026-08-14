CREATE TABLE cognition_graph_materialization_sources (
    descriptor_id TEXT PRIMARY KEY,
    descriptor_sha256 TEXT NOT NULL CHECK (
        descriptor_sha256~'^[0-9a-f]{64}$' AND descriptor_id LIKE '%'||descriptor_sha256
    ),
    episode_id TEXT NOT NULL REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    reconciliation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    ledger_id TEXT NOT NULL,
    candidate_entry_id TEXT NOT NULL,
    proposal_kind TEXT NOT NULL CHECK (proposal_kind IN ('obligation','plan_revision')),
    proposal_index INTEGER NOT NULL CHECK (proposal_index>=0 AND proposal_index<32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (ledger_id,candidate_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    UNIQUE (descriptor_id,descriptor_sha256),
    UNIQUE (descriptor_id,proposal_kind),
    UNIQUE (episode_id,descriptor_id)
);

INSERT INTO cognition_graph_materialization_sources (
    descriptor_id,descriptor_sha256,episode_id,reconciliation_id,ledger_id,
    candidate_entry_id,proposal_kind,proposal_index
)
SELECT materialization_id,materialization_sha256,episode_id,reconciliation_id,
       ledger_id,candidate_entry_id,'obligation',proposal_index
FROM cognition_obligation_materializations;

ALTER TABLE cognition_proposal_dispositions
    DROP CONSTRAINT cognition_proposal_dispositions_proposal_kind_check,
    DROP CONSTRAINT cognition_proposal_dispositions_source_descriptor_id_fkey,
    ADD CONSTRAINT cognition_proposal_dispositions_proposal_kind_check
        CHECK (proposal_kind IN ('obligation','plan_revision')),
    ADD CONSTRAINT cognition_proposal_dispositions_source_fk
        FOREIGN KEY (source_descriptor_id,source_descriptor_sha256)
        REFERENCES cognition_graph_materialization_sources(descriptor_id,descriptor_sha256)
        ON DELETE RESTRICT;

ALTER TABLE cognition_obligation_graphs
    DROP CONSTRAINT cognition_obligation_graphs_command_id_check,
    DROP CONSTRAINT cognition_obligation_graphs_command_kind_check,
    ADD CONSTRAINT cognition_obligation_graphs_command_id_check CHECK (
        command_id~'^cognition_graph_command_[0-9a-f]{64}$' OR
        command_id~'^cognition_obligation_materialization_[0-9a-f]{64}$' OR
        command_id~'^cognition_plan_revision_[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT cognition_obligation_graphs_command_kind_check CHECK (
        command_kind IN ('initial','fail','satisfy','materialize','plan_revision')
    );

CREATE TABLE cognition_plan_revisions (
    plan_revision_id TEXT PRIMARY KEY CHECK (
        plan_revision_id~'^cognition_plan_revision_[0-9a-f]{64}$'
    ),
    plan_revision_sha256 TEXT NOT NULL CHECK (
        plan_revision_sha256~'^[0-9a-f]{64}$' AND
        plan_revision_id='cognition_plan_revision_'||plan_revision_sha256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    reconciliation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    ledger_id TEXT NOT NULL,
    candidate_entry_id TEXT NOT NULL,
    source_snapshot_sha256 TEXT NOT NULL UNIQUE
        REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    source_decision_sha256 TEXT NOT NULL CHECK (source_decision_sha256~'^[0-9a-f]{64}$'),
    source_proposal_sha256 TEXT NOT NULL CHECK (source_proposal_sha256~'^[0-9a-f]{64}$'),
    proposal_index INTEGER NOT NULL CHECK (proposal_index>=0 AND proposal_index<32),
    expected_graph_version BIGINT NOT NULL CHECK (expected_graph_version>0),
    expected_graph_sha256 TEXT NOT NULL CHECK (expected_graph_sha256~'^[0-9a-f]{64}$'),
    active_obligation_id TEXT NOT NULL,
    previous_generation BIGINT NOT NULL CHECK (previous_generation>0),
    next_generation BIGINT NOT NULL CHECK (next_generation=previous_generation+1),
    root_obligation_id TEXT NOT NULL,
    next_obligation_id TEXT NOT NULL,
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
    FOREIGN KEY (ledger_id,candidate_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,active_obligation_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,expected_graph_version)
        REFERENCES cognition_obligation_graphs(episode_id,graph_version) ON DELETE RESTRICT,
    FOREIGN KEY (plan_revision_id,plan_revision_sha256)
        REFERENCES cognition_graph_materialization_sources(descriptor_id,descriptor_sha256)
        DEFERRABLE INITIALLY DEFERRED,
    CHECK (active_obligation_id<>root_obligation_id),
    CHECK (root_obligation_id<>next_obligation_id),
    CHECK (descriptor_json::jsonb->>'schema'='omnidex.cognition-plan-revision-materialization.v1'),
    CHECK (descriptor_json::jsonb->>'id'=plan_revision_id),
    CHECK (descriptor_json::jsonb->>'sha256'=plan_revision_sha256),
    CHECK ((descriptor_json::jsonb->>'previous_generation')::bigint=previous_generation),
    CHECK ((descriptor_json::jsonb->>'next_generation')::bigint=next_generation),
    CHECK (descriptor_json::jsonb->>'expected_graph_sha256'=expected_graph_sha256),
    CHECK (descriptor_json::jsonb->>'active_obligation_id'=active_obligation_id),
    CHECK (descriptor_json::jsonb->'root'->>'id'=root_obligation_id),
    CHECK (descriptor_json::jsonb->'next'->>'id'=next_obligation_id),
    CHECK (descriptor_json::jsonb->>'result_graph_sha256'=result_graph_sha256)
);

CREATE TABLE cognition_plan_revision_applications (
    plan_revision_id TEXT PRIMARY KEY
        REFERENCES cognition_plan_revisions(plan_revision_id) ON DELETE RESTRICT,
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

CREATE TRIGGER cognition_graph_materialization_sources_immutable
BEFORE UPDATE OR DELETE ON cognition_graph_materialization_sources
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_graph_materialization_sources_no_truncate
BEFORE TRUNCATE ON cognition_graph_materialization_sources
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_plan_revisions_immutable
BEFORE UPDATE OR DELETE ON cognition_plan_revisions
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_plan_revisions_no_truncate
BEFORE TRUNCATE ON cognition_plan_revisions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_plan_revision_applications_immutable
BEFORE UPDATE OR DELETE ON cognition_plan_revision_applications
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_plan_revision_applications_no_truncate
BEFORE TRUNCATE ON cognition_plan_revision_applications
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
