CREATE TABLE cognition_proposal_dispositions (
    disposition_id TEXT PRIMARY KEY CHECK (
        disposition_id~'^cognition_proposal_disposition_[0-9a-f]{64}$'
    ),
    disposition_sha256 TEXT NOT NULL CHECK (disposition_sha256~'^[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    ledger_id TEXT NOT NULL,
    reconciliation_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_reconciliations(reconciliation_id) ON DELETE RESTRICT,
    action_id TEXT NOT NULL UNIQUE REFERENCES cognition_actions(action_id) ON DELETE RESTRICT,
    candidate_entry_id TEXT NOT NULL,
    proposal_kind TEXT NOT NULL CHECK (proposal_kind='obligation'),
    proposal_index INTEGER NOT NULL CHECK (proposal_index>=0 AND proposal_index<32),
    source_descriptor_id TEXT NOT NULL UNIQUE
        REFERENCES cognition_obligation_materializations(materialization_id) ON DELETE RESTRICT,
    source_descriptor_sha256 TEXT NOT NULL CHECK (source_descriptor_sha256~'^[0-9a-f]{64}$'),
    outcome TEXT NOT NULL CHECK (outcome IN (
        'accepted_materialization','rejected_action_failure','rejected_terminal_transition'
    )),
    proof_uri TEXT NOT NULL CHECK (task_ledger_text_is_exact(proof_uri)),
    proof_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(proof_version)),
    proof_sha256 TEXT NOT NULL CHECK (proof_sha256~'^[0-9a-f]{64}$'),
    result_entry_id TEXT,
    command_id TEXT NOT NULL CHECK (command_id~'^command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256~'^[0-9a-f]{64}$'),
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
    FOREIGN KEY (ledger_id,candidate_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,result_entry_id)
        REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,command_id)
        REFERENCES task_events(ledger_id,command_id) ON DELETE RESTRICT,
    CHECK (disposition_id='cognition_proposal_disposition_'||disposition_sha256),
    CHECK (disposition_sha256=identity_json_sha256),
    CHECK (identity_json::jsonb=descriptor_json::jsonb-'id'-'sha256'),
    CHECK ((outcome='accepted_materialization')=(result_entry_id IS NOT NULL)),
    CHECK (descriptor_json::jsonb->>'schema'='omnidex.cognition-proposal-disposition.v1'),
    CHECK (descriptor_json::jsonb->>'id'=disposition_id),
    CHECK (descriptor_json::jsonb->>'sha256'=disposition_sha256),
    CHECK (descriptor_json::jsonb->>'episode_id'=episode_id),
    CHECK (descriptor_json::jsonb->>'ledger_id'=ledger_id),
    CHECK (descriptor_json::jsonb->>'reconciliation_id'=reconciliation_id),
    CHECK (descriptor_json::jsonb->>'action_id'=action_id),
    CHECK (descriptor_json::jsonb->>'candidate_entry_id'=candidate_entry_id),
    CHECK (descriptor_json::jsonb->>'proposal_kind'=proposal_kind),
    CHECK ((descriptor_json::jsonb->>'proposal_index')::integer=proposal_index),
    CHECK (descriptor_json::jsonb->>'source_descriptor_id'=source_descriptor_id),
    CHECK (descriptor_json::jsonb->>'source_descriptor_sha256'=source_descriptor_sha256),
    CHECK (descriptor_json::jsonb->>'outcome'=outcome),
    CHECK (descriptor_json::jsonb->'proof_ref'=jsonb_build_object(
        'uri',proof_uri,'version',proof_version,'content_sha256',proof_sha256,'relation','verifies'
    )),
    CHECK ((descriptor_json::jsonb->>'result_entry_id') IS NOT DISTINCT FROM result_entry_id),
    CHECK (descriptor_json::jsonb->>'command_id'=command_id),
    CHECK (descriptor_json::jsonb->>'command_sha256'=command_sha256)
);

