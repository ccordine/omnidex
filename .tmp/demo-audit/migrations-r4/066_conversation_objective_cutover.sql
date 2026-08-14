LOCK TABLE ai_channels, job_generations, jobs, job_steps, artifacts,
    task_artifact_projection_items, task_artifact_projections
IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_channel TEXT;
    invalid_job BIGINT;
    projection_count BIGINT;
    generation_constraint_sha256 TEXT;
BEGIN
    SELECT COUNT(*) INTO projection_count
    FROM task_artifact_projections;
    IF projection_count <> 0 OR EXISTS (
        SELECT 1 FROM task_artifact_projection_items
    ) OR EXISTS (
        SELECT 1 FROM artifacts WHERE kind='intent'
    ) THEN
        RAISE EXCEPTION
            'cannot install conversation objective cutover: legacy accepted-intent projection rows require an explicit retention decision';
    END IF;

    SELECT id INTO invalid_channel
    FROM ai_channels
    WHERE persona <> 'assistant'
       OR system <> ''
       OR provider <> ''
       OR model <> ''
       OR context <> '{}'::jsonb
    ORDER BY id
    LIMIT 1;
    IF invalid_channel IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install conversation objective cutover: legacy channel persona configuration remains on channel %',
            invalid_channel;
    END IF;

    SELECT jobs.id INTO invalid_job
    FROM jobs
    WHERE jobs.status NOT IN ('completed','failed','canceled')
      AND (
          EXISTS (
              SELECT 1
              FROM job_generations
              WHERE job_generations.job_id=jobs.id
                AND job_generations.generation=jobs.current_generation
                AND job_generations.boundary_action='v3_planning'
          ) OR EXISTS (
              SELECT 1
              FROM job_steps
              WHERE job_steps.job_id=jobs.id
                AND job_steps.generation=jobs.current_generation
                AND job_steps.superseded_at_generation IS NULL
                AND job_steps.action IN (
                    'v3_intent_parse','v3_capability_audit','v3_workspace_research',
                    'v3_memory_retrieval','v3_external_research','v3_planning',
                    'v3_subtask','v3_analysis','v3_response_draft','v3_verification',
                    'v3_memory_review','v3_finalize'
                )
          )
      )
    ORDER BY jobs.id
    LIMIT 1;
    IF invalid_job IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot install conversation objective cutover: nonterminal legacy conversation generation remains on job %',
            invalid_job;
    END IF;

    SELECT encode(digest(pg_get_constraintdef(con.oid, true), 'sha256'), 'hex')
    INTO generation_constraint_sha256
    FROM pg_constraint AS con
    WHERE con.conrelid='job_generations'::regclass
      AND con.conname='job_generations_check'
      AND con.contype='c'
      AND con.convalidated
      AND NOT con.connoinherit;
    IF generation_constraint_sha256 IS DISTINCT FROM
       '88a885d8fa8374bc4f771ff5f2960243997a13f73fe484d0dbeaf04fa06cd379' THEN
        RAISE EXCEPTION
            'cannot install conversation objective cutover: job generation boundary constraint differs from the frozen pre-cutover contract';
    END IF;
END $$;

ALTER TABLE ai_channels
    DROP COLUMN persona,
    DROP COLUMN system,
    DROP COLUMN provider,
    DROP COLUMN model,
    DROP COLUMN context;

DROP TRIGGER projected_artifacts_immutable ON artifacts;
DROP TRIGGER intent_artifact_requires_projection ON artifacts;
DROP TABLE task_artifact_projection_items;
DROP TABLE task_artifact_projections;
DROP INDEX idx_artifacts_id_job_step;

DROP FUNCTION validate_task_artifact_projection();
DROP FUNCTION validate_task_artifact_projection_item();
DROP FUNCTION prevent_task_artifact_projection_mutation();
DROP FUNCTION prevent_projected_artifact_mutation();
DROP FUNCTION require_intent_artifact_projection();

ALTER TABLE job_generations
    DROP CONSTRAINT job_generations_check;
ALTER TABLE job_generations
    ADD CONSTRAINT job_generations_authoritative_shape CHECK (
        (generation = 1 AND purpose = 'initial' AND
            predecessor_generation IS NULL AND boundary_action IS NULL AND
            feedback IS NULL AND feedback_sha256 IS NULL) OR
        (generation > 1 AND purpose = 'replan' AND
            predecessor_generation = generation - 1 AND
            boundary_action IN ('v3_coding', 'objective_resolve', 'v3_planning') AND
            task_ledger_text_is_exact(feedback) AND
            octet_length(feedback) <= 65536 AND
            feedback_sha256 ~ '^[0-9a-f]{64}$' AND
            feedback_sha256 = encode(digest(feedback, 'sha256'), 'hex'))
    );

