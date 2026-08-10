CREATE TABLE cognition_belief_revisions (
    revision_id TEXT PRIMARY KEY CHECK (
        revision_id~'^cognition_revision_[0-9a-f]{64}$'
    ),
    revision_sha256 TEXT NOT NULL CHECK (
        revision_sha256~'^[0-9a-f]{64}$' AND
        revision_id='cognition_revision_'||revision_sha256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    reconciliation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    source_snapshot_sha256 TEXT NOT NULL UNIQUE
        REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    source_decision_sha256 TEXT NOT NULL CHECK (source_decision_sha256~'^[0-9a-f]{64}$'),
    ledger_id TEXT NOT NULL,
    expected_ledger_sha256 TEXT NOT NULL CHECK (expected_ledger_sha256~'^[0-9a-f]{64}$'),
    expected_ledger_version BIGINT NOT NULL CHECK (expected_ledger_version>0),
    target_uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(target_uri)),
    target_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(target_version)),
    target_sha256 TEXT NOT NULL CHECK (target_sha256~'^[0-9a-f]{64}$'),
    result_ledger_sha256 TEXT NOT NULL CHECK (result_ledger_sha256~'^[0-9a-f]{64}$'),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND
        octet_length(descriptor_json)<=2097152 AND
        descriptor_json::jsonb->>'schema'='omnidex.cognition-state-belief-revision.v1'
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256~'^[0-9a-f]{64}$' AND
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,job_id)
        REFERENCES task_ledgers(id,job_id) ON DELETE RESTRICT,
    CHECK (descriptor_json::jsonb->>'id' IS NOT DISTINCT FROM revision_id),
    CHECK (descriptor_json::jsonb->>'sha256' IS NOT DISTINCT FROM revision_sha256),
    CHECK (descriptor_json::jsonb->>'source_snapshot_sha256' IS NOT DISTINCT FROM source_snapshot_sha256),
    CHECK (descriptor_json::jsonb->>'source_decision_sha256' IS NOT DISTINCT FROM source_decision_sha256),
    CHECK (descriptor_json::jsonb->>'ledger_id' IS NOT DISTINCT FROM ledger_id),
    CHECK (descriptor_json::jsonb->>'expected_ledger_sha256' IS NOT DISTINCT FROM expected_ledger_sha256),
    CHECK ((descriptor_json::jsonb->>'expected_version')::BIGINT IS NOT DISTINCT FROM expected_ledger_version),
    CHECK (descriptor_json::jsonb->'target_ref'->>'uri' IS NOT DISTINCT FROM target_uri),
    CHECK (descriptor_json::jsonb->'target_ref'->>'version' IS NOT DISTINCT FROM target_version),
    CHECK (descriptor_json::jsonb->'target_ref'->>'content_sha256' IS NOT DISTINCT FROM target_sha256),
    CHECK (descriptor_json::jsonb->>'result_ledger_sha256' IS NOT DISTINCT FROM result_ledger_sha256)
);

CREATE OR REPLACE FUNCTION require_exact_cognition_belief_revision()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_reconciliations reconciliations
        JOIN cognition_episodes episodes ON episodes.episode_id=reconciliations.episode_id
        JOIN task_events events
          ON events.ledger_id=NEW.ledger_id
         AND events.command_id=NEW.descriptor_json::jsonb->'rejection'->>'command_id'
        JOIN task_entries entries
          ON entries.ledger_id=NEW.ledger_id
         AND entries.id=NEW.descriptor_json::jsonb->'rejection'->>'entry_id'
        WHERE reconciliations.reconciliation_id=NEW.reconciliation_id
          AND reconciliations.episode_id=NEW.episode_id
          AND reconciliations.job_id=NEW.job_id
          AND reconciliations.generation=NEW.generation
          AND reconciliations.step_id=NEW.step_id
          AND reconciliations.snapshot_sha256=NEW.source_snapshot_sha256
          AND reconciliations.decision_sha256=NEW.source_decision_sha256
          AND reconciliations.ledger_version=NEW.expected_ledger_version+1
          AND episodes.ledger_id=NEW.ledger_id
          AND events.job_id=NEW.job_id
          AND events.ledger_version=NEW.expected_ledger_version+1
          AND events.command_kind='reject_entry'
          AND events.event_kind='entry_rejected'
          AND events.actor='code'
          AND events.payload->'verification_refs'=
              NEW.descriptor_json::jsonb->'rejection'->'refs'
          AND entries.kind='hypothesis'
          AND entries.authority='model_proposal'
          AND entries.status='rejected'
          AND entries.disposition_by='code'
          AND NEW.target_uri='task:ledger/'||NEW.ledger_id||'/entry/'||entries.id
          AND NEW.target_version::BIGINT=entries.created_version
          AND entries.updated_version=NEW.expected_ledger_version+1
          AND entries.content_sha256=NEW.target_sha256
    ) THEN
        RAISE EXCEPTION 'belief revision lacks exact reconciliation and rejection authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_belief_revisions_require_exact_authority
AFTER INSERT ON cognition_belief_revisions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_belief_revision();

CREATE OR REPLACE FUNCTION require_cognition_hypothesis_rejection_materialization()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.event_kind='entry_rejected' AND NEW.actor='code' AND EXISTS (
        SELECT 1
        FROM task_entries entries
        JOIN cognition_obligations obligations ON obligations.node_id=entries.scope_node_id
        JOIN cognition_episodes episodes
          ON episodes.episode_id=obligations.episode_id AND episodes.ledger_id=NEW.ledger_id
        WHERE entries.ledger_id=NEW.ledger_id
          AND entries.id=NEW.payload->>'entry_id'
          AND entries.kind='hypothesis'
          AND entries.authority='model_proposal'
    ) AND NOT EXISTS (
        SELECT 1 FROM cognition_belief_revisions revisions
        WHERE revisions.ledger_id=NEW.ledger_id
          AND revisions.descriptor_json::jsonb->'rejection'->>'command_id'=NEW.command_id
          AND revisions.descriptor_json::jsonb->'rejection'->>'entry_id'=NEW.payload->>'entry_id'
          AND revisions.expected_ledger_version+1=NEW.ledger_version
    ) THEN
        RAISE EXCEPTION 'cognition model hypothesis rejection has no exact belief-revision materialization';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER task_events_require_cognition_belief_revision
AFTER INSERT ON task_events DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_hypothesis_rejection_materialization();

CREATE TRIGGER cognition_belief_revisions_immutable
BEFORE UPDATE OR DELETE ON cognition_belief_revisions
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_belief_revisions_no_truncate
BEFORE TRUNCATE ON cognition_belief_revisions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
