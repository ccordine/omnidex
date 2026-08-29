BEGIN;

LOCK TABLE jobs, job_steps, workspace_mutation_operations IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    jobs_source TEXT;
    steps_source TEXT;
    jobs_trigger_count INTEGER;
    steps_trigger_count INTEGER;
    invalid_operation_id TEXT;
BEGIN
    SELECT procedure.prosrc INTO jobs_source
    FROM pg_proc AS procedure
    JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND procedure.oid=to_regprocedure(
          current_schema() || '.enforce_jobs_executable_pipeline_authority()'
      )
      AND procedure.prokind='f' AND procedure.pronargs=0
      AND procedure.pronargdefaults=0 AND procedure.prorettype='trigger'::regtype
      AND NOT procedure.proretset AND language.lanname='plpgsql'
      AND procedure.provolatile='v' AND procedure.proparallel='u'
      AND NOT procedure.proisstrict AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof AND procedure.proconfig IS NULL;
    SELECT COUNT(*) INTO jobs_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='jobs'::regclass
      AND (
          (trigger_row.tgname='jobs_executable_pipeline_authority' AND
           trigger_row.tgtype=31) OR
          (trigger_row.tgname='jobs_history_truncate_immutable' AND
           trigger_row.tgtype=34)
      )
      AND trigger_row.tgenabled='O' AND trigger_row.tgattr::TEXT=''
      AND trigger_row.tgqual IS NULL AND trigger_row.tgconstraint=0
      AND trigger_row.tgconstrrelid=0 AND trigger_row.tgconstrindid=0
      AND NOT trigger_row.tgdeferrable AND NOT trigger_row.tginitdeferred
      AND trigger_row.tgnargs=0 AND octet_length(trigger_row.tgargs)=0
      AND trigger_row.tgoldtable IS NULL AND trigger_row.tgnewtable IS NULL
      AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.enforce_jobs_executable_pipeline_authority()'
      );
    IF jobs_trigger_count<>2 OR jobs_source IS NULL OR
       encode(digest(convert_to(jobs_source,'UTF8'),'sha256'),'hex')<>
       '195894f09717e1f965ca49236384c539d8fc4cf1661c1226b198f6946f108e15' THEN
        RAISE EXCEPTION 'job execution identity requires the exact prior pipeline guard';
    END IF;

    SELECT procedure.prosrc INTO steps_source
    FROM pg_proc AS procedure
    JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND procedure.oid=to_regprocedure(
          current_schema() || '.prevent_job_step_generation_identity_mutation()'
      )
      AND procedure.prokind='f' AND procedure.pronargs=0
      AND procedure.pronargdefaults=0 AND procedure.prorettype='trigger'::regtype
      AND NOT procedure.proretset AND language.lanname='plpgsql'
      AND procedure.provolatile='v' AND procedure.proparallel='u'
      AND NOT procedure.proisstrict AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof AND procedure.proconfig IS NULL;
    SELECT COUNT(*) INTO steps_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='job_steps'::regclass
      AND trigger_row.tgname='job_steps_generation_identity_immutable'
      AND trigger_row.tgenabled='O' AND trigger_row.tgtype=19
      AND trigger_row.tgattr::TEXT='' AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.prevent_job_step_generation_identity_mutation()'
      );
    IF steps_trigger_count<>1 OR steps_source IS NULL OR
       encode(digest(convert_to(steps_source,'UTF8'),'sha256'),'hex')<>
       'be196b60c8e6f01100ab1b01af5dc040398d3b0ceb90df2d6355bc65b714b14e' THEN
        RAISE EXCEPTION 'job execution identity requires the exact prior step guard';
    END IF;

    SELECT operation.id INTO invalid_operation_id
    FROM workspace_mutation_operations AS operation
    JOIN jobs ON jobs.id=operation.job_id
    JOIN job_steps AS steps
      ON steps.job_id=operation.job_id AND steps.id=operation.step_id
    WHERE NOT (
        (jobs.pipeline='chat' AND steps.action='objective_resolve') OR
        (jobs.pipeline='coding' AND steps.action='v3_coding') OR
        (jobs.pipeline='scrum' AND steps.action='v3_coding')
    )
    ORDER BY operation.id
    LIMIT 1;
    IF invalid_operation_id IS NOT NULL THEN
        RAISE EXCEPTION
            'job execution identity found stale workspace mutation %',invalid_operation_id;
    END IF;
END $$;

CREATE OR REPLACE FUNCTION enforce_jobs_executable_pipeline_authority()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'job history is immutable';
    END IF;
    IF TG_OP='DELETE' THEN
        IF OLD.pipeline NOT IN ('chat','coding','scrum') THEN
            RAISE EXCEPTION 'historical retired job is immutable';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'new job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    IF TG_OP='UPDATE' AND OLD.pipeline IS DISTINCT FROM NEW.pipeline THEN
        RAISE EXCEPTION 'job pipeline identity is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline NOT IN ('chat','coding','scrum')
       AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'historical retired job is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline IN ('chat','coding','scrum')
       AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'current job pipeline cannot become retired or unregistered';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.status IN ('completed','failed','canceled')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'terminal job cannot become nonterminal';
    END IF;
    IF NEW.pipeline NOT IN ('chat','coding','scrum')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'nonterminal job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.job_id IS DISTINCT FROM NEW.job_id OR
       OLD.generation IS DISTINCT FROM NEW.generation THEN
        RAISE EXCEPTION 'job step generation identity is immutable';
    END IF;
    IF OLD.action IS DISTINCT FROM NEW.action THEN
        RAISE EXCEPTION 'job step action identity is immutable';
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

DO $$
DECLARE
    jobs_digest TEXT;
    steps_digest TEXT;
BEGIN
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')
      INTO jobs_digest
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema() || '.enforce_jobs_executable_pipeline_authority()'
    );
    SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')
      INTO steps_digest
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema() || '.prevent_job_step_generation_identity_mutation()'
    );
    IF jobs_digest IS DISTINCT FROM
       'ad046f5dd4f3059740425b6c2af74ee061cf1715d78ac31d5d0a19279fc7844e' OR
       steps_digest IS DISTINCT FROM
       '04423dc23b8cbeb03a0cbe0d28e1e4f805a86d61a2c1a613d3275bdf71a21bad' THEN
        RAISE EXCEPTION 'job execution identity immutability postcondition failed';
    END IF;
END $$;

COMMIT;
