ALTER TABLE cognition_plan_revisions
    ADD CONSTRAINT cognition_plan_revision_descriptor_projection CHECK (
        descriptor_json::jsonb->>'source_snapshot_sha256'=source_snapshot_sha256 AND
        descriptor_json::jsonb->>'source_decision_sha256'=source_decision_sha256 AND
        descriptor_json::jsonb->>'source_proposal_sha256'=source_proposal_sha256 AND
        (descriptor_json::jsonb->>'proposal_index')::integer=proposal_index AND
        descriptor_json::jsonb->>'episode_id'=episode_id
    );

CREATE OR REPLACE FUNCTION require_exact_cognition_graph_materialization_source()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.proposal_kind='obligation' AND NOT EXISTS (
        SELECT 1 FROM cognition_obligation_materializations materializations
        WHERE materializations.materialization_id=NEW.descriptor_id
          AND materializations.materialization_sha256=NEW.descriptor_sha256
          AND materializations.episode_id=NEW.episode_id
          AND materializations.reconciliation_id=NEW.reconciliation_id
          AND materializations.ledger_id=NEW.ledger_id
          AND materializations.candidate_entry_id=NEW.candidate_entry_id
          AND materializations.proposal_index=NEW.proposal_index
    )) OR (NEW.proposal_kind='plan_revision' AND NOT EXISTS (
        SELECT 1 FROM cognition_plan_revisions revisions
        WHERE revisions.plan_revision_id=NEW.descriptor_id
          AND revisions.plan_revision_sha256=NEW.descriptor_sha256
          AND revisions.episode_id=NEW.episode_id
          AND revisions.reconciliation_id=NEW.reconciliation_id
          AND revisions.ledger_id=NEW.ledger_id
          AND revisions.candidate_entry_id=NEW.candidate_entry_id
          AND revisions.proposal_index=NEW.proposal_index
    )) THEN
        RAISE EXCEPTION 'cognition graph materialization source lacks exact typed descriptor';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_graph_materialization_sources_exact
AFTER INSERT ON cognition_graph_materialization_sources DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_graph_materialization_source();

CREATE OR REPLACE FUNCTION require_cognition_materialization_source_reverse()
RETURNS TRIGGER AS $$
DECLARE
    source_id TEXT;
    source_sha TEXT;
    source_kind TEXT;
    source_reconciliation TEXT;
BEGIN
    IF TG_TABLE_NAME='cognition_obligation_materializations' THEN
        source_id:=NEW.materialization_id;
        source_sha:=NEW.materialization_sha256;
        source_kind:='obligation';
    ELSE
        source_id:=NEW.plan_revision_id;
        source_sha:=NEW.plan_revision_sha256;
        source_kind:='plan_revision';
    END IF;
    source_reconciliation:=NEW.reconciliation_id;
    IF NOT EXISTS (
        SELECT 1 FROM cognition_graph_materialization_sources sources
        WHERE sources.descriptor_id=source_id AND sources.descriptor_sha256=source_sha
          AND sources.proposal_kind=source_kind
          AND sources.reconciliation_id=source_reconciliation
    ) THEN
        RAISE EXCEPTION 'typed cognition graph descriptor omitted its normalized source';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_obligation_materializations_require_source
AFTER INSERT ON cognition_obligation_materializations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_materialization_source_reverse();
CREATE CONSTRAINT TRIGGER cognition_plan_revisions_require_source
AFTER INSERT ON cognition_plan_revisions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_materialization_source_reverse();

CREATE OR REPLACE FUNCTION require_exact_cognition_plan_revision()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_reconciliations reconciliations
        JOIN cognition_runtime_snapshots snapshots
          ON snapshots.snapshot_sha256=reconciliations.snapshot_sha256
        JOIN cognition_episodes episodes ON episodes.episode_id=reconciliations.episode_id
        JOIN cognition_obligation_graphs graphs
          ON graphs.episode_id=NEW.episode_id
         AND graphs.graph_version=NEW.expected_graph_version
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
          AND graphs.graph_sha256=NEW.expected_graph_sha256
          AND (graphs.graph_json::jsonb->>'generation')::bigint=NEW.previous_generation
          AND episodes.completion_authority_json::jsonb=
              NEW.descriptor_json::jsonb->'completion_authority'
    ) THEN
        RAISE EXCEPTION 'cognition plan revision lacks exact reconciliation and graph authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_plan_revisions_exact
AFTER INSERT ON cognition_plan_revisions DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_plan_revision();

