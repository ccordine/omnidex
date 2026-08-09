LOCK TABLE jobs, job_steps, step_contexts, artifacts, evidence, claims,
    claim_support, llm_call_evidence, memory_candidates, task_events
IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_job_id BIGINT;
BEGIN
    SELECT jobs.id
    INTO invalid_job_id
    FROM jobs
    WHERE jobs.status NOT IN ('completed', 'failed', 'canceled')
      AND EXISTS (
          SELECT 1
          FROM job_steps
          JOIN step_contexts ON step_contexts.step_id = job_steps.id
          WHERE job_steps.job_id = jobs.id
            AND step_contexts.key = 'replan_feedback'
      )
    ORDER BY jobs.id
    LIMIT 1;

    IF invalid_job_id IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install job generations: nonterminal legacy-replanned job % has ambiguous current authority',
            invalid_job_id;
    END IF;
END $$;

DO $$
DECLARE
    invalid_id BIGINT;
BEGIN
    SELECT artifacts.id
    INTO invalid_id
    FROM artifacts
    LEFT JOIN job_steps ON job_steps.id = artifacts.step_id
    WHERE (artifacts.job_id IS NULL) IS DISTINCT FROM (artifacts.step_id IS NULL)
       OR (artifacts.job_id IS NOT NULL AND artifacts.step_id IS NOT NULL AND
           (job_steps.id IS NULL OR job_steps.job_id IS DISTINCT FROM artifacts.job_id))
    ORDER BY artifacts.id
    LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: cross-job or orphan artifact %', invalid_id;
    END IF;

    SELECT evidence.id
    INTO invalid_id
    FROM evidence
    LEFT JOIN job_steps ON job_steps.id = evidence.step_id
    WHERE (evidence.job_id IS NULL) IS DISTINCT FROM (evidence.step_id IS NULL)
       OR (evidence.job_id IS NOT NULL AND evidence.step_id IS NOT NULL AND
           (job_steps.id IS NULL OR job_steps.job_id IS DISTINCT FROM evidence.job_id))
    ORDER BY evidence.id
    LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: cross-job or orphan evidence %', invalid_id;
    END IF;

    SELECT claims.id
    INTO invalid_id
    FROM claims
    LEFT JOIN job_steps ON job_steps.id = claims.step_id
    WHERE claims.job_id IS NULL
       OR claims.step_id IS NULL
       OR job_steps.job_id IS DISTINCT FROM claims.job_id
    ORDER BY claims.id
    LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: cross-job or orphan claim %', invalid_id;
    END IF;

    SELECT llm_call_evidence.id
    INTO invalid_id
    FROM llm_call_evidence
    LEFT JOIN job_steps ON job_steps.id = llm_call_evidence.step_id
    WHERE job_steps.job_id IS DISTINCT FROM llm_call_evidence.job_id
    ORDER BY llm_call_evidence.id
    LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: cross-job LLM call evidence %', invalid_id;
    END IF;

    SELECT claim_support.id
    INTO invalid_id
    FROM claim_support
    LEFT JOIN claims ON claims.id = claim_support.claim_id
    LEFT JOIN evidence ON evidence.id = claim_support.evidence_id
    WHERE claims.job_id IS NULL
       OR evidence.job_id IS NULL
       OR claims.job_id IS DISTINCT FROM evidence.job_id
    ORDER BY claim_support.id
    LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: cross-job claim support %', invalid_id;
    END IF;
END $$;

DO $$
DECLARE
    invalid_id BIGINT;
BEGIN
    SELECT id INTO invalid_id
    FROM claims
    WHERE status NOT IN ('supported', 'unsupported')
       OR confidence < 0 OR confidence > 1
    ORDER BY id LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: invalid claim semantics %', invalid_id;
    END IF;

    SELECT id INTO invalid_id
    FROM claim_support
    WHERE support_score < 0 OR support_score > 1
       OR rationale IS NULL OR rationale <> BTRIM(rationale) OR rationale = ''
    ORDER BY id LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: invalid claim support semantics %', invalid_id;
    END IF;

    SELECT id INTO invalid_id
    FROM memory_candidates
    WHERE status NOT IN ('candidate', 'approved', 'durable', 'rejected')
       OR confidence < 0 OR confidence > 1
    ORDER BY id LIMIT 1;
    IF invalid_id IS NOT NULL THEN
        RAISE EXCEPTION 'cannot install job generations: invalid memory candidate semantics %', invalid_id;
    END IF;
END $$;

ALTER TABLE jobs
    ADD COLUMN current_generation BIGINT NOT NULL DEFAULT 1;