CREATE FUNCTION require_current_job_generation_boundary()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.purpose='replan' AND
       NEW.boundary_action NOT IN ('v3_coding', 'objective_resolve') THEN
        RAISE EXCEPTION 'new job generation boundary % is retired', NEW.boundary_action;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER job_generations_require_current_boundary
BEFORE INSERT ON job_generations
FOR EACH ROW EXECUTE FUNCTION require_current_job_generation_boundary();

DO $$
DECLARE
    remaining INTEGER;
BEGIN
    SELECT COUNT(*) INTO remaining
    FROM information_schema.columns
    WHERE table_schema=current_schema()
      AND table_name='ai_channels'
      AND column_name IN ('persona','system','provider','model','context');
    IF remaining <> 0 OR
       to_regclass(current_schema() || '.task_artifact_projections') IS NOT NULL OR
       to_regclass(current_schema() || '.task_artifact_projection_items') IS NOT NULL OR
       to_regprocedure(current_schema() || '.validate_task_artifact_projection()') IS NOT NULL OR
       to_regprocedure(current_schema() || '.validate_task_artifact_projection_item()') IS NOT NULL OR
       to_regprocedure(current_schema() || '.prevent_task_artifact_projection_mutation()') IS NOT NULL OR
       to_regprocedure(current_schema() || '.prevent_projected_artifact_mutation()') IS NOT NULL OR
       to_regprocedure(current_schema() || '.require_intent_artifact_projection()') IS NOT NULL OR
       to_regclass(current_schema() || '.idx_artifacts_id_job_step') IS NOT NULL THEN
        RAISE EXCEPTION 'conversation objective cutover postcondition failed: retired authority remains';
    END IF;

    SELECT COUNT(*) INTO remaining
    FROM pg_trigger AS trigger
    JOIN pg_class AS relation ON relation.oid=trigger.tgrelid
    JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    WHERE namespace.nspname=current_schema()
      AND relation.relname='artifacts'
      AND trigger.tgname IN ('projected_artifacts_immutable', 'intent_artifact_requires_projection')
      AND NOT trigger.tgisinternal;
    IF remaining <> 0 THEN
        RAISE EXCEPTION 'conversation objective cutover postcondition failed: retired artifact triggers remain';
    END IF;

    SELECT COUNT(*) INTO remaining
    FROM pg_constraint AS con
    WHERE con.conrelid='job_generations'::regclass
      AND con.conname='job_generations_authoritative_shape'
      AND con.contype='c'
      AND con.convalidated
      AND NOT con.connoinherit
      AND encode(digest(pg_get_constraintdef(con.oid, true), 'sha256'), 'hex')=
          '6d35378110ee10f551a3db1f9384099ddcae7bbf2e15763262bafcb437e493b3';
    IF remaining <> 1 OR EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='job_generations'::regclass
          AND conname='job_generations_check'
    ) THEN
        RAISE EXCEPTION 'conversation objective cutover postcondition failed: generation constraint differs';
    END IF;

    SELECT COUNT(*) INTO remaining
    FROM pg_trigger AS trigger
    JOIN pg_class AS relation ON relation.oid=trigger.tgrelid
    JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    JOIN pg_proc AS procedure ON procedure.oid=trigger.tgfoid
    WHERE namespace.nspname=current_schema()
      AND relation.relname='job_generations'
      AND trigger.tgname='job_generations_require_current_boundary'
      AND procedure.proname='require_current_job_generation_boundary'
      AND procedure.prokind='f'
      AND procedure.pronargs=0
      AND procedure.prorettype='trigger'::regtype
      AND encode(digest(procedure.prosrc, 'sha256'), 'hex')=
          'b8eecfca02b64a0a72f493c64e93608a67adadaaaff3fc90a6dcf55ea3e02ed3'
      AND trigger.tgtype=7
      AND trigger.tgenabled='O'
      AND trigger.tgconstraint=0
      AND NOT trigger.tgdeferrable
      AND NOT trigger.tginitdeferred
      AND NOT trigger.tgisinternal;
    IF remaining <> 1 THEN
        RAISE EXCEPTION 'conversation objective cutover postcondition failed: current generation boundary guard is absent';
    END IF;
END $$;