CREATE OR REPLACE FUNCTION require_exact_cognition_plan_revision_application()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_plan_revisions revisions
        JOIN cognition_actions actions
          ON actions.reconciliation_id=revisions.reconciliation_id
         AND actions.snapshot_sha256=revisions.source_snapshot_sha256
         AND actions.decision_sha256=revisions.source_decision_sha256
        JOIN cognition_transitions transitions
          ON transitions.episode_id=actions.episode_id AND transitions.action_id=actions.action_id
        JOIN cognition_obligation_graphs graphs
          ON graphs.episode_id=NEW.episode_id AND graphs.graph_version=NEW.output_graph_version
        JOIN cognition_action_events events
          ON events.action_id=actions.action_id AND events.status='succeeded'
        WHERE revisions.plan_revision_id=NEW.plan_revision_id
          AND revisions.episode_id=NEW.episode_id
          AND revisions.expected_graph_version=NEW.input_graph_version
          AND revisions.result_graph_sha256=NEW.result_graph_sha256
          AND actions.action_id=NEW.action_id AND actions.status='succeeded'
          AND actions.result_revision=NEW.transition_revision AND transitions.terminal=FALSE
          AND events.actor_attempt=NEW.actor_attempt
          AND events.actor_worker_id=NEW.actor_worker_id
          AND graphs.command_id=NEW.plan_revision_id AND graphs.command_kind='plan_revision'
          AND graphs.graph_sha256=NEW.result_graph_sha256
          AND (graphs.graph_json::jsonb->>'generation')::bigint=revisions.next_generation
    ) THEN
        RAISE EXCEPTION 'cognition plan revision application lacks exact successful action and graph';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_plan_revision_applications_exact
AFTER INSERT ON cognition_plan_revision_applications DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_plan_revision_application();

CREATE OR REPLACE FUNCTION require_cognition_plan_revision_graph_application()
RETURNS TRIGGER AS $$
DECLARE
    revision cognition_plan_revisions%ROWTYPE;
    root_payload JSONB;
    next_payload JSONB;
BEGIN
    IF NEW.command_kind<>'plan_revision' THEN RETURN NULL; END IF;
    SELECT * INTO revision FROM cognition_plan_revisions
    WHERE plan_revision_id=NEW.command_id AND episode_id=NEW.episode_id;
    IF NOT FOUND OR revision.plan_revision_sha256<>NEW.command_sha256 OR
       revision.result_graph_sha256<>NEW.graph_sha256 OR
       revision.next_generation<>(NEW.graph_json::jsonb->>'generation')::bigint OR
       NOT EXISTS (
           SELECT 1 FROM cognition_plan_revision_applications applications
           WHERE applications.plan_revision_id=NEW.command_id
             AND applications.episode_id=NEW.episode_id
             AND applications.output_graph_version=NEW.graph_version
             AND applications.result_graph_sha256=NEW.graph_sha256
       ) THEN
        RAISE EXCEPTION 'cognition plan revision graph omitted its exact application';
    END IF;
    SELECT item INTO root_payload
    FROM jsonb_array_elements(NEW.graph_json::jsonb->'obligations') item
    WHERE item->>'id'=revision.root_obligation_id;
    SELECT item INTO next_payload
    FROM jsonb_array_elements(NEW.graph_json::jsonb->'obligations') item
    WHERE item->>'id'=revision.next_obligation_id;
    IF root_payload IS NULL OR next_payload IS NULL OR
       root_payload->'desired'<>revision.descriptor_json::jsonb->'root'->'desired' OR
       root_payload->'depends_on'<>revision.descriptor_json::jsonb->'root'->'depends_on' OR
       root_payload->'supporting_refs'<>revision.descriptor_json::jsonb->'root'->'supporting_refs' OR
       root_payload->'completion_check'<>revision.descriptor_json::jsonb->'root'->'completion_check' OR
       root_payload->>'status'<>'blocked' OR
       (root_payload->>'created_generation')::bigint<>revision.next_generation OR
       next_payload->'desired'<>revision.descriptor_json::jsonb->'next'->'desired' OR
       next_payload->'depends_on'<>revision.descriptor_json::jsonb->'next'->'depends_on' OR
       next_payload->'supporting_refs'<>revision.descriptor_json::jsonb->'next'->'supporting_refs' OR
       next_payload->'completion_check'<>revision.descriptor_json::jsonb->'next'->'completion_check' OR
       next_payload->>'status'<>'active' OR
       (next_payload->>'created_generation')::bigint<>revision.next_generation THEN
        RAISE EXCEPTION 'cognition plan revision graph changed its exact root or next payload';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_plan_revision_graphs_require_application
AFTER INSERT ON cognition_obligation_graphs DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_plan_revision_graph_application();
