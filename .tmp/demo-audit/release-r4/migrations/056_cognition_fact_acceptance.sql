DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_episodes) THEN
        RAISE EXCEPTION 'migration 056 cannot invent fact acceptance authority for existing cognition episodes';
    END IF;
END;
$$;

ALTER TABLE cognition_episodes
    ADD COLUMN fact_authority_json TEXT NOT NULL CHECK (
        jsonb_typeof(fact_authority_json::jsonb)='object' AND
        octet_length(fact_authority_json)<=262144
    ),
    ADD COLUMN fact_authority_sha256 TEXT NOT NULL CHECK (
        fact_authority_sha256~'^[0-9a-f]{64}$' AND
        fact_authority_sha256=encode(digest(fact_authority_json,'sha256'),'hex') AND
        fact_authority_json::jsonb->>'schema'='omnidex.cognition-fact-acceptance-authority.v1' AND
        fact_authority_json::jsonb->>'sha256'~'^[0-9a-f]{64}$' AND
        jsonb_typeof(fact_authority_json::jsonb->'planner')='object' AND
        jsonb_typeof(fact_authority_json::jsonb->'policies')='array'
    ),
    ADD COLUMN fact_authority_identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(fact_authority_identity_json::jsonb)='object' AND
        octet_length(fact_authority_identity_json)<=262144 AND
        fact_authority_identity_json::jsonb=fact_authority_json::jsonb-'sha256'
    ),
    ADD COLUMN fact_authority_identity_sha256 TEXT NOT NULL CHECK (
        fact_authority_identity_sha256~'^[0-9a-f]{64}$' AND
        fact_authority_identity_sha256=encode(digest(fact_authority_identity_json,'sha256'),'hex') AND
        fact_authority_json::jsonb->>'sha256'=fact_authority_identity_sha256
    );

CREATE TABLE cognition_episode_fact_policies (
    episode_id TEXT NOT NULL REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    position INTEGER NOT NULL CHECK (position>=0 AND position<64),
    policy_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(policy_id)),
    policy_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(policy_version)),
    policy_sha256 TEXT NOT NULL CHECK (policy_sha256~'^[0-9a-f]{64}$'),
    PRIMARY KEY (episode_id,position),
    UNIQUE (episode_id,policy_id)
);

