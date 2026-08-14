CREATE OR REPLACE FUNCTION cognition_json_object_has_exact_keys(value JSON, expected TEXT[])
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='object' AND
           COALESCE((SELECT array_agg(key ORDER BY key) FROM json_each(value)),ARRAY[]::TEXT[])=
           COALESCE((SELECT array_agg(key ORDER BY key) FROM unnest(expected) AS key),ARRAY[]::TEXT[]);
$$ LANGUAGE SQL IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION cognition_json_array_objects_have_exact_keys(value JSON, expected TEXT[])
RETURNS BOOLEAN AS $$
    SELECT json_typeof(value)='array' AND NOT EXISTS (
        SELECT 1 FROM json_array_elements(value) AS item
        WHERE NOT cognition_json_object_has_exact_keys(item,expected)
    );
$$ LANGUAGE SQL IMMUTABLE STRICT;

ALTER TABLE cognition_terminal_seals
    ADD COLUMN authority_kind TEXT NOT NULL DEFAULT 'worker',
    ADD COLUMN lifecycle_operation_id TEXT,
    ALTER COLUMN sealed_attempt DROP NOT NULL,
    ALTER COLUMN sealed_worker_id DROP NOT NULL;
ALTER TABLE cognition_terminal_seals ALTER COLUMN authority_kind DROP DEFAULT;
ALTER TABLE cognition_terminal_seals
    ADD CONSTRAINT cognition_terminal_seals_authority_kind_check CHECK (
        (authority_kind='worker' AND sealed_attempt IS NOT NULL AND sealed_attempt>0 AND
         task_ledger_text_is_exact(sealed_worker_id) AND lifecycle_operation_id IS NULL) OR
        (authority_kind='lifecycle' AND sealed_attempt IS NULL AND sealed_worker_id IS NULL AND
         lifecycle_operation_id~'^lifecycle_operation_[0-9a-f]{64}$' AND outcome='canceled')
    ),
    ADD CONSTRAINT cognition_terminal_seals_lifecycle_operation_fk
        FOREIGN KEY (lifecycle_operation_id) REFERENCES lifecycle_operation_registry(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE cognition_episode_cancellations
    DROP CONSTRAINT cognition_episode_cancellations_cancellation_code_check,
    ADD COLUMN authority_kind TEXT NOT NULL DEFAULT 'worker',
    ADD COLUMN lifecycle_operation_id TEXT,
    ALTER COLUMN actor_attempt DROP NOT NULL,
    ALTER COLUMN actor_worker_id DROP NOT NULL;
ALTER TABLE cognition_episode_cancellations ALTER COLUMN authority_kind DROP DEFAULT;
ALTER TABLE cognition_episode_cancellations
    ADD CONSTRAINT cognition_episode_cancellations_authority_check CHECK (
        (authority_kind='worker' AND
         cancellation_code IN ('policy_failure','run_budget_exhausted') AND
         actor_attempt IS NOT NULL AND actor_attempt>0 AND
         task_ledger_text_is_exact(actor_worker_id) AND lifecycle_operation_id IS NULL) OR
        (authority_kind='lifecycle' AND
         cancellation_code IN ('job_canceled','generation_superseded') AND
         actor_attempt IS NULL AND actor_worker_id IS NULL AND
         lifecycle_operation_id~'^lifecycle_operation_[0-9a-f]{64}$')
    ),
    ADD CONSTRAINT cognition_episode_cancellations_lifecycle_operation_fk
        FOREIGN KEY (lifecycle_operation_id) REFERENCES lifecycle_operation_registry(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE cognition_lifecycle_retirements (
    retirement_id TEXT PRIMARY KEY CHECK (
        retirement_id~'^cognition_retirement_[0-9a-f]{64}$'
    ),
    retirement_sha256 TEXT NOT NULL CHECK (
        retirement_sha256~'^[0-9a-f]{64}$' AND
        retirement_id='cognition_retirement_'||retirement_sha256
    ),
    identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(identity_json::jsonb)='object' AND
        cognition_json_object_has_exact_keys(identity_json::json,ARRAY[
            'schema','id','sha256','operation_id','operation_kind','operation_sha256',
            'episode_id','job_id','job_generation','step_id','code','expected_revision',
            'graph_version','graph_sha256']) AND
        cognition_json_object_has_exact_keys((identity_json::json->'expected_revision')::json,
            ARRAY['episode_id','number','sha256']) AND
        retirement_sha256=encode(digest(identity_json,'sha256'),'hex') AND
        identity_json::jsonb=jsonb_set(jsonb_set(
            descriptor_json::jsonb,'{id}',to_jsonb(''::TEXT)),
            '{sha256}',to_jsonb(''::TEXT))
    ),
    descriptor_json TEXT NOT NULL CHECK (
        jsonb_typeof(descriptor_json::jsonb)='object' AND
        cognition_json_object_has_exact_keys(descriptor_json::json,ARRAY[
            'schema','id','sha256','operation_id','operation_kind','operation_sha256',
            'episode_id','job_id','job_generation','step_id','code','expected_revision',
            'graph_version','graph_sha256']) AND
        cognition_json_object_has_exact_keys((descriptor_json::json->'expected_revision')::json,
            ARRAY['episode_id','number','sha256']) AND
        octet_length(descriptor_json)<=65536
    ),
    descriptor_json_sha256 TEXT NOT NULL CHECK (
        descriptor_json_sha256~'^[0-9a-f]{64}$' AND
        descriptor_json_sha256=encode(digest(descriptor_json,'sha256'),'hex')
    ),
    operation_id TEXT NOT NULL,
    operation_kind TEXT NOT NULL CHECK (operation_kind IN ('cancel_job','replan_job')),
    operation_sha256 TEXT NOT NULL CHECK (operation_sha256~'^[0-9a-f]{64}$'),
    episode_id TEXT NOT NULL UNIQUE,
    job_id BIGINT NOT NULL,
    job_generation BIGINT NOT NULL CHECK (job_generation>0),
    step_id BIGINT NOT NULL,
    cancellation_code TEXT NOT NULL CHECK (
        (operation_kind='cancel_job' AND cancellation_code='job_canceled') OR
        (operation_kind='replan_job' AND cancellation_code='generation_superseded')
    ),
    expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
    expected_revision_sha256 TEXT NOT NULL CHECK (expected_revision_sha256~'^[0-9a-f]{64}$'),
    graph_version BIGINT NOT NULL CHECK (graph_version>0),
    graph_sha256 TEXT NOT NULL CHECK (graph_sha256~'^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (operation_id,operation_kind,operation_sha256)
        REFERENCES lifecycle_operation_registry(operation_id,kind,command_sha256)
        ON DELETE RESTRICT,
    FOREIGN KEY (episode_id,job_id,job_generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    UNIQUE (operation_id,episode_id),
    CHECK (
        descriptor_json::jsonb ?& ARRAY[
            'schema','id','sha256','operation_id','operation_kind','operation_sha256',
            'episode_id','job_id','job_generation','step_id','code','expected_revision',
            'graph_version','graph_sha256'] AND
        descriptor_json::jsonb-ARRAY[
            'schema','id','sha256','operation_id','operation_kind','operation_sha256',
            'episode_id','job_id','job_generation','step_id','code','expected_revision',
            'graph_version','graph_sha256']='{}'::jsonb AND
        descriptor_json::jsonb->>'schema'='omnidex.cognition-lifecycle-retirement.v1' AND
        descriptor_json::jsonb->>'id'=retirement_id AND
        descriptor_json::jsonb->>'sha256'=retirement_sha256 AND
        descriptor_json::jsonb->>'operation_id'=operation_id AND
        descriptor_json::jsonb->>'operation_kind'=operation_kind AND
        descriptor_json::jsonb->>'operation_sha256'=operation_sha256 AND
        descriptor_json::jsonb->>'episode_id'=episode_id AND
        (descriptor_json::jsonb->>'job_id')::BIGINT=job_id AND
        (descriptor_json::jsonb->>'job_generation')::BIGINT=job_generation AND
        (descriptor_json::jsonb->>'step_id')::BIGINT=step_id AND
        descriptor_json::jsonb->>'code'=cancellation_code AND
        descriptor_json::jsonb->'expected_revision' ?& ARRAY['episode_id','number','sha256'] AND
        (descriptor_json::jsonb->'expected_revision')-ARRAY['episode_id','number','sha256']='{}'::jsonb AND
        descriptor_json::jsonb#>>'{expected_revision,episode_id}'=episode_id AND
        (descriptor_json::jsonb#>>'{expected_revision,number}')::BIGINT=expected_revision AND
        descriptor_json::jsonb#>>'{expected_revision,sha256}'=expected_revision_sha256 AND
        (descriptor_json::jsonb->>'graph_version')::BIGINT=graph_version AND
        descriptor_json::jsonb->>'graph_sha256'=graph_sha256
    )
);

CREATE TABLE cognition_lifecycle_operation_seals (
    operation_id TEXT PRIMARY KEY,
    operation_kind TEXT NOT NULL CHECK (operation_kind IN ('cancel_job','replan_job')),
    operation_sha256 TEXT NOT NULL CHECK (operation_sha256~'^[0-9a-f]{64}$'),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    episode_count INT NOT NULL CHECK (episode_count>=0 AND episode_count<=16384),
    seal_set_json TEXT NOT NULL CHECK (
        jsonb_typeof(seal_set_json::jsonb)='object' AND
        cognition_json_object_has_exact_keys(seal_set_json::json,ARRAY[
            'schema','operation_id','operation_kind','operation_sha256','job_id',
            'generation','entries','sha256']) AND
        cognition_json_array_objects_have_exact_keys((seal_set_json::json->'entries')::json,
            ARRAY['episode_id','retirement_id','retirement_sha256','trace_sha256']) AND
        octet_length(seal_set_json)<=2097152
    ),
    seal_set_sha256 TEXT NOT NULL CHECK (
        seal_set_sha256~'^[0-9a-f]{64}$'
    ),
    identity_json TEXT NOT NULL CHECK (
        jsonb_typeof(identity_json::jsonb)='object' AND
        cognition_json_object_has_exact_keys(identity_json::json,ARRAY[
            'schema','operation_id','operation_kind','operation_sha256','job_id',
            'generation','entries','sha256']) AND
        cognition_json_array_objects_have_exact_keys((identity_json::json->'entries')::json,
            ARRAY['episode_id','retirement_id','retirement_sha256','trace_sha256']) AND
        seal_set_sha256=encode(digest(identity_json,'sha256'),'hex') AND
        identity_json::jsonb=jsonb_set(
            seal_set_json::jsonb,'{sha256}',to_jsonb(''::TEXT))
    ),
    seal_set_json_sha256 TEXT NOT NULL CHECK (
        seal_set_json_sha256~'^[0-9a-f]{64}$' AND
        seal_set_json_sha256=encode(digest(seal_set_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (operation_id,operation_kind,operation_sha256)
        REFERENCES lifecycle_operation_registry(operation_id,kind,command_sha256) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation) REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT,
    CHECK (
        seal_set_json::jsonb ?& ARRAY[
            'schema','operation_id','operation_kind','operation_sha256','job_id',
            'generation','entries','sha256'] AND
        seal_set_json::jsonb-ARRAY[
            'schema','operation_id','operation_kind','operation_sha256','job_id',
            'generation','entries','sha256']='{}'::jsonb AND
        seal_set_json::jsonb->>'schema'='omnidex.cognition-lifecycle-seal-set.v1' AND
        seal_set_json::jsonb->>'operation_id'=operation_id AND
        seal_set_json::jsonb->>'operation_kind'=operation_kind AND
        seal_set_json::jsonb->>'operation_sha256'=operation_sha256 AND
        (seal_set_json::jsonb->>'job_id')::BIGINT=job_id AND
        (seal_set_json::jsonb->>'generation')::BIGINT=generation AND
        jsonb_typeof(seal_set_json::jsonb->'entries')='array' AND
        jsonb_array_length(seal_set_json::jsonb->'entries')=episode_count AND
        seal_set_json::jsonb->>'sha256'=seal_set_sha256
    )
);

CREATE TABLE cognition_lifecycle_operation_seal_episodes (
    operation_id TEXT NOT NULL REFERENCES cognition_lifecycle_operation_seals(operation_id) ON DELETE RESTRICT,
    position INT NOT NULL CHECK (position>=0),
    episode_id TEXT NOT NULL,
    retirement_id TEXT NOT NULL,
    retirement_sha256 TEXT NOT NULL CHECK (retirement_sha256~'^[0-9a-f]{64}$'),
    trace_sha256 TEXT NOT NULL CHECK (trace_sha256~'^[0-9a-f]{64}$'),
    PRIMARY KEY (operation_id,position),
    UNIQUE (operation_id,episode_id),
    FOREIGN KEY (retirement_id) REFERENCES cognition_lifecycle_retirements(retirement_id) ON DELETE RESTRICT,
    FOREIGN KEY (episode_id) REFERENCES cognition_terminal_seals(episode_id) ON DELETE RESTRICT
);

CREATE TRIGGER cognition_lifecycle_retirements_immutable BEFORE UPDATE OR DELETE
ON cognition_lifecycle_retirements FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_lifecycle_operation_seals_immutable BEFORE UPDATE OR DELETE
ON cognition_lifecycle_operation_seals FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_lifecycle_seal_episodes_immutable BEFORE UPDATE OR DELETE
ON cognition_lifecycle_operation_seal_episodes FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_lifecycle_retirements_no_truncate BEFORE TRUNCATE
ON cognition_lifecycle_retirements FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_lifecycle_operation_seals_no_truncate BEFORE TRUNCATE
ON cognition_lifecycle_operation_seals FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_lifecycle_seal_episodes_no_truncate BEFORE TRUNCATE
ON cognition_lifecycle_operation_seal_episodes FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
