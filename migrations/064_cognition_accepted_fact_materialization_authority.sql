CREATE OR REPLACE FUNCTION require_exact_cognition_accepted_fact_materialization()
RETURNS TRIGGER AS $$
DECLARE exact_members JSONB;
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_transitions transitions
        JOIN cognition_episodes episodes ON episodes.episode_id=NEW.episode_id
        JOIN cognition_obligations obligations
          ON obligations.episode_id=NEW.episode_id AND obligations.node_id=NEW.scope_obligation_id
        WHERE transitions.transition_id=NEW.transition_id
          AND transitions.episode_id=NEW.episode_id
          AND transitions.job_id=NEW.job_id
          AND transitions.generation=NEW.generation
          AND transitions.step_id=NEW.step_id
          AND transitions.actor_attempt=NEW.actor_attempt
          AND transitions.actor_worker_id=NEW.actor_worker_id
          AND transitions.transition_sha256=NEW.transition_sha256
          AND transitions.revision=NEW.transition_revision
          AND transitions.action_id IS NOT DISTINCT FROM NEW.action_id
          AND episodes.job_id=NEW.job_id
          AND episodes.generation=NEW.generation
          AND episodes.step_id=NEW.step_id
          AND episodes.ledger_id=NEW.ledger_id
          AND episodes.fact_authority_json::jsonb=NEW.payload_json::jsonb->'fact_authority'
          AND episodes.fact_authority_json::jsonb->>'sha256'=NEW.authority_sha256
          AND obligations.ledger_id=NEW.ledger_id
          AND (
              (NEW.action_id IS NULL AND NEW.call_ordinal=0) OR
              EXISTS (
                  SELECT 1 FROM cognition_actions actions
                  JOIN cognition_runtime_snapshots snapshots
                    ON snapshots.snapshot_sha256=actions.snapshot_sha256
                  WHERE actions.action_id=NEW.action_id
                    AND actions.episode_id=NEW.episode_id
                    AND actions.job_id=NEW.job_id
                    AND actions.generation=NEW.generation
                    AND actions.step_id=NEW.step_id
                    AND actions.origin_attempt=NEW.actor_attempt
                    AND actions.origin_worker_id=NEW.actor_worker_id
                    AND actions.obligation_node_id=NEW.scope_obligation_id
                    AND snapshots.call_ordinal=NEW.call_ordinal
              )
          )
    ) THEN
        RAISE EXCEPTION 'accepted-fact materialization lacks exact transition and episode authority';
    END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'index',members.position,
        'fact',facts.descriptor_json::jsonb,
        'command',batch.payload_json::jsonb->'members'->members.position->'command',
        'entry_uri',members.entry_uri,
        'output_ledger_version',members.output_ledger_version,
        'output_ledger_status',members.output_ledger_status
    ) ORDER BY members.position),'[]'::jsonb)
      INTO exact_members
      FROM cognition_accepted_fact_materializations batch
      JOIN cognition_accepted_fact_materialization_members members
        ON members.materialization_id=batch.materialization_id
      JOIN cognition_accepted_facts facts ON facts.fact_id=members.fact_id
      WHERE batch.materialization_id=NEW.materialization_id;
    IF NEW.payload_json::jsonb->'members' IS DISTINCT FROM exact_members OR
       NEW.member_count<>(
           SELECT COUNT(*) FROM cognition_accepted_fact_materialization_members members
           WHERE members.materialization_id=NEW.materialization_id
       ) THEN
        RAISE EXCEPTION 'accepted-fact materialization lacks exact ordered member totality';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_materializations_require_exact_authority
AFTER INSERT ON cognition_accepted_fact_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_accepted_fact_materialization();