ALTER TABLE jobs
    ADD CONSTRAINT jobs_current_generation_positive
    CHECK (current_generation > 0);

CREATE TABLE job_generations (
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    generation BIGINT NOT NULL CHECK (generation > 0),
    purpose TEXT NOT NULL CHECK (purpose IN ('initial', 'replan')),
    predecessor_generation BIGINT,
    boundary_action TEXT,
    feedback TEXT,
    feedback_sha256 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, generation),
    FOREIGN KEY (job_id, predecessor_generation)
        REFERENCES job_generations(job_id, generation)
        ON DELETE RESTRICT,
    CHECK (
        (generation = 1 AND purpose = 'initial' AND
            predecessor_generation IS NULL AND boundary_action IS NULL AND
            feedback IS NULL AND feedback_sha256 IS NULL) OR
        (generation > 1 AND purpose = 'replan' AND
            predecessor_generation = generation - 1 AND
            boundary_action IN ('v3_coding', 'v3_planning') AND
            task_ledger_text_is_exact(feedback) AND
            octet_length(feedback) <= 65536 AND
            feedback_sha256 ~ '^[0-9a-f]{64}$' AND
            feedback_sha256 = encode(digest(feedback, 'sha256'), 'hex'))
    )
);

CREATE OR REPLACE FUNCTION prevent_job_generation_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'job generation records are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_generations_immutable
BEFORE UPDATE OR DELETE ON job_generations
FOR EACH ROW EXECUTE FUNCTION prevent_job_generation_mutation();

CREATE TRIGGER job_generations_truncate_immutable
BEFORE TRUNCATE ON job_generations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_generation_mutation();

INSERT INTO job_generations (job_id, generation, purpose)
SELECT jobs.id, 1, 'initial'
FROM jobs
ORDER BY jobs.id;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_current_generation_fkey
    FOREIGN KEY (id, current_generation)
    REFERENCES job_generations(job_id, generation)
    DEFERRABLE INITIALLY DEFERRED;

CREATE OR REPLACE FUNCTION enforce_job_current_generation_advance()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.current_generation IS DISTINCT FROM NEW.current_generation AND
       NEW.current_generation <> OLD.current_generation + 1 THEN
        RAISE EXCEPTION 'job current generation must advance exactly once';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_current_generation_exact_advance
BEFORE UPDATE OF current_generation ON jobs
FOR EACH ROW EXECUTE FUNCTION enforce_job_current_generation_advance();

ALTER TABLE task_events
    ADD COLUMN job_generation BIGINT NOT NULL DEFAULT 1;
ALTER TABLE task_events
    ALTER COLUMN job_generation DROP DEFAULT,
    ADD CONSTRAINT task_events_job_generation_fkey
        FOREIGN KEY (job_id, job_generation)
        REFERENCES job_generations(job_id, generation)
        ON DELETE RESTRICT;

CREATE INDEX idx_task_events_job_generation
    ON task_events (job_id, job_generation, id);

ALTER TABLE job_steps
    ADD COLUMN generation BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN superseded_at_generation BIGINT;
ALTER TABLE job_steps
    ADD CONSTRAINT job_steps_generation_positive CHECK (generation > 0),
    ADD CONSTRAINT job_steps_superseded_generation_order
        CHECK (superseded_at_generation IS NULL OR superseded_at_generation > generation),
    ADD CONSTRAINT job_steps_generation_fkey
        FOREIGN KEY (job_id, generation)
        REFERENCES job_generations(job_id, generation)
        ON DELETE RESTRICT,
    ADD CONSTRAINT job_steps_superseded_generation_fkey
        FOREIGN KEY (job_id, superseded_at_generation)
        REFERENCES job_generations(job_id, generation)
        ON DELETE RESTRICT;

CREATE UNIQUE INDEX idx_job_steps_job_generation_id
    ON job_steps (job_id, generation, id);
CREATE INDEX idx_job_steps_current_generation_sort
    ON job_steps (job_id, generation, sort_index, id)
    WHERE superseded_at_generation IS NULL;
CREATE INDEX idx_job_steps_current_generation_action
    ON job_steps (job_id, generation, action, id)
    WHERE superseded_at_generation IS NULL;

CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.job_id IS DISTINCT FROM NEW.job_id OR
       OLD.generation IS DISTINCT FROM NEW.generation THEN
        RAISE EXCEPTION 'job step generation identity is immutable';
    END IF;
    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'job step execution identity cannot return to pending';
    END IF;
    IF OLD.superseded_at_generation IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'superseded job step history is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_steps_generation_identity_immutable
