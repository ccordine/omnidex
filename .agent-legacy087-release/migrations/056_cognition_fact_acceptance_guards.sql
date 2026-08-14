CREATE OR REPLACE FUNCTION require_cognition_episode_fact_authority()
RETURNS TRIGGER AS $$
DECLARE normalized JSONB;
BEGIN
    SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'id',policy_id,'version',policy_version,'sha256',policy_sha256
    ) ORDER BY position),'[]'::jsonb)
      INTO normalized
      FROM cognition_episode_fact_policies WHERE episode_id=NEW.episode_id;
    IF NEW.fact_authority_json::jsonb->'policies' IS DISTINCT FROM normalized OR
       NOT task_ledger_text_is_exact(NEW.fact_authority_json::jsonb#>>'{planner,id}') OR
       NOT task_ledger_text_is_exact(NEW.fact_authority_json::jsonb#>>'{planner,version}') OR
       NEW.fact_authority_json::jsonb#>>'{planner,sha256}' !~ '^[0-9a-f]{64}$' THEN
        RAISE EXCEPTION 'cognition episode lacks exact normalized fact authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episodes_exact_fact_authority
AFTER INSERT ON cognition_episodes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_episode_fact_authority();

CREATE OR REPLACE FUNCTION require_cognition_episode_fact_policy()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_episodes episodes
        WHERE episodes.episode_id=NEW.episode_id
          AND episodes.fact_authority_json::jsonb->'policies'->NEW.position=
              jsonb_build_object('id',NEW.policy_id,'version',NEW.policy_version,'sha256',NEW.policy_sha256)
          AND jsonb_array_length(episodes.fact_authority_json::jsonb->'policies')=(
              SELECT COUNT(*) FROM cognition_episode_fact_policies policies
              WHERE policies.episode_id=NEW.episode_id
          )
    ) THEN RAISE EXCEPTION 'normalized cognition fact policy differs from episode authority'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episode_fact_policies_exact_authority
AFTER INSERT ON cognition_episode_fact_policies DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_episode_fact_policy();

CREATE OR REPLACE FUNCTION require_exact_cognition_accepted_fact()
RETURNS TRIGGER AS $$
DECLARE exact_evidence JSONB;
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM cognition_episodes episodes
          JOIN cognition_transitions transitions ON transitions.transition_id=NEW.transition_id
          JOIN cognition_obligations obligations
            ON obligations.episode_id=NEW.episode_id AND obligations.node_id=NEW.scope_obligation_id
          JOIN cognition_episode_fact_policies policies
            ON policies.episode_id=NEW.episode_id AND policies.policy_id=NEW.policy_id
          JOIN task_entries entries
            ON entries.ledger_id=NEW.ledger_id AND entries.id=NEW.entry_id
          JOIN task_events events
            ON events.ledger_id=NEW.ledger_id AND events.command_id=NEW.command_id
         WHERE episodes.episode_id=NEW.episode_id AND episodes.ledger_id=NEW.ledger_id
           AND episodes.fact_authority_json::jsonb->>'sha256'=NEW.authority_sha256
           AND episodes.fact_authority_json::jsonb->'planner'=jsonb_build_object(
               'id',NEW.planner_id,'version',NEW.planner_version,'sha256',NEW.planner_sha256
           )
           AND policies.policy_version=NEW.policy_version AND policies.policy_sha256=NEW.policy_sha256
           AND transitions.episode_id=NEW.episode_id
           AND transitions.transition_sha256=NEW.transition_sha256
           AND obligations.ledger_id=NEW.ledger_id
           AND entries.scope_node_id=NEW.scope_obligation_id
           AND entries.kind='fact' AND entries.status='active'
           AND entries.authority='code' AND entries.created_by='code'
           AND entries.metadata->>'schema'='omnidex.cognition-state-entry-mapping.v1'
           AND entries.metadata->>'source_kind'='accepted_fact'
           AND entries.metadata->'acceptance_policy'=jsonb_build_object(
               'id',NEW.policy_id,'version',NEW.policy_version,'sha256',NEW.policy_sha256
           )
           AND entries.metadata->>'source_sha256'=NEW.descriptor_json::jsonb#>>'{mapping,SourceSHA256}'
           AND entries.created_version=(NEW.descriptor_json::jsonb#>>'{mapping,ExpectedVersion}')::BIGINT+1
           AND events.ledger_version=entries.created_version
           AND events.command_kind='add_entry' AND events.event_kind='entry_added'
           AND events.actor='code' AND events.command_sha256=NEW.command_sha256
           AND NEW.descriptor_json::jsonb#>>'{mapping,Schema}'='omnidex.cognition-state-entry-mapping.v1'
           AND NEW.descriptor_json::jsonb#>>'{mapping,LedgerID}'=NEW.ledger_id
           AND NEW.descriptor_json::jsonb#>>'{mapping,Actor}'='code'
    ) THEN
        RAISE EXCEPTION 'accepted cognition fact lacks exact episode, transition, policy, and ledger authority';
    END IF;

    SELECT jsonb_agg(jsonb_build_object(
        'observation_id',evidence.observation_id,
        'revision',jsonb_build_object(
            'episode_id',NEW.episode_id,'number',evidence.revision,'sha256',evidence.revision_sha256
        ),
        'sha256',evidence.content_sha256
    ) ORDER BY evidence.position)
      INTO exact_evidence
      FROM cognition_accepted_fact_evidence evidence WHERE evidence.fact_id=NEW.fact_id;
    IF NEW.descriptor_json::jsonb->'evidence_refs' IS DISTINCT FROM exact_evidence OR
       EXISTS (
           SELECT 1 FROM cognition_accepted_fact_evidence evidence
           WHERE evidence.fact_id=NEW.fact_id
             AND NOT EXISTS (
                 SELECT 1 FROM cognition_transition_observations observations
                 WHERE observations.transition_id=NEW.transition_id
                   AND observations.observation_id=evidence.observation_id
                   AND observations.content_sha256=evidence.content_sha256
                   AND (observations.observation_json::jsonb#>>'{revision,number}')::BIGINT=evidence.revision
                   AND observations.observation_json::jsonb#>>'{revision,sha256}'=evidence.revision_sha256
                   AND observations.observation_json::jsonb#>>'{revision,episode_id}'=NEW.episode_id
             )
       ) OR EXISTS (
           SELECT 1 FROM cognition_accepted_fact_evidence evidence
           WHERE evidence.fact_id=NEW.fact_id AND NOT EXISTS (
               SELECT 1 FROM task_entry_refs refs
               WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.entry_id
                 AND refs.position=evidence.position
                 AND refs.uri='cognition:episode/'||NEW.episode_id||'/observation/'||evidence.observation_id
                 AND refs.version=evidence.revision::TEXT
                 AND refs.content_sha256=evidence.content_sha256 AND refs.relation='evidence'
           )
       ) OR (SELECT COUNT(*) FROM task_entry_refs refs
             WHERE refs.ledger_id=NEW.ledger_id AND refs.entry_id=NEW.entry_id)<>
            (SELECT COUNT(*) FROM cognition_accepted_fact_evidence evidence
             WHERE evidence.fact_id=NEW.fact_id) THEN
        RAISE EXCEPTION 'accepted cognition fact lacks exact transition evidence and ledger lineage';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_facts_exact_authority
AFTER INSERT ON cognition_accepted_facts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_exact_cognition_accepted_fact();

CREATE OR REPLACE FUNCTION require_cognition_accepted_fact_evidence()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_accepted_facts facts
        JOIN cognition_transition_observations observations
          ON observations.transition_id=facts.transition_id
         AND observations.observation_id=NEW.observation_id
        JOIN task_entry_refs refs
          ON refs.ledger_id=facts.ledger_id AND refs.entry_id=facts.entry_id
         AND refs.position=NEW.position
        WHERE facts.fact_id=NEW.fact_id
          AND facts.descriptor_json::jsonb->'evidence_refs'->NEW.position=jsonb_build_object(
              'observation_id',NEW.observation_id,
              'revision',jsonb_build_object(
                  'episode_id',facts.episode_id,'number',NEW.revision,'sha256',NEW.revision_sha256
              ),'sha256',NEW.content_sha256
          )
          AND observations.content_sha256=NEW.content_sha256
          AND observations.observation_json::jsonb#>>'{revision,episode_id}'=facts.episode_id
          AND (observations.observation_json::jsonb#>>'{revision,number}')::BIGINT=NEW.revision
          AND observations.observation_json::jsonb#>>'{revision,sha256}'=NEW.revision_sha256
          AND refs.uri='cognition:episode/'||facts.episode_id||'/observation/'||NEW.observation_id
          AND refs.version=NEW.revision::TEXT AND refs.content_sha256=NEW.content_sha256
          AND refs.relation='evidence'
    ) THEN RAISE EXCEPTION 'accepted cognition fact evidence lacks exact normalized authority'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_accepted_fact_evidence_exact_authority
AFTER INSERT ON cognition_accepted_fact_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_accepted_fact_evidence();

CREATE OR REPLACE FUNCTION require_cognition_accepted_fact_reverse()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind='fact' AND NEW.authority='code' AND
       NEW.metadata->>'source_kind'='accepted_fact' AND NOT EXISTS (
        SELECT 1 FROM cognition_accepted_facts facts
        WHERE facts.ledger_id=NEW.ledger_id AND facts.entry_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'code-derived cognition fact lacks normalized acceptance authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER task_entries_require_cognition_accepted_fact
AFTER INSERT ON task_entries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_accepted_fact_reverse();
