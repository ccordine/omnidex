CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_id_job_step
    ON artifacts (id, job_id, step_id);

CREATE TABLE task_artifact_projections (
    artifact_id BIGINT PRIMARY KEY,
    job_id BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    job_generation BIGINT NOT NULL CHECK (job_generation > 0),
    ledger_id TEXT NOT NULL,
    artifact_kind TEXT NOT NULL CHECK (artifact_kind = 'intent'),
    artifact_version TEXT NOT NULL CHECK (artifact_version = '1'),
    projection_schema TEXT NOT NULL
        CHECK (projection_schema = 'omnidex.accepted-intent-projection.v1'),
    payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    objective_node_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(objective_node_id)),
    ledger_start_version BIGINT NOT NULL CHECK (ledger_start_version > 0),
    ledger_end_version BIGINT NOT NULL CHECK (ledger_end_version > ledger_start_version),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (artifact_id, job_id, step_id)
        REFERENCES artifacts(id, job_id, step_id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, job_generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, objective_node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    UNIQUE (job_id, artifact_kind),
    UNIQUE (artifact_id, job_id, ledger_id, payload_sha256)
);

CREATE TABLE task_artifact_projection_items (
    artifact_id BIGINT NOT NULL,
    job_id BIGINT NOT NULL,
    ledger_id TEXT NOT NULL,
    item_kind TEXT NOT NULL
        CHECK (item_kind IN ('objective', 'constraint', 'ambiguity')),
    ordinal INT NOT NULL CHECK (ordinal >= 0),
    node_id TEXT,
    entry_id TEXT,
    source_uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(source_uri)),
    source_version TEXT NOT NULL CHECK (source_version = '1'),
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (artifact_id, item_kind, ordinal),
    FOREIGN KEY (artifact_id, job_id, ledger_id, source_sha256)
        REFERENCES task_artifact_projections(
            artifact_id, job_id, ledger_id, payload_sha256
        ) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, job_id)
        REFERENCES task_ledgers(id, job_id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, node_id)
        REFERENCES task_nodes(ledger_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ledger_id, entry_id)
        REFERENCES task_entries(ledger_id, id) ON DELETE RESTRICT,
    CHECK (
        (item_kind = 'objective' AND ordinal = 0 AND node_id IS NOT NULL AND entry_id IS NULL) OR
        (item_kind IN ('constraint', 'ambiguity') AND node_id IS NULL AND entry_id IS NOT NULL)
    ),
    CHECK (
        source_uri = 'artifact://job/' || job_id::text || '/artifact/' ||
            artifact_id::text || '/intent/' || item_kind || '/' || ordinal::text
    ),
    UNIQUE (ledger_id, node_id),
    UNIQUE (ledger_id, entry_id)
);

CREATE OR REPLACE FUNCTION validate_task_artifact_projection()
RETURNS TRIGGER AS $$
DECLARE
    persisted_kind TEXT;
    persisted_version TEXT;
    persisted_hash TEXT;
    persisted_generation BIGINT;
    persisted_ledger_version BIGINT;
    persisted_node_kind TEXT;
    persisted_created_step BIGINT;
    persisted_assigned_step BIGINT;
BEGIN
    SELECT artifact.kind, artifact.version,
           encode(digest(artifact.payload_json::text, 'sha256'), 'hex'),
           step.generation, ledger.version,
           node.kind, node.created_step_id, node.assigned_step_id
    INTO persisted_kind, persisted_version, persisted_hash,
         persisted_generation, persisted_ledger_version,
         persisted_node_kind, persisted_created_step, persisted_assigned_step
    FROM artifacts AS artifact
    JOIN job_steps AS step
      ON step.job_id=artifact.job_id AND step.id=artifact.step_id
    JOIN task_ledgers AS ledger ON ledger.job_id=artifact.job_id
    JOIN task_nodes AS node
      ON node.ledger_id=ledger.id AND node.id=NEW.objective_node_id
    WHERE artifact.id=NEW.artifact_id
      AND artifact.job_id=NEW.job_id
      AND artifact.step_id=NEW.step_id
      AND ledger.id=NEW.ledger_id;
    IF NOT FOUND OR persisted_kind IS DISTINCT FROM NEW.artifact_kind OR
       persisted_version IS DISTINCT FROM NEW.artifact_version OR
       persisted_hash IS DISTINCT FROM NEW.payload_sha256 OR
       persisted_generation IS DISTINCT FROM NEW.job_generation OR
       persisted_ledger_version IS DISTINCT FROM NEW.ledger_end_version OR
       persisted_node_kind IS DISTINCT FROM 'objective' OR
       persisted_created_step IS DISTINCT FROM NEW.step_id OR
       persisted_assigned_step IS NOT NULL THEN
        RAISE EXCEPTION 'accepted task artifact projection disagrees with source authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER task_artifact_projection_validate