BEFORE UPDATE ON job_steps
FOR EACH ROW EXECUTE FUNCTION prevent_job_step_generation_identity_mutation();

CREATE OR REPLACE FUNCTION prevent_job_step_history_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'job step generation history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_steps_history_delete_immutable
BEFORE DELETE ON job_steps
FOR EACH ROW EXECUTE FUNCTION prevent_job_step_history_delete();

CREATE TRIGGER job_steps_history_truncate_immutable
BEFORE TRUNCATE ON job_steps
FOR EACH STATEMENT EXECUTE FUNCTION prevent_job_step_history_delete();

ALTER TABLE job_steps
    ALTER COLUMN generation DROP DEFAULT;

ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_job_step_shape CHECK (
        (job_id IS NULL AND step_id IS NULL) OR
        (job_id IS NOT NULL AND step_id IS NOT NULL)
    ),
    ADD CONSTRAINT artifacts_job_step_fkey
        FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id)
        ON DELETE RESTRICT;

ALTER TABLE evidence
    ADD CONSTRAINT evidence_job_step_shape CHECK (
        (job_id IS NULL AND step_id IS NULL) OR
        (job_id IS NOT NULL AND step_id IS NOT NULL)
    ),
    ADD CONSTRAINT evidence_job_step_fkey
        FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id)
        ON DELETE RESTRICT;

ALTER TABLE claims
	ALTER COLUMN job_id SET NOT NULL,
	ALTER COLUMN step_id SET NOT NULL,
	ADD CONSTRAINT claims_status_registered
		CHECK (status IN ('supported', 'unsupported')),
	ADD CONSTRAINT claims_confidence_bounded
		CHECK (confidence >= 0 AND confidence <= 1),
	ADD CONSTRAINT claims_job_step_fkey
        FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id)
        ON DELETE RESTRICT;

ALTER TABLE llm_call_evidence
    ADD CONSTRAINT llm_call_evidence_job_step_fkey
        FOREIGN KEY (job_id, step_id)
        REFERENCES job_steps(job_id, id)
        ON DELETE RESTRICT;

CREATE UNIQUE INDEX idx_claims_job_id_id
    ON claims (job_id, id);
CREATE UNIQUE INDEX idx_evidence_job_id_id
    ON evidence (job_id, id);

ALTER TABLE claim_support
    ADD COLUMN job_id BIGINT;
UPDATE claim_support
SET job_id = claims.job_id
FROM claims
WHERE claims.id = claim_support.claim_id;
ALTER TABLE claim_support
	ALTER COLUMN job_id SET NOT NULL,
	ALTER COLUMN rationale SET NOT NULL,
	ADD CONSTRAINT claim_support_score_bounded
		CHECK (support_score >= 0 AND support_score <= 1),
	ADD CONSTRAINT claim_support_rationale_exact
		CHECK (rationale <> '' AND rationale = BTRIM(rationale)),
    ADD CONSTRAINT claim_support_job_claim_fkey
        FOREIGN KEY (job_id, claim_id)
        REFERENCES claims(job_id, id)
        ON DELETE CASCADE,
    ADD CONSTRAINT claim_support_job_evidence_fkey
        FOREIGN KEY (job_id, evidence_id)
        REFERENCES evidence(job_id, id)
        ON DELETE CASCADE;

CREATE INDEX idx_claim_support_job_claim
    ON claim_support (job_id, claim_id, support_score DESC);

ALTER TABLE memory_candidates
    ADD COLUMN generation BIGINT;
UPDATE memory_candidates
SET generation = 1
WHERE job_id IS NOT NULL;
ALTER TABLE memory_candidates
	DROP CONSTRAINT memory_candidates_job_id_fkey,
	ADD CONSTRAINT memory_candidates_status_registered
		CHECK (status IN ('candidate', 'approved', 'durable', 'rejected')),
	ADD CONSTRAINT memory_candidates_confidence_bounded
		CHECK (confidence >= 0 AND confidence <= 1),
    ADD CONSTRAINT memory_candidates_job_id_fkey
        FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE RESTRICT,
    ADD CONSTRAINT memory_candidates_generation_shape CHECK (
        (job_id IS NULL AND generation IS NULL) OR
        (job_id IS NOT NULL AND generation IS NOT NULL AND generation > 0)
    ),
    ADD CONSTRAINT memory_candidates_job_generation_fkey
        FOREIGN KEY (job_id, generation)
        REFERENCES job_generations(job_id, generation)
        ON DELETE RESTRICT;

CREATE INDEX idx_memory_candidates_job_generation_status
    ON memory_candidates (job_id, generation, status, id)
    WHERE job_id IS NOT NULL;
