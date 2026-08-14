CREATE TABLE lifecycle_operation_registry (
    operation_id TEXT PRIMARY KEY CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    kind TEXT NOT NULL CHECK (
        kind IN (
            'complete_step', 'fail_step', 'submit_feedback', 'replan_job',
            'scrum_channel_message', 'cancel_job'
        )
    ),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_payload JSONB NOT NULL CHECK (
        jsonb_typeof(command_payload) = 'object' AND
        command_payload ? 'operation_id' AND
        command_payload ->> 'operation_id' = operation_id
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (operation_id, kind, command_sha256)
);

INSERT INTO lifecycle_operation_registry (
    operation_id, kind, command_sha256, command_payload, created_at
)
SELECT operation_id, kind, command_sha256, command_payload, created_at
FROM job_lifecycle_operations;

ALTER TABLE job_lifecycle_operations
    DROP CONSTRAINT job_lifecycle_operations_kind_check,
    DROP CONSTRAINT job_lifecycle_operations_check1,
    DROP CONSTRAINT job_lifecycle_operations_check3,
    ADD CONSTRAINT job_lifecycle_operations_kind_check CHECK (
        kind IN ('complete_step', 'fail_step', 'submit_feedback', 'replan_job', 'cancel_job')
    ),
    ADD CONSTRAINT job_lifecycle_operations_check1 CHECK (
        (kind = 'complete_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'output', 'context_key', 'context_value'] AND
            command_payload - ARRAY['operation_id', 'step_id', 'output', 'context_key', 'context_value'] = '{}'::jsonb) OR
        (kind = 'fail_step' AND
            command_payload ?& ARRAY['operation_id', 'step_id', 'error'] AND
            command_payload - ARRAY['operation_id', 'step_id', 'error'] = '{}'::jsonb) OR
        (kind IN ('submit_feedback', 'replan_job') AND
            command_payload ?& ARRAY['operation_id', 'job_id', 'feedback'] AND
            command_payload - ARRAY['operation_id', 'job_id', 'feedback'] = '{}'::jsonb) OR
        (kind = 'cancel_job' AND
            command_payload ?& ARRAY['operation_id', 'job_id', 'reason'] AND
            command_payload - ARRAY['operation_id', 'job_id', 'reason'] = '{}'::jsonb)
    ),
    ADD CONSTRAINT job_lifecycle_operations_check3 CHECK (
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
            result_generation = observed_generation + 1 AND result_job_status = 'running') OR
        (kind = 'cancel_job' AND step_id IS NULL AND step_context_id IS NULL AND result_step_status IS NULL AND
            result_generation = observed_generation AND result_job_status = 'canceled')
    );

ALTER TABLE job_lifecycle_operations
    ADD CONSTRAINT job_lifecycle_operations_global_identity
    FOREIGN KEY (operation_id, kind, command_sha256)
    REFERENCES lifecycle_operation_registry(operation_id, kind, command_sha256)
    ON DELETE RESTRICT;

CREATE TABLE scrum_channel_operations (
    operation_id TEXT PRIMARY KEY CHECK (
        operation_id ~ '^lifecycle_operation_[0-9a-f]{64}$'
    ),
    project_id BIGINT NOT NULL,
    card_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind = 'scrum_channel_message'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_payload JSONB NOT NULL CHECK (
        jsonb_typeof(command_payload) = 'object' AND
        command_payload ?& ARRAY['operation_id', 'project_id', 'card_id', 'message'] AND
        command_payload - ARRAY['operation_id', 'project_id', 'card_id', 'message'] = '{}'::jsonb AND
        command_payload ->> 'operation_id' = operation_id AND
        (command_payload ->> 'project_id')::BIGINT = project_id AND
        command_payload ->> 'card_id' = card_id AND
        BTRIM(command_payload ->> 'message') <> ''
    ),
    effect_kind TEXT NOT NULL CHECK (
        effect_kind IN ('start_job', 'replan_job', 'submit_feedback')
    ),
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    result_action TEXT NOT NULL CHECK (
        result_action IN ('started', 'steered', 'revised', 'feedback')
    ),
    result_agent TEXT NOT NULL CHECK (BTRIM(result_agent) <> ''),
    result_job JSONB NOT NULL CHECK (
        jsonb_typeof(result_job) = 'object' AND
        result_job ? 'id' AND
        (result_job ->> 'id')::BIGINT = job_id
    ),
    result_card JSONB NOT NULL CHECK (
        jsonb_typeof(result_card) = 'object' AND
        result_card ?& ARRAY['id', 'project_id', 'job_id', 'chat'] AND
        result_card ->> 'id' = card_id AND
        (result_card ->> 'project_id')::BIGINT = project_id AND
        result_card ->> 'job_id' = job_id::TEXT AND
        jsonb_typeof(result_card -> 'chat') = 'array'
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (operation_id, kind, command_sha256)
        REFERENCES lifecycle_operation_registry(operation_id, kind, command_sha256)
        ON DELETE RESTRICT
);

CREATE INDEX idx_scrum_channel_operations_card
    ON scrum_channel_operations (project_id, card_id, created_at DESC, operation_id);
CREATE INDEX idx_scrum_channel_operations_job
    ON scrum_channel_operations (job_id, operation_id);

CREATE OR REPLACE FUNCTION prevent_scrum_channel_operation_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Scrum channel operation records are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER scrum_channel_operations_immutable
BEFORE UPDATE OR DELETE ON scrum_channel_operations
FOR EACH ROW EXECUTE FUNCTION prevent_scrum_channel_operation_mutation();

CREATE TRIGGER scrum_channel_operations_truncate_immutable
BEFORE TRUNCATE ON scrum_channel_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_scrum_channel_operation_mutation();

CREATE OR REPLACE FUNCTION prevent_lifecycle_operation_registry_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle operation registry records are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER lifecycle_operation_registry_immutable
BEFORE UPDATE OR DELETE ON lifecycle_operation_registry
FOR EACH ROW EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();

CREATE TRIGGER lifecycle_operation_registry_truncate_immutable
BEFORE TRUNCATE ON lifecycle_operation_registry
FOR EACH STATEMENT EXECUTE FUNCTION prevent_lifecycle_operation_registry_mutation();