BEFORE INSERT ON task_artifact_projections
FOR EACH ROW EXECUTE FUNCTION validate_task_artifact_projection();

CREATE OR REPLACE FUNCTION validate_task_artifact_projection_item()
RETURNS TRIGGER AS $$
DECLARE
    projected_objective TEXT;
    target_kind TEXT;
    target_authority TEXT;
    target_scope TEXT;
    target_created_step BIGINT;
    projected_step BIGINT;
BEGIN
    SELECT objective_node_id, step_id
    INTO projected_objective, projected_step
    FROM task_artifact_projections
    WHERE artifact_id=NEW.artifact_id AND job_id=NEW.job_id
      AND ledger_id=NEW.ledger_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'accepted task artifact projection item has no projection';
    END IF;
    IF NEW.item_kind = 'objective' THEN
        IF NEW.node_id IS DISTINCT FROM projected_objective THEN
            RAISE EXCEPTION 'intent objective item disagrees with projected objective';
        END IF;
    ELSE
        SELECT kind, authority, scope_node_id, created_step_id
        INTO target_kind, target_authority, target_scope, target_created_step
        FROM task_entries
        WHERE ledger_id=NEW.ledger_id AND id=NEW.entry_id;
        IF NOT FOUND OR target_scope IS DISTINCT FROM projected_objective OR
           target_created_step IS DISTINCT FROM projected_step OR
           (NEW.item_kind = 'constraint' AND
                (target_kind <> 'constraint' OR target_authority <> 'code')) OR
           (NEW.item_kind = 'ambiguity' AND
                (target_kind <> 'question' OR target_authority <> 'model_proposal')) THEN
            RAISE EXCEPTION 'intent projection item disagrees with task entry authority';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER task_artifact_projection_item_validate
BEFORE INSERT ON task_artifact_projection_items
FOR EACH ROW EXECUTE FUNCTION validate_task_artifact_projection_item();

CREATE OR REPLACE FUNCTION prevent_task_artifact_projection_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'task artifact projections are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER task_artifact_projections_immutable
BEFORE UPDATE OR DELETE ON task_artifact_projections
FOR EACH ROW EXECUTE FUNCTION prevent_task_artifact_projection_mutation();
CREATE TRIGGER task_artifact_projection_items_immutable
BEFORE UPDATE OR DELETE ON task_artifact_projection_items
FOR EACH ROW EXECUTE FUNCTION prevent_task_artifact_projection_mutation();
CREATE TRIGGER task_artifact_projections_truncate_immutable
BEFORE TRUNCATE ON task_artifact_projections
FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_artifact_projection_mutation();
CREATE TRIGGER task_artifact_projection_items_truncate_immutable
BEFORE TRUNCATE ON task_artifact_projection_items
FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_artifact_projection_mutation();

CREATE OR REPLACE FUNCTION prevent_projected_artifact_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM task_artifact_projections WHERE artifact_id=OLD.id
    ) THEN
        RAISE EXCEPTION 'accepted task artifact is immutable';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER projected_artifacts_immutable
BEFORE UPDATE OR DELETE ON artifacts
FOR EACH ROW EXECUTE FUNCTION prevent_projected_artifact_mutation();

CREATE OR REPLACE FUNCTION require_intent_artifact_projection()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind = 'intent' AND NOT EXISTS (
        SELECT 1 FROM task_artifact_projections WHERE artifact_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'intent artifact requires an accepted task projection';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER intent_artifact_requires_projection
AFTER INSERT OR UPDATE ON artifacts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_intent_artifact_projection();