CREATE TABLE cognition_accepted_facts (
    fact_id TEXT PRIMARY KEY CHECK (fact_id~'^cognition_accepted_fact_[0-9a-f]{64}$'),
    fact_sha256 TEXT NOT NULL CHECK (
        fact_sha256~'^[0-9a-f]{64}$' AND fact_id='cognition_accepted_fact_'||fact_sha256
    ),
    episode_id TEXT NOT NULL REFERENCES cognition_episodes(episode_id) ON DELETE RESTRICT,
    ledger_id TEXT NOT NULL,
    transition_id TEXT NOT NULL REFERENCES cognition_transitions(transition_id) ON DELETE RESTRICT,
    transition_sha256 TEXT NOT NULL CHECK (transition_sha256~'^[0-9a-f]{64}$'),
    scope_obligation_id TEXT NOT NULL,
    authority_sha256 TEXT NOT NULL CHECK (authority_sha256~'^[0-9a-f]{64}$'),
    planner_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(planner_id)),
    planner_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(planner_version)),
    planner_sha256 TEXT NOT NULL CHECK (planner_sha256~'^[0-9a-f]{64}$'),
    policy_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(policy_id)),
    policy_version TEXT NOT NULL CHECK (task_ledger_text_is_exact(policy_version)),
    policy_sha256 TEXT NOT NULL CHECK (policy_sha256~'^[0-9a-f]{64}$'),
    entry_id TEXT NOT NULL,
    command_id TEXT NOT NULL CHECK (command_id~'^command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256~'^[0-9a-f]{64}$'),
    identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(identity_json::jsonb)='object' AND octet_length(identity_json)<=524288
    ),
    identity_json_sha256 TEXT NOT NULL CHECK (
        identity_json_sha256=encode(digest(identity_json,'sha256'),'hex')
    ),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND octet_length(descriptor_json)<=524288
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (episode_id,scope_obligation_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,entry_id) REFERENCES task_entries(ledger_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id,command_id) REFERENCES task_events(ledger_id,command_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,policy_id)
        REFERENCES cognition_episode_fact_policies(episode_id,policy_id) ON DELETE RESTRICT,
    UNIQUE (ledger_id,entry_id),
    UNIQUE (ledger_id,command_id),
    CHECK (fact_sha256=identity_json_sha256),
    CHECK (identity_json::jsonb=descriptor_json::jsonb-'id'-'sha256'),
    CHECK (descriptor_json::jsonb->>'schema'='omnidex.cognition-accepted-fact.v1'),
    CHECK (descriptor_json::jsonb->>'id'=fact_id),
    CHECK (descriptor_json::jsonb->>'sha256'=fact_sha256),
    CHECK (descriptor_json::jsonb->>'episode_id'=episode_id),
    CHECK (descriptor_json::jsonb->>'ledger_id'=ledger_id),
    CHECK (descriptor_json::jsonb->>'transition_id'=transition_id),
    CHECK (descriptor_json::jsonb->>'transition_sha256'=transition_sha256),
    CHECK (descriptor_json::jsonb->>'scope_obligation_id'=scope_obligation_id),
    CHECK (descriptor_json::jsonb->>'authority_sha256'=authority_sha256),
    CHECK (descriptor_json::jsonb->'planner'=jsonb_build_object(
        'id',planner_id,'version',planner_version,'sha256',planner_sha256
    )),
    CHECK (descriptor_json::jsonb->'policy'=jsonb_build_object(
        'id',policy_id,'version',policy_version,'sha256',policy_sha256
    )),
    CHECK (descriptor_json::jsonb#>>'{mapping,EntryID}'=entry_id),
    CHECK (descriptor_json::jsonb#>>'{mapping,CommandID}'=command_id),
    CHECK (descriptor_json::jsonb#>>'{mapping,CommandSHA256}'=command_sha256),
    CHECK (descriptor_json::jsonb#>>'{mapping,SourceKind}'='accepted_fact')
);

CREATE TABLE cognition_accepted_fact_evidence (
    fact_id TEXT NOT NULL REFERENCES cognition_accepted_facts(fact_id) ON DELETE RESTRICT,
    position INTEGER NOT NULL CHECK (position>=0 AND position<64),
    observation_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(observation_id)),
    revision BIGINT NOT NULL CHECK (revision>0),
    revision_sha256 TEXT NOT NULL CHECK (revision_sha256~'^[0-9a-f]{64}$'),
    content_sha256 TEXT NOT NULL CHECK (content_sha256~'^[0-9a-f]{64}$'),
    PRIMARY KEY (fact_id,position),
    UNIQUE (fact_id,observation_id,revision)
);

CREATE OR REPLACE FUNCTION guard_cognition_episode_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.episode_id,NEW.schema_name,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.created_attempt,NEW.created_worker_id,NEW.ledger_id,NEW.working_set_id,
           NEW.scenario_id,NEW.scenario_sha256,NEW.goal_json,NEW.goal_sha256,
           NEW.completion_authority_json,NEW.completion_authority_sha256,
           NEW.action_catalog_json,NEW.action_catalog_id,NEW.action_catalog_version,
           NEW.action_catalog_sha256,NEW.runtime_budget_json,NEW.runtime_budget_sha256,
           NEW.attested_brain_json,NEW.attested_brain_sha256,
           NEW.fact_authority_json,NEW.fact_authority_sha256,
           NEW.fact_authority_identity_json,NEW.fact_authority_identity_sha256,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.episode_id,OLD.schema_name,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.created_attempt,OLD.created_worker_id,OLD.ledger_id,OLD.working_set_id,
           OLD.scenario_id,OLD.scenario_sha256,OLD.goal_json,OLD.goal_sha256,
           OLD.completion_authority_json,OLD.completion_authority_sha256,
           OLD.action_catalog_json,OLD.action_catalog_id,OLD.action_catalog_version,
           OLD.action_catalog_sha256,OLD.runtime_budget_json,OLD.runtime_budget_sha256,
           OLD.attested_brain_json,OLD.attested_brain_sha256,
           OLD.fact_authority_json,OLD.fact_authority_sha256,
           OLD.fact_authority_identity_json,OLD.fact_authority_identity_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'cognition episode authority is immutable';
    END IF;
    IF OLD.status<>'active' THEN RAISE EXCEPTION 'terminal cognition episode is immutable'; END IF;
    IF NEW.version<>OLD.version+1 OR NOT (
        (NEW.current_revision=OLD.current_revision+1 AND NEW.action_count=OLD.action_count+1 AND
         NEW.total_cost>=OLD.total_cost) OR
        (NEW.current_revision=OLD.current_revision AND NEW.status<>'active' AND
         NEW.action_count=OLD.action_count AND NEW.total_cost=OLD.total_cost)
    ) THEN RAISE EXCEPTION 'cognition episode progress must be one transition or one terminal seal'; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_episode_fact_policies_immutable
BEFORE UPDATE OR DELETE ON cognition_episode_fact_policies
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_fact_policies_no_truncate
BEFORE TRUNCATE ON cognition_episode_fact_policies
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_facts_immutable
BEFORE UPDATE OR DELETE ON cognition_accepted_facts
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_facts_no_truncate
BEFORE TRUNCATE ON cognition_accepted_facts
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_fact_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_accepted_fact_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_fact_evidence_no_truncate
BEFORE TRUNCATE ON cognition_accepted_fact_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
