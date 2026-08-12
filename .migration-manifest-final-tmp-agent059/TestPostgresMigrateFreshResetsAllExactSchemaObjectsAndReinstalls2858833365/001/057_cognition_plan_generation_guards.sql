CREATE OR REPLACE FUNCTION require_exact_cognition_obligation_support()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_transitions transitions
        JOIN cognition_transition_observations observations
          ON observations.transition_id=transitions.transition_id
        WHERE transitions.episode_id=NEW.episode_id
          AND transitions.revision=NEW.revision_number
          AND transitions.current_revision_sha256=NEW.revision_sha256
          AND observations.observation_id=NEW.observation_id
          AND observations.content_sha256=NEW.content_sha256
          AND observations.observation_json::jsonb->'revision'=NEW.ref_json::jsonb->'revision'
          AND observations.observation_json::jsonb->>'id'=NEW.observation_id
          AND observations.observation_json::jsonb->>'content_sha256'=NEW.content_sha256
    ) THEN
        RAISE EXCEPTION 'cognition obligation support lacks exact durable observation evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_obligation_supporting_refs_exact
AFTER INSERT ON cognition_obligation_supporting_refs DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_support();

CREATE OR REPLACE FUNCTION require_task_node_supersession_event()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM task_events events
        WHERE events.ledger_id=NEW.ledger_id AND events.job_id=NEW.job_id
          AND events.ledger_version=NEW.created_version
          AND events.job_generation=NEW.job_generation
          AND events.event_kind='node_generation_superseded'
          AND (events.payload->>'retiring_generation')::BIGINT=NEW.retiring_generation
          AND (events.payload->>'superseded_at_generation')::BIGINT=NEW.superseded_at_generation
          AND events.payload->>'reason'=NEW.reason
          AND events.payload->'node_ids' @> to_jsonb(ARRAY[NEW.node_id]::TEXT[])
    ) THEN RAISE EXCEPTION 'task node supersession has no exact immutable event'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_exact_cognition_obligation_projection()
RETURNS TRIGGER AS $$
DECLARE
    target_episode TEXT;
    current_graph JSONB;
    normalized_count INT;
    graph_count INT;
    mismatch_node TEXT;
    mismatch_detail TEXT;
BEGIN
    target_episode := NEW.episode_id;
    SELECT graphs.graph_json::jsonb INTO current_graph
    FROM cognition_obligation_graphs graphs
    WHERE graphs.episode_id=target_episode
    ORDER BY graphs.graph_version DESC LIMIT 1;
    IF current_graph IS NULL THEN
        RAISE EXCEPTION 'cognition obligation projection has no durable graph';
    END IF;
    SELECT COUNT(*) INTO normalized_count FROM cognition_obligations obligations
    WHERE obligations.episode_id=target_episode;
    graph_count:=jsonb_array_length(current_graph->'obligations');
    IF normalized_count<>graph_count THEN
        RAISE EXCEPTION 'cognition obligation row count % differs from graph count % for episode %',
            normalized_count,graph_count,target_episode;
    END IF;
    SELECT node_id,detail INTO mismatch_node,mismatch_detail FROM (
        SELECT item->>'id' AS node_id,concat(
            'row=',obligations.node_id IS NOT NULL,
            ',parent=',COALESCE(obligations.parent_node_id,'')=COALESCE(item->>'parent_id',''),
            ',generation=',obligations.created_generation=(item->>'created_generation')::bigint,
            ',desired=',obligations.desired_json::jsonb=item->'desired',
            ',check=',item->'completion_check'=jsonb_build_object(
                'id',obligations.completion_check_id,
                'version',obligations.completion_check_version,
                'sha256',obligations.completion_check_sha256
            ),
            ',dependencies=',item->'depends_on'=COALESCE((
                SELECT jsonb_agg(dependencies.dependency_node_id
                                 ORDER BY dependencies.dependency_node_id)
                FROM cognition_obligation_dependencies dependencies
                WHERE dependencies.episode_id=target_episode
                  AND dependencies.node_id=obligations.node_id
            ),'[]'::jsonb),
            ',graph_dependencies=',item->'depends_on',
            ',row_dependencies=',COALESCE((
                SELECT jsonb_agg(dependencies.dependency_node_id
                                 ORDER BY dependencies.dependency_node_id)
                FROM cognition_obligation_dependencies dependencies
                WHERE dependencies.episode_id=target_episode
                  AND dependencies.node_id=obligations.node_id
            ),'[]'::jsonb),
            ',support=',item->'supporting_refs'=COALESCE((
                SELECT jsonb_agg(support.ref_json::jsonb ORDER BY
                    support.observation_id,
                    support.episode_id,
                    support.revision_number::text,
                    support.content_sha256)
                FROM cognition_obligation_supporting_refs support
                WHERE support.episode_id=target_episode
                  AND support.node_id=obligations.node_id
            ),'[]'::jsonb)
        ) AS detail FROM jsonb_array_elements(current_graph->'obligations') item
        LEFT JOIN cognition_obligations obligations
          ON obligations.episode_id=target_episode AND obligations.node_id=item->>'id'
        WHERE obligations.node_id IS NULL
           OR COALESCE(obligations.parent_node_id,'')<>COALESCE(item->>'parent_id','')
           OR obligations.created_generation<>(item->>'created_generation')::bigint
           OR obligations.desired_json::jsonb<>item->'desired'
           OR item->'completion_check'<>jsonb_build_object(
               'id',obligations.completion_check_id,
               'version',obligations.completion_check_version,
               'sha256',obligations.completion_check_sha256
           )
           OR item->'depends_on'<>COALESCE((
               SELECT jsonb_agg(dependencies.dependency_node_id
                                ORDER BY dependencies.dependency_node_id)
               FROM cognition_obligation_dependencies dependencies
               WHERE dependencies.episode_id=target_episode
                 AND dependencies.node_id=obligations.node_id
           ),'[]'::jsonb)
           OR item->'supporting_refs'<>COALESCE((
               SELECT jsonb_agg(support.ref_json::jsonb ORDER BY
                   support.observation_id,
                   support.episode_id,
                   support.revision_number::text,
                   support.content_sha256)
               FROM cognition_obligation_supporting_refs support
               WHERE support.episode_id=target_episode
                 AND support.node_id=obligations.node_id
           ),'[]'::jsonb)
    ) mismatch LIMIT 1;
    IF mismatch_node IS NOT NULL THEN
        RAISE EXCEPTION 'cognition obligation % graph differs from normalized authority for episode %: %',
            mismatch_node,target_episode,mismatch_detail;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_obligation_graphs_exact_projection
AFTER INSERT ON cognition_obligation_graphs DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_projection();
CREATE CONSTRAINT TRIGGER cognition_obligations_exact_projection
AFTER INSERT ON cognition_obligations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_projection();
CREATE CONSTRAINT TRIGGER cognition_obligation_dependencies_exact_projection
AFTER INSERT ON cognition_obligation_dependencies DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_projection();
CREATE CONSTRAINT TRIGGER cognition_obligation_supporting_refs_exact_projection
AFTER INSERT ON cognition_obligation_supporting_refs DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_obligation_projection();
