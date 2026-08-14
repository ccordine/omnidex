LOCK TABLE worker_skills, worker_skill_checks, worker_skill_embeddings,
    jobs, job_steps, job_step_attempts IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM worker_skills) THEN
        RAISE EXCEPTION
            'cannot install independent worker-skill promotion authority while pre-cutover skill versions exist';
    END IF;
END;
$$;

DROP TRIGGER worker_skills_immutable_content ON worker_skills;
DROP FUNCTION prevent_worker_skill_content_update();

ALTER TABLE worker_skills
    DROP CONSTRAINT worker_skills_origin_check,
    DROP CONSTRAINT worker_skills_skill_kind_check,
    DROP CONSTRAINT worker_skills_check,
    DROP CONSTRAINT worker_skills_check1,
    DROP COLUMN preferred_models,
    DROP COLUMN allowed_tools,
    DROP COLUMN forbidden_tools,
    DROP COLUMN context_budget,
    DROP COLUMN stop_conditions,
    DROP COLUMN retry_policy,
    DROP COLUMN require_evidence,
    ADD CONSTRAINT worker_skills_learned_origin CHECK (origin='learned'),
    ADD CONSTRAINT worker_skills_code_procedure_kind CHECK (skill_kind='code_procedure'),
    ADD CONSTRAINT worker_skills_creating_job_required CHECK (created_by_job_id IS NOT NULL);

CREATE UNIQUE INDEX idx_worker_skills_one_pending_version
    ON worker_skills(skill_id)
    WHERE status IN ('candidate','validating');

CREATE FUNCTION prevent_worker_skill_content_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(
        OLD.skill_id, OLD.version, OLD.origin, OLD.skill_kind, OLD.purpose,
        OLD.instructions, OLD.input_schema, OLD.output_schema,
        OLD.content_sha256, OLD.created_by_job_id, OLD.created_at
    ) IS DISTINCT FROM ROW(
        NEW.skill_id, NEW.version, NEW.origin, NEW.skill_kind, NEW.purpose,
        NEW.instructions, NEW.input_schema, NEW.output_schema,
        NEW.content_sha256, NEW.created_by_job_id, NEW.created_at
    ) THEN
        RAISE EXCEPTION 'worker skill content is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skills_immutable_content
BEFORE UPDATE ON worker_skills
FOR EACH ROW EXECUTE FUNCTION prevent_worker_skill_content_update();

CREATE FUNCTION validate_worker_skill_insert()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status<>'candidate' OR NEW.validation<>'[]'::jsonb OR
       NEW.activated_at IS NOT NULL OR NEW.rejected_at IS NOT NULL OR
       NEW.retired_at IS NOT NULL THEN
        RAISE EXCEPTION 'new worker skill versions must begin as candidate';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skills_validate_insert
BEFORE INSERT ON worker_skills
FOR EACH ROW EXECUTE FUNCTION validate_worker_skill_insert();

