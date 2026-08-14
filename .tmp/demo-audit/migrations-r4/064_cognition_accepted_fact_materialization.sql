DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cognition_transitions) THEN
        RAISE EXCEPTION 'migration 064 cannot invent accepted-fact materialization authority for existing transitions';
    END IF;
END;
$$;

CREATE TABLE cognition_accepted_fact_materializations (
    materialization_id TEXT PRIMARY KEY CHECK (
        materialization_id~'^cognition_accepted_fact_materialization_[0-9a-f]{64}$'
    ),
    materialization_sha256 TEXT NOT NULL CHECK (
        materialization_sha256~'^[0-9a-f]{64}$' AND
        materialization_id='cognition_accepted_fact_materialization_'||materialization_sha256
    ),
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    ledger_id TEXT NOT NULL,
    transition_id TEXT NOT NULL UNIQUE,
    transition_sha256 TEXT NOT NULL CHECK (transition_sha256~'^[0-9a-f]{64}$'),
    transition_revision BIGINT NOT NULL CHECK (transition_revision>0),
    action_id TEXT UNIQUE,
    call_ordinal BIGINT NOT NULL CHECK (call_ordinal>=0),
    scope_obligation_id TEXT NOT NULL,
    authority_sha256 TEXT NOT NULL CHECK (authority_sha256~'^[0-9a-f]{64}$'),
    pre_fact_ledger_version BIGINT NOT NULL CHECK (pre_fact_ledger_version>0),
    pre_fact_ledger_sha256 TEXT NOT NULL CHECK (pre_fact_ledger_sha256~'^[0-9a-f]{64}$'),
    pre_fact_ledger_json TEXT NOT NULL CHECK (
        jsonb_typeof(pre_fact_ledger_json::jsonb)='object' AND
        octet_length(pre_fact_ledger_json)<=2097152 AND
        pre_fact_ledger_json=cognition_canonical_jsonb(pre_fact_ledger_json::jsonb)
    ),
    pre_fact_ledger_json_sha256 TEXT NOT NULL CHECK (
        pre_fact_ledger_json_sha256~'^[0-9a-f]{64}$' AND
        pre_fact_ledger_json_sha256=encode(digest(pre_fact_ledger_json,'sha256'),'hex')
    ),
    member_count INTEGER NOT NULL CHECK (member_count>=0 AND member_count<=64),
    output_ledger_version BIGINT NOT NULL CHECK (
        output_ledger_version=pre_fact_ledger_version+member_count
    ),
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
    FOREIGN KEY (transition_id)
        REFERENCES cognition_transitions(transition_id) ON DELETE RESTRICT,
    FOREIGN KEY (action_id)
        REFERENCES cognition_actions(action_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,scope_obligation_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT,
    CHECK (materialization_sha256=identity_json_sha256),
    CHECK (identity_json::jsonb=payload_json::jsonb-'id'-'sha256'),
    CHECK (payload_json::jsonb->>'schema'='omnidex.cognition-accepted-fact-materialization.v1'),
    CHECK (payload_json::jsonb->>'id'=materialization_id),
    CHECK (payload_json::jsonb->>'sha256'=materialization_sha256),
    CHECK (payload_json::jsonb->>'episode_id'=episode_id),
    CHECK (payload_json::jsonb->>'ledger_id'=ledger_id),
    CHECK (payload_json::jsonb->>'transition_id'=transition_id),
    CHECK (payload_json::jsonb->>'transition_sha256'=transition_sha256),
    CHECK ((payload_json::jsonb->>'transition_revision')::BIGINT=transition_revision),
    CHECK (NULLIF(payload_json::jsonb->>'action_id','') IS NOT DISTINCT FROM action_id),
    CHECK ((payload_json::jsonb->>'call_ordinal')::BIGINT=call_ordinal),
    CHECK (payload_json::jsonb->>'scope_obligation_id'=scope_obligation_id),
    CHECK (payload_json::jsonb#>>'{fact_authority,sha256}'=authority_sha256),
    CHECK ((payload_json::jsonb->>'pre_fact_ledger_version')::BIGINT=pre_fact_ledger_version),
    CHECK (payload_json::jsonb->>'pre_fact_ledger_sha256'=pre_fact_ledger_sha256),
    CHECK (payload_json::jsonb->>'pre_fact_ledger_json_sha256'=pre_fact_ledger_json_sha256),
    CHECK (payload_json::jsonb->'pre_fact_ledger'=pre_fact_ledger_json::jsonb),
    CHECK ((pre_fact_ledger_json::jsonb->>'version')::BIGINT=pre_fact_ledger_version),
    CHECK (pre_fact_ledger_json::jsonb->>'id'=ledger_id),
    CHECK (jsonb_typeof(payload_json::jsonb->'members')='array'),
    CHECK (jsonb_array_length(payload_json::jsonb->'members')=member_count),
    CHECK ((payload_json::jsonb->>'output_ledger_version')::BIGINT=output_ledger_version),
    CHECK (payload_json::jsonb->>'output_ledger_status'=output_ledger_status),
    CHECK ((action_id IS NULL AND call_ordinal=0 AND transition_revision=1) OR
           (action_id IS NOT NULL AND call_ordinal>0 AND transition_revision>1))
);

CREATE TABLE cognition_accepted_fact_materialization_members (
    materialization_id TEXT NOT NULL REFERENCES cognition_accepted_fact_materializations(materialization_id)
        ON DELETE RESTRICT,
    position INTEGER NOT NULL CHECK (position>=0 AND position<64),
    fact_id TEXT NOT NULL UNIQUE REFERENCES cognition_accepted_facts(fact_id) ON DELETE RESTRICT,
    fact_sha256 TEXT NOT NULL CHECK (fact_sha256~'^[0-9a-f]{64}$'),
    command_id TEXT NOT NULL UNIQUE CHECK (command_id~'^command_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256~'^[0-9a-f]{64}$'),
    entry_id TEXT NOT NULL UNIQUE CHECK (task_ledger_text_is_exact(entry_id)),
    entry_uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(entry_uri)),
    output_ledger_version BIGINT NOT NULL CHECK (output_ledger_version>0),
    output_ledger_status TEXT NOT NULL CHECK (output_ledger_status='active'),
    PRIMARY KEY (materialization_id,position)
);

CREATE INDEX cognition_accepted_fact_materializations_episode_order
    ON cognition_accepted_fact_materializations(episode_id,transition_revision);

CREATE TRIGGER cognition_accepted_fact_materializations_immutable
BEFORE UPDATE OR DELETE ON cognition_accepted_fact_materializations
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_fact_materializations_no_truncate
BEFORE TRUNCATE ON cognition_accepted_fact_materializations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_fact_materialization_members_immutable
BEFORE UPDATE OR DELETE ON cognition_accepted_fact_materialization_members
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_accepted_fact_materialization_members_no_truncate
BEFORE TRUNCATE ON cognition_accepted_fact_materialization_members
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