CREATE OR REPLACE FUNCTION require_exact_cognition_accepted_fact_materialization_member()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM cognition_accepted_fact_materializations batch
        JOIN cognition_accepted_facts facts ON facts.fact_id=NEW.fact_id
        JOIN task_events events
          ON events.ledger_id=facts.ledger_id AND events.command_id=NEW.command_id
        JOIN task_entries entries
          ON entries.ledger_id=facts.ledger_id AND entries.id=NEW.entry_id
        WHERE batch.materialization_id=NEW.materialization_id
          AND NEW.position<batch.member_count
          AND facts.fact_sha256=NEW.fact_sha256
          AND facts.episode_id=batch.episode_id
          AND facts.ledger_id=batch.ledger_id
          AND facts.transition_id=batch.transition_id
          AND facts.transition_sha256=batch.transition_sha256
          AND facts.scope_obligation_id=batch.scope_obligation_id
          AND facts.authority_sha256=batch.authority_sha256
          AND facts.command_id=NEW.command_id
          AND facts.command_sha256=NEW.command_sha256
          AND facts.entry_id=NEW.entry_id
          AND NEW.entry_uri='task:ledger/'||batch.ledger_id||'/entry/'||NEW.entry_id
          AND NEW.output_ledger_version=batch.pre_fact_ledger_version+NEW.position+1
          AND events.ledger_version=NEW.output_ledger_version
          AND events.command_sha256=NEW.command_sha256
          AND events.command_kind='add_entry'
          AND events.event_kind='entry_added'
          AND events.actor='code'
          AND entries.kind='fact'
          AND entries.status='active'
          AND entries.authority='code'
          AND entries.created_by='code'
          AND entries.created_version=NEW.output_ledger_version
          AND entries.updated_version=NEW.output_ledger_version
          AND batch.payload_json::jsonb->'members'->NEW.position->'fact'=facts.descriptor_json::jsonb
          AND batch.payload_json::jsonb->'members'->NEW.position->>'entry_uri'=NEW.entry_uri
          AND (batch.payload_json::jsonb->'members'->NEW.position->>'output_ledger_version')::BIGINT=
              NEW.output_ledger_version
          AND batch.payload_json::jsonb->'members'->NEW.position->>'output_ledger_status'=
              NEW.output_ledger_status
          AND batch.payload_json::jsonb->'members'->NEW.position#>>'{command,command_id}'=NEW.command_id
          AND (batch.payload_json::jsonb->'members'->NEW.position#>>'{command,expected_version}')::BIGINT=
              batch.pre_fact_ledger_version+NEW.position
          AND batch.payload_json::jsonb->'members'->NEW.position#>>'{command,actor}'='code'
          AND batch.payload_json::jsonb->'members'->NEW.position#>>'{command,id}'=NEW.entry_id
          AND batch.payload_json::jsonb->'members'->NEW.position#>>'{command,kind}'='fact'
          AND entries.content=batch.payload_json::jsonb->'members'->NEW.position#>>'{command,content}'
          AND entries.metadata=batch.payload_json::jsonb->'members'->NEW.position#>'{command,metadata}'
          AND events.payload->'entry'->>'id'=NEW.entry_id
          AND events.payload->'entry'->>'content'=entries.content
          AND events.payload->'entry'->'metadata'=entries.metadata
          AND events.payload->'entry'->'refs'=COALESCE(
              NULLIF(batch.payload_json::jsonb->'members'->NEW.position#>'{command,refs}','null'::jsonb),
              '[]'::jsonb
          )
          AND COALESCE(
              NULLIF(batch.payload_json::jsonb->'members'->NEW.position#>'{command,refs}','null'::jsonb),
              '[]'::jsonb
          )=(
              SELECT COALESCE(jsonb_agg(jsonb_build_object(
                  'uri',refs.uri,'version',refs.version,'content_sha256',refs.content_sha256,
                  'relation',refs.relation
              ) ORDER BY refs.position),'[]'::jsonb)
              FROM task_entry_refs refs
              WHERE refs.ledger_id=facts.ledger_id AND refs.entry_id=NEW.entry_id
          )
    ) THEN
        RAISE EXCEPTION 'accepted-fact materialization member lacks exact fact and ledger authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_materialization_members_require_exact_authority
AFTER INSERT ON cognition_accepted_fact_materialization_members DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_accepted_fact_materialization_member();

CREATE OR REPLACE FUNCTION require_cognition_accepted_fact_materialization_reverse()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_accepted_fact_materialization_members members
        WHERE members.fact_id=NEW.fact_id AND members.fact_sha256=NEW.fact_sha256
    ) THEN
        RAISE EXCEPTION 'accepted cognition fact lacks portable materialization authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_facts_require_materialization
AFTER INSERT ON cognition_accepted_facts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_accepted_fact_materialization_reverse();

CREATE OR REPLACE FUNCTION require_cognition_transition_fact_materialization()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_accepted_fact_materializations batch
        WHERE batch.transition_id=NEW.transition_id
          AND batch.transition_sha256=NEW.transition_sha256
          AND batch.episode_id=NEW.episode_id
          AND batch.transition_revision=NEW.revision
          AND batch.action_id IS NOT DISTINCT FROM NEW.action_id
    ) THEN
        RAISE EXCEPTION 'cognition transition lacks one zero-capable accepted-fact materialization';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_transitions_require_fact_materialization
AFTER INSERT ON cognition_transitions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_transition_fact_materialization();

CREATE OR REPLACE FUNCTION reject_cognition_accepted_fact_materialization_batch_omission()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_transitions transitions
        WHERE transitions.transition_id=OLD.transition_id
    ) AND NOT EXISTS (
        SELECT 1 FROM cognition_accepted_fact_materializations batch
        WHERE batch.transition_id=OLD.transition_id
    ) THEN
        RAISE EXCEPTION 'cognition transition lost its accepted-fact materialization batch';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_materializations_reject_omission
AFTER DELETE ON cognition_accepted_fact_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_cognition_accepted_fact_materialization_batch_omission();

CREATE OR REPLACE FUNCTION reject_cognition_accepted_fact_materialization_member_omission()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_accepted_facts facts WHERE facts.fact_id=OLD.fact_id
    ) AND NOT EXISTS (
        SELECT 1 FROM cognition_accepted_fact_materialization_members members
        WHERE members.fact_id=OLD.fact_id
    ) THEN
        RAISE EXCEPTION 'accepted cognition fact lost its materialization membership';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_materialization_members_reject_omission
AFTER DELETE ON cognition_accepted_fact_materialization_members DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_cognition_accepted_fact_materialization_member_omission();
