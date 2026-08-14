LOCK TABLE job_steps, repository_mutation_operations IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM job_steps WHERE status='running') THEN
        RAISE EXCEPTION 'step-attempt migration rejects running legacy job step; quiesce workers first';
    END IF;
    IF EXISTS (
        SELECT 1 FROM repository_mutation_operations
        WHERE status IN ('prepared','applying','indeterminate')
    ) THEN
        RAISE EXCEPTION 'step-attempt migration rejects unresolved legacy repository mutation';
    END IF;
END;
$$;

ALTER TABLE job_steps
    ADD COLUMN current_attempt BIGINT NOT NULL DEFAULT 0
        CHECK (current_attempt >= 0),
    ADD CONSTRAINT job_steps_running_attempt_required CHECK (
        status <> 'running' OR (current_attempt > 0 AND worker_id IS NOT NULL)
    );

CREATE TABLE job_step_attempts (
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation > 0),
    step_id BIGINT NOT NULL,
    attempt BIGINT NOT NULL CHECK (attempt > 0),
    worker_id TEXT NOT NULL CHECK (
        worker_id <> '' AND worker_id=BTRIM(worker_id) AND
        octet_length(worker_id)<=256
    ),
    status TEXT NOT NULL DEFAULT 'active' CHECK (
        status IN (
            'active','completed','failed','waiting_input',
            'canceled','superseded','expired'
        )
    ),
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    renewed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (
        clock_timestamp() + INTERVAL '75 seconds'
    ),
    finished_at TIMESTAMPTZ,
    PRIMARY KEY (job_id,generation,step_id,attempt),
    FOREIGN KEY (job_id,generation,step_id)
        REFERENCES job_steps(job_id,generation,id) ON DELETE RESTRICT,
    CHECK (renewed_at >= claimed_at),
    CHECK (expires_at = renewed_at + INTERVAL '75 seconds'),
    CHECK (
        (status='active' AND finished_at IS NULL) OR
        (status<>'active' AND finished_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_job_step_attempts_one_active
    ON job_step_attempts(job_id,generation,step_id)
    WHERE status='active';
CREATE INDEX idx_job_step_attempts_expiry
    ON job_step_attempts(status,expires_at,job_id,step_id);

CREATE OR REPLACE FUNCTION validate_job_step_attempt_insert()
RETURNS TRIGGER AS $$
DECLARE
    step_attempt BIGINT;
    step_generation BIGINT;
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    SELECT current_attempt,generation INTO step_attempt,step_generation
    FROM job_steps WHERE job_id=NEW.job_id AND id=NEW.step_id FOR UPDATE;
    IF NOT FOUND OR step_generation<>NEW.generation THEN
        RAISE EXCEPTION 'step attempt has no exact job generation step';
    END IF;
    IF NEW.attempt<>step_attempt+1 THEN
        RAISE EXCEPTION 'step attempt must increase monotonically by one';
    END IF;
    IF NEW.status<>'active' OR NEW.finished_at IS NOT NULL THEN
        RAISE EXCEPTION 'new step attempt must be active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_step_attempt_insert_validate
BEFORE INSERT ON job_step_attempts
FOR EACH ROW EXECUTE FUNCTION validate_job_step_attempt_insert();

CREATE OR REPLACE FUNCTION prevent_job_step_attempt_invalid_change()
RETURNS TRIGGER AS $$
BEGIN
    NEW.expires_at := NEW.renewed_at + INTERVAL '75 seconds';
    IF ROW(OLD.job_id,OLD.generation,OLD.step_id,OLD.attempt,OLD.worker_id,OLD.claimed_at)
       IS DISTINCT FROM
       ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.attempt,NEW.worker_id,NEW.claimed_at) THEN
        RAISE EXCEPTION 'step attempt identity is immutable';
    END IF;
    IF OLD.status<>'active' THEN
        RAISE EXCEPTION 'terminal step attempt is immutable';
    END IF;
    IF NEW.status='active' THEN
        IF NEW.finished_at IS NOT NULL OR NEW.renewed_at<=OLD.renewed_at THEN
            RAISE EXCEPTION 'active step attempt renewal is invalid';
        END IF;
    ELSIF NEW.renewed_at<>OLD.renewed_at OR NEW.finished_at IS NULL THEN
        RAISE EXCEPTION 'step attempt terminal transition is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_step_attempt_change_validate
BEFORE UPDATE ON job_step_attempts
FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_invalid_change();

CREATE OR REPLACE FUNCTION prevent_job_step_attempt_removal()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'step attempt history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_step_attempt_delete_immutable
BEFORE DELETE ON job_step_attempts
FOR EACH ROW EXECUTE FUNCTION prevent_job_step_attempt_removal();
CREATE TRIGGER job_step_attempt_truncate_immutable
BEFORE TRUNCATE ON job_step_attempts
FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_attempt_removal();

ALTER TABLE context_projections
    ADD COLUMN step_attempt BIGINT,
    ADD COLUMN worker_id TEXT,
    ADD CONSTRAINT context_projections_attempt_authority_complete CHECK (
        (step_attempt IS NULL AND worker_id IS NULL) OR
        (step_attempt > 0 AND worker_id IS NOT NULL AND worker_id <> '')
    ),
    ADD CONSTRAINT context_projections_step_attempt_fkey
        FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT;

ALTER TABLE llm_call_evidence
    ADD COLUMN step_attempt BIGINT,
    ADD COLUMN worker_id TEXT,
    ADD CONSTRAINT llm_call_evidence_attempt_authority_complete CHECK (
        (step_attempt IS NULL AND worker_id IS NULL) OR
        (step_attempt > 0 AND worker_id IS NOT NULL AND worker_id <> '')
    ),
    ADD CONSTRAINT llm_call_evidence_step_attempt_fkey
        FOREIGN KEY (job_id,job_generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT;

ALTER TABLE repository_mutation_operations
    ADD COLUMN step_attempt BIGINT CHECK (step_attempt > 0),
    ADD CONSTRAINT repository_mutation_operations_step_attempt_fkey
        FOREIGN KEY (job_id,generation,step_id,step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT;