CREATE OR REPLACE FUNCTION require_exact_cognition_proposal_disposition()
RETURNS TRIGGER AS $$
DECLARE event_payload JSONB;
BEGIN
    SELECT events.payload INTO event_payload
      FROM cognition_obligation_materializations materializations
      JOIN cognition_reconciliations reconciliations
        ON reconciliations.reconciliation_id=materializations.reconciliation_id
      JOIN cognition_actions actions ON actions.reconciliation_id=reconciliations.reconciliation_id
      JOIN task_entries candidate
        ON candidate.ledger_id=NEW.ledger_id AND candidate.id=NEW.candidate_entry_id
      JOIN task_events events
        ON events.ledger_id=NEW.ledger_id AND events.command_id=NEW.command_id
     WHERE materializations.materialization_id=NEW.source_descriptor_id
       AND materializations.materialization_sha256=NEW.source_descriptor_sha256
       AND materializations.episode_id=NEW.episode_id
       AND materializations.ledger_id=NEW.ledger_id
       AND materializations.candidate_entry_id=NEW.candidate_entry_id
       AND materializations.proposal_index=NEW.proposal_index
       AND reconciliations.reconciliation_id=NEW.reconciliation_id
       AND actions.action_id=NEW.action_id
       AND candidate.kind='decision_candidate' AND candidate.authority='model_proposal'
       AND candidate.metadata->>'source_kind'='model_obligation_candidate'
       AND candidate.metadata->>'candidate_kind'='obligation'
       AND (candidate.metadata->>'proposal_index')::integer=NEW.proposal_index
       AND events.command_sha256=NEW.command_sha256 AND events.actor='code';
    IF event_payload IS NULL THEN
        RAISE EXCEPTION 'cognition proposal disposition lacks exact source authority';
    END IF;
    IF NEW.outcome='accepted_materialization' THEN
        IF NOT EXISTS (
            SELECT 1 FROM cognition_actions actions
            JOIN cognition_transitions transitions
              ON transitions.episode_id=actions.episode_id AND transitions.action_id=actions.action_id
            JOIN cognition_obligation_materialization_applications applications
              ON applications.action_id=actions.action_id
            JOIN task_entries accepted
              ON accepted.ledger_id=NEW.ledger_id AND accepted.id=NEW.result_entry_id
            WHERE actions.action_id=NEW.action_id AND actions.status='succeeded'
              AND transitions.terminal=FALSE
              AND transitions.transition_sha256=NEW.proof_sha256
              AND NEW.proof_uri='cognition:transition/'||NEW.action_id
              AND NEW.proof_version=transitions.current_revision_sha256
              AND applications.materialization_id=NEW.source_descriptor_id
              AND accepted.kind='accepted_decision' AND accepted.status='active'
              AND accepted.authority='accepted_model_decision'
              AND accepted.source_entry_id=NEW.candidate_entry_id
              AND accepted.acceptance_policy='cognition-obligation-materialization-v1'
              AND accepted.accepted_by='code'
              AND event_payload->>'event_kind'='decision_accepted'
              AND event_payload->>'entry_id'=NEW.candidate_entry_id
              AND event_payload->>'replacement_id'=NEW.result_entry_id
        ) THEN
            RAISE EXCEPTION 'accepted cognition proposal lacks exact applied materialization';
        END IF;
    ELSIF NEW.outcome='rejected_action_failure' THEN
        IF NOT EXISTS (
            SELECT 1 FROM cognition_actions actions
            WHERE actions.action_id=NEW.action_id AND actions.status='failed'
              AND actions.failure_sha256=NEW.proof_sha256
              AND NEW.proof_uri='cognition:action-failure/'||NEW.action_id
              AND NEW.proof_version=actions.failure_json::jsonb->>'code'
              AND event_payload->>'event_kind'='entry_rejected'
              AND event_payload->>'entry_id'=NEW.candidate_entry_id
              AND event_payload->>'reason'='cognition-proposal-action-failure-v1'
              AND event_payload->'verification_refs'=jsonb_build_array(jsonb_build_object(
                  'uri',NEW.proof_uri,'version',NEW.proof_version,
                  'content_sha256',NEW.proof_sha256,'relation','verifies'
              ))
        ) OR EXISTS (
            SELECT 1 FROM cognition_obligation_materialization_applications
            WHERE action_id=NEW.action_id
        ) THEN
            RAISE EXCEPTION 'failed cognition proposal lacks exact rejection disposition';
        END IF;
    ELSE
        IF NOT EXISTS (
            SELECT 1 FROM cognition_actions actions
            JOIN cognition_transitions transitions
              ON transitions.episode_id=actions.episode_id AND transitions.action_id=actions.action_id
            WHERE actions.action_id=NEW.action_id AND actions.status='succeeded'
              AND transitions.terminal=TRUE AND transitions.transition_sha256=NEW.proof_sha256
              AND NEW.proof_uri='cognition:transition/'||NEW.action_id
              AND NEW.proof_version=transitions.current_revision_sha256
              AND event_payload->>'event_kind'='entry_rejected'
              AND event_payload->>'entry_id'=NEW.candidate_entry_id
              AND event_payload->>'reason'='cognition-proposal-terminal-transition-v1'
        ) OR EXISTS (
            SELECT 1 FROM cognition_obligation_materialization_applications
            WHERE action_id=NEW.action_id
        ) THEN
            RAISE EXCEPTION 'terminal cognition proposal lacks exact rejection disposition';
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_proposal_dispositions_exact
AFTER INSERT ON cognition_proposal_dispositions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_proposal_disposition();

