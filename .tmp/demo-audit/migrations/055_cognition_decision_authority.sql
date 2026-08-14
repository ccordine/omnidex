DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_reconciliations) THEN
        RAISE EXCEPTION 'migration 055 requires cognition_reconciliations empty; accepted decision authority cannot be backfilled';
    END IF;
END;
$$;

ALTER TABLE cognition_obligation_materializations
    ADD COLUMN ledger_id TEXT NOT NULL,
    ADD COLUMN candidate_entry_id TEXT NOT NULL,
    ADD CONSTRAINT cognition_obligation_materialization_candidate_fk
        FOREIGN KEY (ledger_id,candidate_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT;

CREATE TABLE cognition_decision_acceptances (
    acceptance_id TEXT PRIMARY KEY CHECK (
        acceptance_id~'^cognition_decision_acceptance_[0-9a-f]{64}$'
    ),
    acceptance_sha256 TEXT NOT NULL CHECK (acceptance_sha256~'^[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL,
    reconciliation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    ledger_id TEXT NOT NULL,
    policy_call_id TEXT NOT NULL UNIQUE REFERENCES cognition_policy_calls(call_id) ON DELETE RESTRICT,
    snapshot_sha256 TEXT NOT NULL UNIQUE
        REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    decision_sha256 TEXT NOT NULL CHECK (decision_sha256~'^[0-9a-f]{64}$'),
    candidate_entry_id TEXT NOT NULL,
    accepted_entry_id TEXT NOT NULL,
    action_schema_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_id)),
    action_schema_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(action_schema_version)),
    action_schema_sha256 TEXT NOT NULL CHECK (action_schema_sha256~'^[0-9a-f]{64}$'),
    acceptance_command_id TEXT NOT NULL CHECK (acceptance_command_id~'^command_[0-9a-f]{64}$'),
    acceptance_command_sha256 TEXT NOT NULL CHECK (acceptance_command_sha256~'^[0-9a-f]{64}$'),
    identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(identity_json::jsonb)='object' AND octet_length(identity_json)<=262144
    ),
    identity_json_sha256 TEXT NOT NULL CHECK (
        identity_json_sha256=encode(digest(identity_json,'sha256'),'hex')
    ),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND octet_length(descriptor_json)<=262144
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id)
        REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,candidate_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,accepted_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,acceptance_command_id)
        REFERENCES task_events(ledger_id,command_id) ON DELETE RESTRICT,
    CHECK (acceptance_id='cognition_decision_acceptance_'||acceptance_sha256),
    CHECK (acceptance_sha256=identity_json_sha256),
    CHECK (identity_json::jsonb=descriptor_json::jsonb-'id'-'sha256'),
    CHECK (descriptor_json::jsonb->>'schema'='omnidex.cognition-decision-acceptance.v1'),
    CHECK (descriptor_json::jsonb->>'id'=acceptance_id),
    CHECK (descriptor_json::jsonb->>'sha256'=acceptance_sha256),
    CHECK (descriptor_json::jsonb->>'ledger_id'=ledger_id),
    CHECK (descriptor_json::jsonb->>'candidate_entry_id'=candidate_entry_id),
    CHECK (descriptor_json::jsonb->>'accepted_entry_id'=accepted_entry_id),
    CHECK (descriptor_json::jsonb->>'policy_call_id'=policy_call_id),
    CHECK (descriptor_json::jsonb->>'snapshot_sha256'=snapshot_sha256),
    CHECK (descriptor_json::jsonb->>'decision_sha256'=decision_sha256),
    CHECK (descriptor_json::jsonb->'action_schema'=jsonb_build_object(
        'id',action_schema_id,'version',action_schema_version,'sha256',action_schema_sha256
    )),
    CHECK (descriptor_json::jsonb->>'acceptance_command_id'=acceptance_command_id),
    CHECK (descriptor_json::jsonb->>'acceptance_command_sha256'=acceptance_command_sha256)
);

