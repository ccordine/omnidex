BEGIN;

LOCK TABLE evidence, job_lifecycle_operations, lifecycle_operation_registry,
    job_step_attempts, job_steps IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM evidence WHERE kind='objective_citation') THEN
        RAISE EXCEPTION
            'cannot install objective completion evidence authority while unauthenticated objective citations exist';
    END IF;
END $$;

CREATE TABLE step_completion_evidence_sets (
    operation_id TEXT PRIMARY KEY CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    attempt BIGINT NOT NULL CHECK (attempt>0),
    worker_id TEXT NOT NULL CHECK (
        worker_id<>'' AND worker_id=BTRIM(worker_id) AND
        octet_length(worker_id)<=256
    ),
    evidence_count INTEGER NOT NULL CHECK (
        evidence_count>=0 AND evidence_count<=32
    ),
    records_json JSONB NOT NULL CHECK (
        jsonb_typeof(records_json)='array'
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (evidence_count=jsonb_array_length(records_json)),
    FOREIGN KEY (operation_id)
        REFERENCES job_lifecycle_operations(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (job_id,generation,step_id,attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt)
        ON DELETE RESTRICT
);

ALTER TABLE evidence
    ADD COLUMN completion_operation_id TEXT,
    ADD COLUMN completion_evidence_index INTEGER,
    ADD CONSTRAINT evidence_objective_completion_authority CHECK (
        (kind='objective_citation' AND completion_operation_id IS NOT NULL AND
         completion_evidence_index IS NOT NULL AND completion_evidence_index>=0) OR
        (kind<>'objective_citation' AND completion_operation_id IS NULL AND
         completion_evidence_index IS NULL)
    ),
    ADD CONSTRAINT evidence_completion_set_fkey
        FOREIGN KEY (completion_operation_id)
        REFERENCES step_completion_evidence_sets(operation_id)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX evidence_completion_set_exact_index
    ON evidence(completion_operation_id,completion_evidence_index)
    WHERE completion_operation_id IS NOT NULL;

CREATE FUNCTION objective_completion_evidence_set_is_valid(
    requested_operation_id TEXT
) RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1
        FROM step_completion_evidence_sets AS authority
        JOIN job_lifecycle_operations AS operation
          ON operation.operation_id=authority.operation_id
        JOIN job_step_attempts AS attempt
          ON attempt.job_id=authority.job_id AND
             attempt.generation=authority.generation AND
             attempt.step_id=authority.step_id AND
             attempt.attempt=authority.attempt
        WHERE authority.operation_id=requested_operation_id AND
              operation.kind='complete_step' AND
              operation.command_payload->>'context_key'='objective_result' AND
              operation.job_id=authority.job_id AND
              operation.observed_generation=authority.generation AND
              operation.result_generation=authority.generation AND
              operation.step_id=authority.step_id AND
              operation.result_step_status='completed' AND
              attempt.worker_id=authority.worker_id AND
              attempt.status='completed' AND
              (SELECT COUNT(*) FROM evidence AS exact_evidence
               WHERE exact_evidence.completion_operation_id=authority.operation_id)
                  = authority.evidence_count AND
              NOT EXISTS (
                  SELECT 1
                  FROM jsonb_array_elements(authority.records_json)
                       WITH ORDINALITY AS item(payload,ordinality)
                  LEFT JOIN evidence AS exact_evidence
                    ON exact_evidence.completion_operation_id=authority.operation_id AND
                       exact_evidence.completion_evidence_index=item.ordinality-1
                  WHERE jsonb_typeof(item.payload) IS DISTINCT FROM 'object' OR
                        exact_evidence.id IS NULL OR
                        exact_evidence.job_id IS DISTINCT FROM authority.job_id OR
                        exact_evidence.step_id IS DISTINCT FROM authority.step_id OR
                        exact_evidence.kind IS DISTINCT FROM 'objective_citation' OR
                        exact_evidence.source_type IS DISTINCT FROM item.payload->>'source_type' OR
                        exact_evidence.source_ref IS DISTINCT FROM item.payload->>'source_ref' OR
                        exact_evidence.payload_json IS DISTINCT FROM item.payload OR
                        item.payload->>'job_id' IS DISTINCT FROM authority.job_id::TEXT OR
                        item.payload->>'step_id' IS DISTINCT FROM authority.step_id::TEXT OR
                        item.payload->>'kind' IS DISTINCT FROM 'objective_citation'
              )
    );
$$ LANGUAGE SQL STABLE STRICT;

CREATE FUNCTION validate_objective_completion_evidence_set()
RETURNS TRIGGER AS $$
BEGIN
    IF objective_completion_evidence_set_is_valid(NEW.operation_id)
       IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence set % does not match one exact completed attempt',
            NEW.operation_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER step_completion_evidence_sets_validate
AFTER INSERT ON step_completion_evidence_sets
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_set();

CREATE FUNCTION validate_objective_completion_evidence_row()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.completion_operation_id IS NOT NULL AND
       objective_completion_evidence_set_is_valid(NEW.completion_operation_id)
           IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION
            'objective completion evidence row does not match its exact completion set';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER evidence_validate_objective_completion_set
AFTER INSERT ON evidence
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_objective_completion_evidence_row();

CREATE FUNCTION require_objective_completion_evidence_set()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind='complete_step' AND
       NEW.command_payload->>'context_key'='objective_result' THEN
        IF objective_completion_evidence_set_is_valid(NEW.operation_id)
           IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION
                'objective lifecycle completion requires one exact evidence set';
        END IF;
    ELSIF EXISTS (
        SELECT 1 FROM step_completion_evidence_sets
        WHERE operation_id=NEW.operation_id
    ) THEN
        RAISE EXCEPTION
            'non-objective lifecycle operation cannot own objective evidence';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER job_lifecycle_operations_require_objective_evidence
AFTER INSERT ON job_lifecycle_operations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_objective_completion_evidence_set();

CREATE FUNCTION prevent_objective_completion_evidence_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'objective completion evidence authority is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER step_completion_evidence_sets_immutable
BEFORE UPDATE OR DELETE ON step_completion_evidence_sets
FOR EACH ROW EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();
CREATE TRIGGER step_completion_evidence_sets_no_truncate
BEFORE TRUNCATE ON step_completion_evidence_sets
FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();
CREATE TRIGGER objective_completion_evidence_update_immutable
BEFORE UPDATE ON evidence
FOR EACH ROW WHEN (
    OLD.kind='objective_citation' OR OLD.completion_operation_id IS NOT NULL OR
    NEW.kind='objective_citation' OR NEW.completion_operation_id IS NOT NULL
) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();
CREATE TRIGGER objective_completion_evidence_delete_immutable
BEFORE DELETE ON evidence
FOR EACH ROW WHEN (
    OLD.kind='objective_citation' OR OLD.completion_operation_id IS NOT NULL
) EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();
CREATE TRIGGER objective_completion_evidence_no_truncate
BEFORE TRUNCATE ON evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_objective_completion_evidence_mutation();

COMMIT;
