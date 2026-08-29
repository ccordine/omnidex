CREATE OR REPLACE FUNCTION require_exact_cognition_proposal_materialization()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_reconciliations reconciliations
        JOIN cognition_policy_calls calls ON calls.call_id=NEW.policy_call_id
        JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=NEW.snapshot_sha256
        JOIN cognition_episodes episodes ON episodes.episode_id=NEW.episode_id
        JOIN task_events events
          ON events.ledger_id=NEW.ledger_id AND events.command_id=NEW.command_id
        JOIN task_entries entries
          ON entries.ledger_id=NEW.ledger_id AND entries.id=NEW.entry_id
        WHERE reconciliations.reconciliation_id=NEW.reconciliation_id
          AND reconciliations.episode_id=NEW.episode_id
          AND reconciliations.job_id=NEW.job_id
          AND reconciliations.generation=NEW.generation
          AND reconciliations.step_id=NEW.step_id
          AND reconciliations.actor_attempt=NEW.actor_attempt
          AND reconciliations.actor_worker_id=NEW.actor_worker_id
          AND reconciliations.policy_call_id=NEW.policy_call_id
          AND reconciliations.snapshot_sha256=NEW.snapshot_sha256
          AND reconciliations.decision_sha256=NEW.decision_sha256
          AND calls.episode_id=NEW.episode_id
          AND calls.job_id=NEW.job_id
          AND calls.generation=NEW.generation
          AND calls.step_id=NEW.step_id
          AND calls.step_attempt=NEW.actor_attempt
          AND calls.worker_id=NEW.actor_worker_id
          AND calls.status='accepted'
          AND calls.snapshot_sha256=NEW.snapshot_sha256
          AND calls.result_json::jsonb->>'decision_sha256'=NEW.decision_sha256
          AND snapshots.episode_id=NEW.episode_id
          AND snapshots.call_ordinal=NEW.call_ordinal
          AND snapshots.job_id=NEW.job_id
          AND snapshots.generation=NEW.generation
          AND snapshots.step_id=NEW.step_id
          AND snapshots.actor_attempt=NEW.actor_attempt
          AND snapshots.actor_worker_id=NEW.actor_worker_id
          AND episodes.ledger_id=NEW.ledger_id
          AND events.job_id=NEW.job_id
          AND events.job_generation=NEW.generation
          AND events.ledger_version=NEW.output_ledger_version
          AND events.command_sha256=NEW.command_sha256
          AND events.command_kind='add_entry'
          AND events.event_kind='entry_added'
          AND events.actor='model_proposal'
          AND entries.job_id=NEW.job_id
          AND entries.scope_node_id=NEW.payload_json::jsonb#>>'{command,scope_node_id}'
          AND entries.kind=NEW.payload_json::jsonb#>>'{command,kind}'
          AND entries.status='active'
          AND entries.authority='model_proposal'
          AND entries.created_by='model_proposal'
          AND entries.content=NEW.payload_json::jsonb#>>'{command,content}'
          AND entries.created_version=NEW.output_ledger_version
          AND entries.updated_version=NEW.output_ledger_version
          AND entries.metadata=NEW.payload_json::jsonb#>'{command,metadata}'
          AND events.payload->'entry'->>'id'=NEW.entry_id
          AND events.payload->'entry'->>'scope_node_id'=
              NEW.payload_json::jsonb#>>'{command,scope_node_id}'
          AND events.payload->'entry'->>'kind'=NEW.payload_json::jsonb#>>'{command,kind}'
          AND events.payload->'entry'->>'content'=NEW.payload_json::jsonb#>>'{command,content}'
          AND events.payload->'entry'->'metadata'=NEW.payload_json::jsonb#>'{command,metadata}'
          AND events.payload->'entry'->'refs'=COALESCE(
              NULLIF(NEW.payload_json::jsonb#>'{command,refs}','null'::jsonb),'[]'::jsonb
          )
    ) OR COALESCE(
        NULLIF(NEW.payload_json::jsonb#>'{command,refs}','null'::jsonb),'[]'::jsonb
    ) IS DISTINCT FROM (
        SELECT COALESCE(jsonb_agg(jsonb_build_object(
            'uri',refs.uri,'version',refs.version,'content_sha256',refs.content_sha256,
            'relation',refs.relation
        ) ORDER BY refs.position),'[]'::jsonb)
        FROM task_entry_refs refs
        WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.entry_id
    ) THEN
        RAISE EXCEPTION 'cognition proposal materialization % lacks exact reconciliation and ledger authority',
            NEW.proposal_index;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_proposal_materializations_require_exact_authority
AFTER INSERT ON cognition_proposal_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_proposal_materialization();

CREATE OR REPLACE FUNCTION require_cognition_reconciliation_proposal_materializations()
RETURNS TRIGGER AS $$
DECLARE expected_count INTEGER; materialized_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO expected_count
    FROM jsonb_array_elements(COALESCE(
        NEW.command_json::jsonb#>'{decision,ledger_proposals}','[]'::jsonb
    )) WITH ORDINALITY proposals(proposal,ordinality)
    WHERE proposal->>'kind'<>'revision';
    SELECT COUNT(*) INTO materialized_count
    FROM cognition_proposal_materializations materializations
    WHERE materializations.reconciliation_id=NEW.reconciliation_id;
    IF materialized_count<>expected_count OR EXISTS (
        SELECT 1
        FROM jsonb_array_elements(COALESCE(
            NEW.command_json::jsonb#>'{decision,ledger_proposals}','[]'::jsonb
        )) WITH ORDINALITY proposals(proposal,ordinality)
        WHERE proposal->>'kind'<>'revision' AND NOT EXISTS (
            SELECT 1 FROM cognition_proposal_materializations materializations
            JOIN cognition_runtime_snapshots snapshots
              ON snapshots.snapshot_sha256=materializations.snapshot_sha256
            WHERE materializations.reconciliation_id=NEW.reconciliation_id
              AND materializations.episode_id=NEW.episode_id
              AND materializations.policy_call_id=NEW.policy_call_id
              AND materializations.snapshot_sha256=NEW.snapshot_sha256
              AND materializations.decision_sha256=NEW.decision_sha256
              AND materializations.call_ordinal=snapshots.call_ordinal
              AND materializations.proposal_index=ordinality-1
              AND materializations.proposal_json=proposal
        )
    ) OR EXISTS (
        SELECT 1 FROM cognition_proposal_materializations materializations
        WHERE materializations.reconciliation_id=NEW.reconciliation_id AND NOT EXISTS (
            SELECT 1
            FROM jsonb_array_elements(COALESCE(
                NEW.command_json::jsonb#>'{decision,ledger_proposals}','[]'::jsonb
            )) WITH ORDINALITY proposals(proposal,ordinality)
            WHERE proposal->>'kind'<>'revision'
              AND materializations.proposal_index=ordinality-1
              AND materializations.proposal_json=proposal
        )
    ) OR (expected_count>0 AND NOT EXISTS (
        SELECT 1 FROM cognition_proposal_materializations materializations
        WHERE materializations.reconciliation_id=NEW.reconciliation_id
          AND materializations.output_ledger_version=NEW.ledger_version
          AND materializations.proposal_index=(
              SELECT MAX(ordinality-1)
              FROM jsonb_array_elements(COALESCE(
                  NEW.command_json::jsonb#>'{decision,ledger_proposals}','[]'::jsonb
              )) WITH ORDINALITY proposals(proposal,ordinality)
              WHERE proposal->>'kind'<>'revision'
          )
    )) THEN
        RAISE EXCEPTION 'cognition reconciliation lacks exact proposal materialization totality';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_reconciliations_require_proposal_materializations
AFTER INSERT ON cognition_reconciliations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_reconciliation_proposal_materializations();

CREATE OR REPLACE FUNCTION reject_cognition_proposal_materialization_omission()
RETURNS TRIGGER AS $$
DECLARE expected_count INTEGER; materialized_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO expected_count
    FROM cognition_reconciliations reconciliations,
         jsonb_array_elements(COALESCE(
             reconciliations.command_json::jsonb#>'{decision,ledger_proposals}',
             '[]'::jsonb
         )) proposals(proposal)
    WHERE reconciliations.reconciliation_id=OLD.reconciliation_id
      AND proposal->>'kind'<>'revision';
    SELECT COUNT(*) INTO materialized_count
    FROM cognition_proposal_materializations materializations
    WHERE materializations.reconciliation_id=OLD.reconciliation_id;
    IF materialized_count<>expected_count THEN
        RAISE EXCEPTION 'cognition reconciliation lost required proposal materialization';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_proposal_materializations_reject_omission
AFTER DELETE ON cognition_proposal_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_cognition_proposal_materialization_omission();

CREATE OR REPLACE FUNCTION require_cognition_model_proposal_entry_materialization()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.authority='model_proposal' AND
       NEW.metadata->>'schema'='omnidex.cognition-state-entry-mapping.v1' AND
       NEW.metadata->>'source_kind' IN (
           'model_observation','model_hypothesis','model_question',
           'model_obligation_candidate','model_plan_revision_candidate'
       ) AND NOT EXISTS (
           SELECT 1 FROM cognition_proposal_materializations materializations
           WHERE materializations.ledger_id=NEW.ledger_id
             AND materializations.entry_id=NEW.id
             AND materializations.source_kind=NEW.metadata->>'source_kind'
       ) THEN
        RAISE EXCEPTION 'cognition model proposal entry lacks normalized materialization authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER task_entries_require_cognition_proposal_materialization
AFTER INSERT ON task_entries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_model_proposal_entry_materialization();