CREATE OR REPLACE FUNCTION require_cognition_decision_acceptance()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM cognition_reconciliations reconciliations
          JOIN cognition_policy_calls calls ON calls.call_id=NEW.policy_call_id
          JOIN task_entries candidate
            ON candidate.ledger_id=NEW.ledger_id AND candidate.id=NEW.candidate_entry_id
          JOIN task_entries accepted
            ON accepted.ledger_id=NEW.ledger_id AND accepted.id=NEW.accepted_entry_id
          JOIN task_events event
            ON event.ledger_id=NEW.ledger_id AND event.command_id=NEW.acceptance_command_id
         WHERE reconciliations.reconciliation_id=NEW.reconciliation_id
           AND reconciliations.episode_id=NEW.episode_id
           AND reconciliations.policy_call_id=NEW.policy_call_id
           AND reconciliations.snapshot_sha256=NEW.snapshot_sha256
           AND reconciliations.decision_sha256=NEW.decision_sha256
           AND reconciliations.action_schema_id=NEW.action_schema_id
           AND reconciliations.action_schema_version=NEW.action_schema_version
           AND reconciliations.action_schema_sha256=NEW.action_schema_sha256
           AND calls.status='accepted' AND calls.result_sha256 IS NOT NULL
           AND candidate.kind='decision_candidate' AND candidate.authority='model_proposal'
           AND candidate.status='superseded'
           AND candidate.disposition_reason='cognition-policy-call-and-action-schema-v1'
           AND candidate.disposition_by='code'
           AND accepted.kind='accepted_decision' AND accepted.authority='accepted_model_decision'
           AND accepted.status='active' AND accepted.source_entry_id=NEW.candidate_entry_id
           AND accepted.acceptance_policy='cognition-policy-call-and-action-schema-v1'
           AND accepted.accepted_by='code'
           AND accepted.metadata=NEW.descriptor_json::jsonb-'id'-'sha256'-'ledger_id'-
               'candidate_entry_id'-'accepted_entry_id'-'acceptance_refs'-
               'acceptance_command_id'-'acceptance_command_sha256'
           AND event.command_kind='accept_decision' AND event.actor='code'
           AND event.command_sha256=NEW.acceptance_command_sha256
    ) OR NOT EXISTS (
        SELECT 1 FROM task_entry_refs refs
        JOIN cognition_policy_calls calls ON calls.call_id=NEW.policy_call_id
        WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.accepted_entry_id
          AND refs.uri='cognition:policy-call/'||NEW.policy_call_id
          AND refs.version='accepted' AND refs.content_sha256=calls.result_sha256
          AND refs.relation='verifies'
    ) OR NOT EXISTS (
        SELECT 1 FROM task_entry_refs refs
        WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.accepted_entry_id
          AND refs.uri='cognition:action-schema/'||NEW.action_schema_id
          AND refs.version=NEW.action_schema_version
          AND refs.content_sha256=NEW.action_schema_sha256 AND refs.relation='verifies'
    ) OR NEW.descriptor_json::jsonb->'acceptance_refs' IS DISTINCT FROM (
        SELECT jsonb_agg(jsonb_build_object(
            'uri',refs.uri,'version',refs.version,'content_sha256',refs.content_sha256,
            'relation',refs.relation
        ) ORDER BY refs.position)
        FROM task_entry_refs refs
        WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.accepted_entry_id
    ) THEN
        RAISE EXCEPTION 'selected cognition decision lacks exact code acceptance authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_decision_acceptances_exact_authority
AFTER INSERT ON cognition_decision_acceptances DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_decision_acceptance();

CREATE OR REPLACE FUNCTION require_cognition_selected_decision_reverse()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind='accepted_decision' AND
       NEW.metadata->>'schema'='omnidex.cognition-decision-acceptance.v1' AND NOT EXISTS (
        SELECT 1 FROM cognition_decision_acceptances authority
        WHERE authority.ledger_id=NEW.ledger_id AND authority.accepted_entry_id=NEW.id
          AND authority.candidate_entry_id=NEW.source_entry_id
    ) THEN
        RAISE EXCEPTION 'selected cognition accepted entry lacks normalized authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER task_entries_require_cognition_selected_decision
AFTER INSERT ON task_entries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_selected_decision_reverse();

CREATE TRIGGER cognition_decision_acceptances_immutable
BEFORE UPDATE OR DELETE ON cognition_decision_acceptances
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_decision_acceptances_no_truncate
BEFORE TRUNCATE ON cognition_decision_acceptances
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