CREATE OR REPLACE FUNCTION require_cognition_proposal_candidate_disposition()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status='active' AND NEW.status<>'active' AND
       NEW.metadata->>'source_kind'='model_obligation_candidate' AND NOT EXISTS (
        SELECT 1 FROM cognition_proposal_dispositions dispositions
        WHERE dispositions.ledger_id=NEW.ledger_id
          AND dispositions.candidate_entry_id=NEW.id
          AND ((NEW.status='superseded' AND dispositions.outcome='accepted_materialization') OR
               (NEW.status='rejected' AND dispositions.outcome IN (
                   'rejected_action_failure','rejected_terminal_transition'
               )))
    ) THEN
        RAISE EXCEPTION 'cognition proposal candidate lacks normalized disposition';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER task_entries_require_cognition_proposal_disposition
AFTER UPDATE ON task_entries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_proposal_candidate_disposition();

CREATE OR REPLACE FUNCTION require_cognition_action_materialization_application()
RETURNS TRIGGER AS $$
DECLARE is_terminal BOOLEAN;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_obligation_materializations materializations
        WHERE materializations.reconciliation_id=NEW.reconciliation_id
    ) THEN RETURN NULL; END IF;
    IF NEW.status='succeeded' THEN
        SELECT transitions.terminal INTO STRICT is_terminal
        FROM cognition_transitions transitions
        WHERE transitions.episode_id=NEW.episode_id AND transitions.action_id=NEW.action_id;
        IF (NOT is_terminal AND NOT EXISTS (
            SELECT 1 FROM cognition_obligation_materialization_applications applications
            JOIN cognition_proposal_dispositions dispositions
              ON dispositions.source_descriptor_id=applications.materialization_id
            WHERE applications.action_id=NEW.action_id
              AND dispositions.action_id=NEW.action_id
              AND dispositions.outcome='accepted_materialization'
        )) OR (is_terminal AND (
            EXISTS (SELECT 1 FROM cognition_obligation_materialization_applications WHERE action_id=NEW.action_id) OR
            NOT EXISTS (SELECT 1 FROM cognition_proposal_dispositions
                        WHERE action_id=NEW.action_id AND outcome='rejected_terminal_transition')
        )) THEN RAISE EXCEPTION 'resolved cognition action omitted exact proposal disposition'; END IF;
    ELSIF NEW.status='failed' AND (
        EXISTS (SELECT 1 FROM cognition_obligation_materialization_applications WHERE action_id=NEW.action_id) OR
        NOT EXISTS (SELECT 1 FROM cognition_proposal_dispositions
                    WHERE action_id=NEW.action_id AND outcome='rejected_action_failure')
    ) THEN RAISE EXCEPTION 'failed cognition action omitted exact proposal rejection';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_proposal_dispositions_immutable
BEFORE UPDATE OR DELETE ON cognition_proposal_dispositions
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_proposal_dispositions_no_truncate
BEFORE TRUNCATE ON cognition_proposal_dispositions
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