CREATE TABLE worker_skill_promotion_receipts (
    skill_id TEXT NOT NULL,
    skill_version INTEGER NOT NULL,
    content_sha256 TEXT NOT NULL CHECK (content_sha256~'^[0-9a-f]{64}$'),
    replay_job_id BIGINT NOT NULL,
    replay_generation BIGINT NOT NULL CHECK (replay_generation>0),
    replay_step_id BIGINT NOT NULL,
    replay_step_attempt BIGINT NOT NULL CHECK (replay_step_attempt>0),
    replay_worker_id TEXT NOT NULL CHECK (
        replay_worker_id<>'' AND replay_worker_id=BTRIM(replay_worker_id) AND
        octet_length(replay_worker_id)<=256
    ),
    held_out_fixture_set_sha256 TEXT NOT NULL CHECK (
        held_out_fixture_set_sha256~'^[0-9a-f]{64}$'
    ),
    isolated_stage_result_sha256 TEXT NOT NULL CHECK (
        isolated_stage_result_sha256~'^[0-9a-f]{64}$'
    ),
    workspace_verification_sha256 TEXT NOT NULL CHECK (
        workspace_verification_sha256~'^[0-9a-f]{64}$'
    ),
    held_out_case_count INTEGER NOT NULL CHECK (
        held_out_case_count BETWEEN 2 AND 1000
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (skill_id,skill_version),
    FOREIGN KEY (skill_id,skill_version)
        REFERENCES worker_skills(skill_id,version) ON DELETE RESTRICT,
    FOREIGN KEY (replay_job_id,replay_generation,replay_step_id,replay_step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT
);

CREATE FUNCTION validate_worker_skill_promotion_receipt_insert()
RETURNS TRIGGER AS $$
DECLARE
    skill_created_by_job_id BIGINT;
    skill_status TEXT;
    skill_origin TEXT;
    skill_kind_value TEXT;
    skill_content_sha256 TEXT;
    job_status TEXT;
    job_generation BIGINT;
    step_status TEXT;
    step_generation BIGINT;
    step_attempt BIGINT;
    step_worker_id TEXT;
    step_superseded_at BIGINT;
    attempt_status TEXT;
    attempt_worker_id TEXT;
    attempt_expires_at TIMESTAMPTZ;
BEGIN
    SELECT created_by_job_id,status,origin,skill_kind,content_sha256
    INTO skill_created_by_job_id,skill_status,skill_origin,skill_kind_value,skill_content_sha256
    FROM worker_skills
    WHERE skill_id=NEW.skill_id AND version=NEW.skill_version
    FOR UPDATE;
    IF NOT FOUND OR skill_status<>'candidate' OR skill_origin<>'learned' OR
       skill_kind_value<>'code_procedure' OR skill_content_sha256<>NEW.content_sha256 THEN
        RAISE EXCEPTION 'worker skill promotion receipt has no exact immutable candidate';
    END IF;
    IF skill_created_by_job_id=NEW.replay_job_id THEN
        RAISE EXCEPTION 'worker skill promotion replay job must differ from its creating job';
    END IF;

    SELECT status,current_generation INTO job_status,job_generation
    FROM jobs WHERE id=NEW.replay_job_id FOR UPDATE;
    SELECT status,generation,current_attempt,worker_id,superseded_at_generation
    INTO step_status,step_generation,step_attempt,step_worker_id,step_superseded_at
    FROM job_steps
    WHERE job_id=NEW.replay_job_id AND id=NEW.replay_step_id
    FOR UPDATE;
    SELECT status,worker_id,expires_at
    INTO attempt_status,attempt_worker_id,attempt_expires_at
    FROM job_step_attempts
    WHERE job_id=NEW.replay_job_id AND generation=NEW.replay_generation AND
          step_id=NEW.replay_step_id AND attempt=NEW.replay_step_attempt
    FOR UPDATE;
    IF job_status<>'running' OR job_generation<>NEW.replay_generation OR
       step_status<>'running' OR step_generation<>NEW.replay_generation OR
       step_attempt<>NEW.replay_step_attempt OR step_worker_id<>NEW.replay_worker_id OR
       step_superseded_at IS NOT NULL OR attempt_status<>'active' OR
       attempt_worker_id<>NEW.replay_worker_id OR attempt_expires_at<=clock_timestamp() THEN
        RAISE EXCEPTION 'worker skill promotion receipt requires one exact active replay attempt';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skill_promotion_receipts_validate_insert
BEFORE INSERT ON worker_skill_promotion_receipts
FOR EACH ROW EXECUTE FUNCTION validate_worker_skill_promotion_receipt_insert();

CREATE FUNCTION prevent_worker_skill_promotion_receipt_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'worker skill promotion receipts are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skill_promotion_receipts_immutable_update
BEFORE UPDATE ON worker_skill_promotion_receipts
FOR EACH ROW EXECUTE FUNCTION prevent_worker_skill_promotion_receipt_change();
CREATE TRIGGER worker_skill_promotion_receipts_immutable_delete
BEFORE DELETE ON worker_skill_promotion_receipts
FOR EACH ROW EXECUTE FUNCTION prevent_worker_skill_promotion_receipt_change();
CREATE TRIGGER worker_skill_promotion_receipts_immutable_truncate
BEFORE TRUNCATE ON worker_skill_promotion_receipts
FOR EACH STATEMENT EXECUTE FUNCTION prevent_worker_skill_promotion_receipt_change();

CREATE FUNCTION validate_worker_skill_status_transition()
RETURNS TRIGGER AS $$
DECLARE
    passed_checks INTEGER;
BEGIN
    IF OLD.status=NEW.status THEN
        RETURN NEW;
    END IF;
    IF NOT (
        (OLD.status='candidate' AND NEW.status IN ('validating','rejected')) OR
        (OLD.status='validating' AND NEW.status IN ('active','rejected')) OR
        (OLD.status='active' AND NEW.status='retired')
    ) THEN
        RAISE EXCEPTION 'worker skill status transition % -> % is forbidden',OLD.status,NEW.status;
    END IF;
    IF NEW.status IN ('validating','active') AND NOT EXISTS (
        SELECT 1 FROM worker_skill_promotion_receipts receipt
        WHERE receipt.skill_id=NEW.skill_id AND receipt.skill_version=NEW.version AND
              receipt.content_sha256=NEW.content_sha256
    ) THEN
        RAISE EXCEPTION 'worker skill validation and activation require an independent promotion receipt';
    END IF;
    IF NEW.status='active' THEN
        SELECT COUNT(*) INTO passed_checks
        FROM worker_skill_checks
        WHERE skill_id=NEW.skill_id AND skill_version=NEW.version AND status='passed' AND
              check_name IN ('contract','held_out_replay','held_out_workspace_verification');
        IF passed_checks<>3 OR JSONB_TYPEOF(NEW.validation)<>'array' OR
           JSONB_ARRAY_LENGTH(NEW.validation)<>3 THEN
            RAISE EXCEPTION 'worker skill activation lacks exact held-out replay evidence';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER worker_skills_validate_status_transition
BEFORE UPDATE OF status ON worker_skills
FOR EACH ROW EXECUTE FUNCTION validate_worker_skill_status_transition();
