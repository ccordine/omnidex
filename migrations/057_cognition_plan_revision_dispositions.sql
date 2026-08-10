CREATE OR REPLACE FUNCTION require_exact_cognition_proposal_disposition()
RETURNS TRIGGER AS $$
DECLARE
    event_payload JSONB;
    expected_source_kind TEXT;
    expected_candidate_kind TEXT;
    expected_policy TEXT;
    application_exists BOOLEAN;
BEGIN
    expected_source_kind:=CASE NEW.proposal_kind
        WHEN 'obligation' THEN 'model_obligation_candidate'
        WHEN 'plan_revision' THEN 'model_plan_revision_candidate'
        ELSE NULL END;
    expected_candidate_kind:=NEW.proposal_kind;
    expected_policy:=CASE NEW.proposal_kind
        WHEN 'obligation' THEN 'cognition-obligation-materialization-v1'
        WHEN 'plan_revision' THEN 'cognition-plan-revision-materialization-v1'
        ELSE NULL END;
    SELECT events.payload INTO event_payload
    FROM cognition_graph_materialization_sources sources
    JOIN cognition_reconciliations reconciliations
      ON reconciliations.reconciliation_id=sources.reconciliation_id
    JOIN cognition_actions actions ON actions.reconciliation_id=reconciliations.reconciliation_id
    JOIN task_entries candidate
      ON candidate.ledger_id=NEW.ledger_id AND candidate.id=NEW.candidate_entry_id
    JOIN task_events events
      ON events.ledger_id=NEW.ledger_id AND events.command_id=NEW.command_id
    WHERE sources.descriptor_id=NEW.source_descriptor_id
      AND sources.descriptor_sha256=NEW.source_descriptor_sha256
      AND sources.episode_id=NEW.episode_id AND sources.ledger_id=NEW.ledger_id
      AND sources.candidate_entry_id=NEW.candidate_entry_id
      AND sources.proposal_kind=NEW.proposal_kind
      AND sources.proposal_index=NEW.proposal_index
      AND reconciliations.reconciliation_id=NEW.reconciliation_id
      AND actions.action_id=NEW.action_id
      AND candidate.kind='decision_candidate' AND candidate.authority='model_proposal'
      AND candidate.metadata->>'source_kind'=expected_source_kind
      AND candidate.metadata->>'candidate_kind'=expected_candidate_kind
      AND (candidate.metadata->>'proposal_index')::integer=NEW.proposal_index
      AND events.command_sha256=NEW.command_sha256 AND events.actor='code';
    IF event_payload IS NULL THEN
        RAISE EXCEPTION 'cognition proposal disposition lacks exact typed source authority';
    END IF;
    application_exists:=CASE NEW.proposal_kind
        WHEN 'obligation' THEN EXISTS (
            SELECT 1 FROM cognition_obligation_materialization_applications applications
            WHERE applications.materialization_id=NEW.source_descriptor_id
              AND applications.action_id=NEW.action_id)
        WHEN 'plan_revision' THEN EXISTS (
            SELECT 1 FROM cognition_plan_revision_applications applications
            WHERE applications.plan_revision_id=NEW.source_descriptor_id
              AND applications.action_id=NEW.action_id)
        ELSE FALSE END;
    IF NEW.outcome='accepted_materialization' THEN
        IF NOT application_exists OR NOT EXISTS (
            SELECT 1 FROM cognition_actions actions
            JOIN cognition_transitions transitions
              ON transitions.episode_id=actions.episode_id AND transitions.action_id=actions.action_id
            JOIN task_entries accepted
              ON accepted.ledger_id=NEW.ledger_id AND accepted.id=NEW.result_entry_id
            WHERE actions.action_id=NEW.action_id AND actions.status='succeeded'
              AND transitions.terminal=FALSE AND transitions.transition_sha256=NEW.proof_sha256
              AND NEW.proof_uri='cognition:transition/'||NEW.action_id
              AND NEW.proof_version=transitions.current_revision_sha256
              AND accepted.kind='accepted_decision' AND accepted.status='active'
              AND accepted.authority='accepted_model_decision'
              AND accepted.source_entry_id=NEW.candidate_entry_id
              AND accepted.acceptance_policy=expected_policy AND accepted.accepted_by='code'
              AND event_payload->>'event_kind'='decision_accepted'
              AND event_payload->>'entry_id'=NEW.candidate_entry_id
              AND event_payload->>'replacement_id'=NEW.result_entry_id
        ) THEN
            RAISE EXCEPTION 'accepted cognition proposal lacks exact typed materialization';
        END IF;
    ELSIF NEW.outcome='rejected_action_failure' THEN
        IF application_exists OR NOT EXISTS (
            SELECT 1 FROM cognition_actions actions
            WHERE actions.action_id=NEW.action_id AND actions.status='failed'
              AND actions.failure_sha256=NEW.proof_sha256
              AND NEW.proof_uri='cognition:action-failure/'||NEW.action_id
              AND NEW.proof_version=actions.failure_json::jsonb->>'code'
              AND event_payload->>'event_kind'='entry_rejected'
              AND event_payload->>'entry_id'=NEW.candidate_entry_id
              AND event_payload->>'reason'='cognition-proposal-action-failure-v1'
        ) THEN
            RAISE EXCEPTION 'failed cognition proposal lacks exact rejection disposition';
        END IF;
    ELSE
        IF application_exists OR NOT EXISTS (
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
        ) THEN
            RAISE EXCEPTION 'terminal cognition proposal lacks exact rejection disposition';
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_proposal_candidate_disposition()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status='active' AND NEW.status<>'active' AND
       NEW.metadata->>'source_kind' IN (
           'model_obligation_candidate','model_plan_revision_candidate'
       ) AND NOT EXISTS (
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

CREATE OR REPLACE FUNCTION require_cognition_action_materialization_application()
RETURNS TRIGGER AS $$
DECLARE
    source_kind TEXT;
    is_terminal BOOLEAN;
    application_count INTEGER;
BEGIN
    SELECT sources.proposal_kind INTO source_kind
    FROM cognition_graph_materialization_sources sources
    WHERE sources.reconciliation_id=NEW.reconciliation_id;
    IF source_kind IS NULL THEN RETURN NULL; END IF;
    application_count:=CASE source_kind
        WHEN 'obligation' THEN (SELECT COUNT(*) FROM cognition_obligation_materialization_applications
                                WHERE action_id=NEW.action_id)
        WHEN 'plan_revision' THEN (SELECT COUNT(*) FROM cognition_plan_revision_applications
                                   WHERE action_id=NEW.action_id)
        ELSE -1 END;
    IF NEW.status='succeeded' THEN
        SELECT transitions.terminal INTO STRICT is_terminal
        FROM cognition_transitions transitions
        WHERE transitions.episode_id=NEW.episode_id AND transitions.action_id=NEW.action_id;
        IF (NOT is_terminal AND (application_count<>1 OR NOT EXISTS (
            SELECT 1 FROM cognition_proposal_dispositions dispositions
            WHERE dispositions.action_id=NEW.action_id
              AND dispositions.outcome='accepted_materialization'
        ))) OR (is_terminal AND (application_count<>0 OR NOT EXISTS (
            SELECT 1 FROM cognition_proposal_dispositions dispositions
            WHERE dispositions.action_id=NEW.action_id
              AND dispositions.outcome='rejected_terminal_transition'
        ))) THEN
            RAISE EXCEPTION 'resolved cognition action omitted exact graph proposal disposition';
        END IF;
    ELSIF NEW.status='failed' AND (application_count<>0 OR NOT EXISTS (
        SELECT 1 FROM cognition_proposal_dispositions dispositions
        WHERE dispositions.action_id=NEW.action_id
          AND dispositions.outcome='rejected_action_failure'
    )) THEN
        RAISE EXCEPTION 'failed cognition action omitted exact graph proposal rejection';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
