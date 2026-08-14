DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM cognition_obligations obligations
        JOIN cognition_episodes episodes ON episodes.episode_id=obligations.episode_id
        WHERE obligations.created_generation<>episodes.generation
    ) THEN
        RAISE EXCEPTION 'migration 057 cannot infer job generation from already divergent plan generations';
    END IF;
    IF EXISTS (
        SELECT 1 FROM cognition_obligation_graphs graphs
        CROSS JOIN LATERAL jsonb_array_elements(graphs.graph_json::jsonb->'obligations') item
        WHERE jsonb_typeof(item->'depends_on')<>'array'
           OR jsonb_typeof(item->'supporting_refs')<>'array'
    ) THEN
        RAISE EXCEPTION 'migration 057 requires explicit obligation dependency and evidence arrays';
    END IF;
END;
$$;

ALTER TABLE cognition_obligations ADD COLUMN job_generation BIGINT;
ALTER TABLE cognition_obligations DISABLE TRIGGER cognition_obligations_immutable;
UPDATE cognition_obligations SET job_generation=created_generation;
ALTER TABLE cognition_obligations ENABLE TRIGGER cognition_obligations_immutable;
ALTER TABLE cognition_obligations
    ALTER COLUMN job_generation SET NOT NULL,
    ADD CONSTRAINT cognition_obligations_job_generation_check CHECK (job_generation>0),
    DROP CONSTRAINT cognition_obligations_episode_id_job_id_created_generation_fkey,
    ADD CONSTRAINT cognition_obligations_episode_job_generation_fk
        FOREIGN KEY (episode_id,job_id,job_generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT;

ALTER TABLE task_node_generation_supersessions ADD COLUMN job_generation BIGINT;
ALTER TABLE task_node_generation_supersessions DISABLE TRIGGER task_node_supersessions_immutable;
UPDATE task_node_generation_supersessions supersessions
SET job_generation=events.job_generation
FROM task_events events
WHERE events.ledger_id=supersessions.ledger_id
  AND events.job_id=supersessions.job_id
  AND events.ledger_version=supersessions.created_version
  AND events.event_kind='node_generation_superseded'
  AND (events.payload->>'retiring_generation')::BIGINT=supersessions.retiring_generation
  AND (events.payload->>'superseded_at_generation')::BIGINT=supersessions.superseded_at_generation;
ALTER TABLE task_node_generation_supersessions ENABLE TRIGGER task_node_supersessions_immutable;

DO $$
DECLARE
    old_fk TEXT;
    old_fk_count INT;
BEGIN
    IF EXISTS (SELECT 1 FROM task_node_generation_supersessions WHERE job_generation IS NULL) THEN
        RAISE EXCEPTION 'migration 057 cannot recover task supersession job-generation authority';
    END IF;
    SELECT COUNT(*),MIN(constraints.constraint_name) INTO old_fk_count,old_fk
    FROM information_schema.constraint_column_usage usage
    JOIN information_schema.table_constraints constraints
      ON constraints.constraint_schema=usage.constraint_schema
     AND constraints.constraint_name=usage.constraint_name
    WHERE constraints.table_schema=current_schema()
      AND constraints.table_name='task_node_generation_supersessions'
      AND constraints.constraint_type='FOREIGN KEY'
      AND usage.table_name='job_generations'
      AND usage.column_name='generation';
    IF old_fk_count<>1 THEN
        RAISE EXCEPTION 'expected one legacy task supersession generation FK, found %',old_fk_count;
    END IF;
    EXECUTE format(
        'ALTER TABLE task_node_generation_supersessions DROP CONSTRAINT %I',old_fk
    );
END;
$$;

ALTER TABLE task_node_generation_supersessions
    ALTER COLUMN job_generation SET NOT NULL,
    ADD CONSTRAINT task_node_generation_supersessions_job_generation_check
        CHECK (job_generation>0),
    ADD CONSTRAINT task_node_generation_supersessions_job_generation_fk
        FOREIGN KEY (job_id,job_generation)
        REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT;

CREATE TABLE cognition_obligation_dependencies (
    episode_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    dependency_node_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (episode_id,node_id,dependency_node_id),
    FOREIGN KEY (episode_id,node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (episode_id,dependency_node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED,
    CHECK (node_id<>dependency_node_id)
);

CREATE TABLE cognition_obligation_supporting_refs (
    episode_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    observation_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(observation_id)),
    revision_number BIGINT NOT NULL CHECK (revision_number>0),
    revision_sha256 TEXT NOT NULL CHECK (revision_sha256~'^[0-9a-f]{64}$'),
    content_sha256 TEXT NOT NULL CHECK (content_sha256~'^[0-9a-f]{64}$'),
    ref_json TEXT NOT NULL CHECK (
        jsonb_typeof(ref_json::jsonb)='object' AND octet_length(ref_json)<=131072
    ),
    ref_json_sha256 TEXT NOT NULL CHECK (
        ref_json_sha256~'^[0-9a-f]{64}$' AND
        ref_json_sha256=encode(digest(ref_json,'sha256'),'hex')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (episode_id,node_id,observation_id),
    FOREIGN KEY (episode_id,node_id)
        REFERENCES cognition_obligations(episode_id,node_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED,
    CHECK (ref_json::jsonb->>'observation_id'=observation_id),
    CHECK ((ref_json::jsonb->'revision'->>'episode_id')=episode_id),
    CHECK ((ref_json::jsonb->'revision'->>'number')::bigint=revision_number),
    CHECK (ref_json::jsonb->'revision'->>'sha256'=revision_sha256),
    CHECK (ref_json::jsonb->>'sha256'=content_sha256)
);

INSERT INTO cognition_obligation_dependencies (episode_id,node_id,dependency_node_id)
SELECT graphs.episode_id,item->>'id',dependency.value
FROM cognition_obligation_graphs graphs
JOIN LATERAL jsonb_array_elements(graphs.graph_json::jsonb->'obligations') item ON TRUE
JOIN LATERAL jsonb_array_elements_text(item->'depends_on') dependency(value) ON TRUE
WHERE graphs.graph_version=(
    SELECT MAX(current.graph_version) FROM cognition_obligation_graphs current
    WHERE current.episode_id=graphs.episode_id
);

INSERT INTO cognition_obligation_supporting_refs (
    episode_id,node_id,observation_id,revision_number,revision_sha256,
    content_sha256,ref_json,ref_json_sha256
)
SELECT graphs.episode_id,item->>'id',ref->>'observation_id',
       (ref->'revision'->>'number')::bigint,ref->'revision'->>'sha256',
       ref->>'sha256',ref::text,encode(digest(ref::text,'sha256'),'hex')
FROM cognition_obligation_graphs graphs
JOIN LATERAL jsonb_array_elements(graphs.graph_json::jsonb->'obligations') item ON TRUE
JOIN LATERAL jsonb_array_elements(item->'supporting_refs') ref ON TRUE
WHERE graphs.graph_version=(
    SELECT MAX(current.graph_version) FROM cognition_obligation_graphs current
    WHERE current.episode_id=graphs.episode_id
);

CREATE TRIGGER cognition_obligation_dependencies_immutable
BEFORE UPDATE OR DELETE ON cognition_obligation_dependencies
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_dependencies_no_truncate
BEFORE TRUNCATE ON cognition_obligation_dependencies
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_supporting_refs_immutable
BEFORE UPDATE OR DELETE ON cognition_obligation_supporting_refs
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_supporting_refs_no_truncate
BEFORE TRUNCATE ON cognition_obligation_supporting_refs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
