CREATE UNIQUE INDEX idx_step_contexts_step_identity
    ON step_contexts (step_id, id);

CREATE TABLE job_lifecycle_operations (
    operation_id TEXT PRIMARY KEY CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    observed_generation BIGINT NOT NULL CHECK (observed_generation > 0),
    result_generation BIGINT NOT NULL CHECK (result_generation > 0),
    step_id BIGINT,
    step_context_id BIGINT,
    kind TEXT NOT NULL CHECK (
        kind IN ('complete_step', 'fail_step', 'submit_feedback', 'replan_job')
    ),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_payload JSONB NOT NULL CHECK (
        jsonb_typeof(command_payload) = 'object' AND
        command_payload ? 'operation_id' AND
        command_payload ->> 'operation_id' = operation_id
    ),
    CHECK (
        (kind = 'complete_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'output', 'context_key', 'context_value'] AND
            command_payload - ARRAY['operation_id', 'step_id', 'output', 'context_key', 'context_value'] = '{}'::jsonb) OR
        (kind = 'fail_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'error'] AND
            command_payload - ARRAY['operation_id', 'step_id', 'error'] = '{}'::jsonb) OR
        (kind IN ('submit_feedback', 'replan_job') AND
            command_payload ?& ARRAY['operation_id', 'job_id', 'feedback'] AND
            command_payload - ARRAY['operation_id', 'job_id', 'feedback'] = '{}'::jsonb)
    ),
    result_job_status TEXT NOT NULL CHECK (
        result_job_status IN ('pending', 'running', 'completed', 'failed', 'canceled', 'waiting_input')
    ),
    result_step_status TEXT CHECK (
        result_step_status IS NULL OR
        result_step_status IN ('completed', 'failed')
    ),
    result_job JSONB NOT NULL CHECK (
        jsonb_typeof(result_job) = 'object' AND
        result_job ? 'id' AND result_job ? 'current_generation' AND result_job ? 'status' AND
        (result_job ->> 'id')::BIGINT = job_id AND
        (result_job ->> 'current_generation')::BIGINT = result_generation AND
        result_job ->> 'status' = result_job_status
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (job_id, observed_generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, result_generation)
        REFERENCES job_generations(job_id, generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id, observed_generation, step_id)
        REFERENCES job_steps(job_id, generation, id) ON DELETE RESTRICT,
    FOREIGN KEY (step_id, step_context_id)
        REFERENCES step_contexts(step_id, id) ON DELETE RESTRICT,
    CHECK (
        (kind = 'complete_step' AND step_id IS NOT NULL AND
            ((command_payload ->> 'context_key' = '' AND step_context_id IS NULL) OR
             (command_payload ->> 'context_key' <> '' AND step_context_id IS NOT NULL)) AND
            result_generation = observed_generation AND result_step_status = 'completed') OR
        (kind = 'fail_step' AND step_id IS NOT NULL AND step_context_id IS NULL AND
            result_generation = observed_generation AND result_step_status = 'failed' AND
            result_job_status = 'failed') OR
        (kind = 'submit_feedback' AND step_id IS NOT NULL AND step_context_id IS NOT NULL AND
            result_generation = observed_generation AND result_step_status = 'completed') OR
        (kind = 'replan_job' AND step_id IS NULL AND step_context_id IS NULL AND result_step_status IS NULL AND
            result_generation = observed_generation + 1 AND result_job_status = 'running')
    ),
    CHECK (
        CASE
            WHEN kind IN ('complete_step', 'fail_step') THEN
                command_payload ? 'step_id' AND
                (command_payload ->> 'step_id')::BIGINT = step_id
            ELSE
                command_payload ? 'job_id' AND
                (command_payload ->> 'job_id')::BIGINT = job_id
        END
    )
);

CREATE INDEX idx_job_lifecycle_operations_job_generation
    ON job_lifecycle_operations (job_id, result_generation, operation_id);
CREATE INDEX idx_job_lifecycle_operations_step
    ON job_lifecycle_operations (step_id, operation_id)
    WHERE step_id IS NOT NULL;

CREATE OR REPLACE FUNCTION prevent_job_lifecycle_operation_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'job lifecycle operation records are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_lifecycle_operations_immutable
BEFORE UPDATE OR DELETE ON job_lifecycle_operations
FOR EACH ROW EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();

CREATE TRIGGER job_lifecycle_operations_truncate_immutable
BEFORE TRUNCATE ON job_lifecycle_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_lifecycle_operation_mutation();
