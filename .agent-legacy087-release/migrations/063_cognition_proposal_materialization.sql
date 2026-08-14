DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_reconciliations) THEN
        RAISE EXCEPTION 'migration 063 cannot invent proposal materialization authority for existing reconciliations';
    END IF;
END;
$$;

CREATE TABLE cognition_proposal_materializations (
    materialization_id TEXT PRIMARY KEY CHECK (
        materialization_id~'^cognition_proposal_materialization_[0-9a-f]{64}$'
    ),
    materialization_sha256 TEXT NOT NULL CHECK (
        materialization_sha256~'^[0-9a-f]{64}$' AND
        materialization_id='cognition_proposal_materialization_'||materialization_sha256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    reconciliation_id TEXT NOT NULL,
    policy_call_id TEXT NOT NULL,
    call_ordinal BIGINT NOT NULL CHECK (call_ordinal>0),
    snapshot_sha256 TEXT NOT NULL CHECK (snapshot_sha256~'^[0-9a-f]{64}$'),
    decision_sha256 TEXT NOT NULL CHECK (decision_sha256~'^[0-9a-f]{64}$'),
    proposal_index INTEGER NOT NULL CHECK (proposal_index>=0 AND proposal_index<32),
    proposal_kind TEXT NOT NULL CHECK (proposal_kind IN (
        'observation','hypothesis','question','obligation','plan_revision'
    )),
    proposal_json JSONB NOT NULL CHECK (jsonb_typeof(proposal_json)='object'),
    source_kind TEXT NOT NULL CHECK (source_kind IN (
        'model_observation','model_hypothesis','model_question',
        'model_obligation_candidate','model_plan_revision_candidate'
    )),
    ledger_id TEXT NOT NULL,
    pre_proposal_ledger_version BIGINT NOT NULL CHECK (pre_proposal_ledger_version>0),
    pre_proposal_ledger_sha256 TEXT NOT NULL CHECK (
        pre_proposal_ledger_sha256~'^[0-9a-f]{64}$'
    ),
    pre_proposal_ledger_json TEXT NOT NULL CHECK (
        jsonb_typeof(pre_proposal_ledger_json::jsonb)='object' AND
        octet_length(pre_proposal_ledger_json)<=2097152 AND
        pre_proposal_ledger_json=cognition_canonical_jsonb(pre_proposal_ledger_json::jsonb)
    ),
    pre_proposal_ledger_json_sha256 TEXT NOT NULL CHECK (
        pre_proposal_ledger_json_sha256~'^[0-9a-f]{64}$' AND
        pre_proposal_ledger_json_sha256=encode(digest(pre_proposal_ledger_json,'sha256'),'hex')
    ),
    mapping_id TEXT NOT NULL UNIQUE CHECK (mapping_id~'^cognition_mapping_[0-9a-f]{64}$'),
    mapping_sha256 TEXT NOT NULL CHECK (
        mapping_sha256~'^[0-9a-f]{64}$' AND mapping_id='cognition_mapping_'||mapping_sha256
    ),
    command_id TEXT NOT NULL CHECK (command_id~'^command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256~'^[0-9a-f]{64}$'),
    entry_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(entry_id)),
    entry_uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(entry_uri)),
    output_ledger_version BIGINT NOT NULL CHECK (output_ledger_version>0),
    output_ledger_status TEXT NOT NULL CHECK (output_ledger_status='active'),
    identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(identity_json::jsonb)='object' AND octet_length(identity_json)<=2097152 AND
        identity_json=cognition_canonical_jsonb(identity_json::jsonb)
    ),
    identity_json_sha256 TEXT NOT NULL CHECK (
        identity_json_sha256~'^[0-9a-f]{64}$' AND
        identity_json_sha256=encode(digest(identity_json,'sha256'),'hex')
    ),
    payload_json TEXT NOT NULL CHECK (
        jsonb_typeof(payload_json::jsonb)='object' AND octet_length(payload_json)<=2097152 AND
        payload_json=cognition_canonical_jsonb(payload_json::jsonb)
    ),
    payload_json_sha256 TEXT NOT NULL CHECK (
        payload_json_sha256~'^[0-9a-f]{64}$' AND
        payload_json_sha256=encode(digest(payload_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (reconciliation_id)
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (policy_call_id)
        REFERENCES cognition_policy_calls(call_id) ON DELETE RESTRICT,
    FOREIGN KEY (snapshot_sha256)
        REFERENCES cognition_runtime_snapshots(snapshot_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,command_id)
        REFERENCES task_events(ledger_id,command_id) ON DELETE RESTRICT,
    UNIQUE (reconciliation_id,proposal_index),
    UNIQUE (ledger_id,entry_id),
    UNIQUE (ledger_id,command_id),
    CHECK (materialization_sha256=identity_json_sha256),
    CHECK (identity_json::jsonb=payload_json::jsonb-'id'-'sha256'),
    CHECK (payload_json::jsonb->>'schema'='omnidex.cognition-proposal-materialization.v1'),
    CHECK (payload_json::jsonb->>'id'=materialization_id),
    CHECK (payload_json::jsonb->>'sha256'=materialization_sha256),
    CHECK (payload_json::jsonb->>'episode_id'=episode_id),
    CHECK (payload_json::jsonb->>'reconciliation_id'=reconciliation_id),
    CHECK (payload_json::jsonb->>'policy_call_id'=policy_call_id),
    CHECK ((payload_json::jsonb->>'call_ordinal')::BIGINT=call_ordinal),
    CHECK (payload_json::jsonb->>'snapshot_sha256'=snapshot_sha256),
    CHECK (payload_json::jsonb->>'decision_sha256'=decision_sha256),
    CHECK ((payload_json::jsonb->>'proposal_index')::INTEGER=proposal_index),
    CHECK (payload_json::jsonb->'proposal'=proposal_json),
    CHECK (proposal_json->>'kind'=proposal_kind),
    CHECK (payload_json::jsonb->>'source_kind'=source_kind),
    CHECK ((payload_json::jsonb->>'pre_proposal_ledger_version')::BIGINT=pre_proposal_ledger_version),
    CHECK (payload_json::jsonb->>'pre_proposal_ledger_sha256'=pre_proposal_ledger_sha256),
    CHECK (payload_json::jsonb->>'pre_proposal_ledger_json_sha256'=pre_proposal_ledger_json_sha256),
    CHECK (payload_json::jsonb->'pre_proposal_ledger'=pre_proposal_ledger_json::jsonb),
    CHECK ((pre_proposal_ledger_json::jsonb->>'version')::BIGINT=pre_proposal_ledger_version),
    CHECK (pre_proposal_ledger_json::jsonb->>'id'=ledger_id),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,ID}'=mapping_id),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,SHA256}'=mapping_sha256),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,SourceKind}'=source_kind),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,LedgerID}'=ledger_id),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,LedgerSHA256}'=pre_proposal_ledger_sha256),
    CHECK ((payload_json::jsonb#>>'{replay_descriptor,ExpectedVersion}')::BIGINT=
           pre_proposal_ledger_version+proposal_index),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,Actor}'='model_proposal'),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,EntryID}'=entry_id),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,CommandID}'=command_id),
    CHECK (payload_json::jsonb#>>'{replay_descriptor,CommandSHA256}'=command_sha256),
    CHECK (payload_json::jsonb#>>'{command,command_id}'=command_id),
    CHECK ((payload_json::jsonb#>>'{command,expected_version}')::BIGINT=
           pre_proposal_ledger_version+proposal_index),
    CHECK (payload_json::jsonb#>>'{command,actor}'='model_proposal'),
    CHECK (payload_json::jsonb#>>'{command,id}'=entry_id),
    CHECK (payload_json::jsonb->>'entry_uri'=entry_uri),
    CHECK (entry_uri='task:ledger/'||ledger_id||'/entry/'||entry_id),
    CHECK ((payload_json::jsonb->>'output_ledger_version')::BIGINT=output_ledger_version),
    CHECK (output_ledger_version=pre_proposal_ledger_version+proposal_index+1),
    CHECK (payload_json::jsonb->>'output_ledger_status'=output_ledger_status),
    CHECK ((proposal_kind='observation' AND source_kind='model_observation') OR
           (proposal_kind='hypothesis' AND source_kind='model_hypothesis') OR
           (proposal_kind='question' AND source_kind='model_question') OR
           (proposal_kind='obligation' AND source_kind='model_obligation_candidate') OR
           (proposal_kind='plan_revision' AND source_kind='model_plan_revision_candidate'))
);

CREATE INDEX cognition_proposal_materializations_episode_order
    ON cognition_proposal_materializations(episode_id,call_ordinal,proposal_index);

CREATE TRIGGER cognition_proposal_materializations_immutable
BEFORE UPDATE OR DELETE ON cognition_proposal_materializations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_proposal_materializations_no_truncate
BEFORE TRUNCATE ON cognition_proposal_materializations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
