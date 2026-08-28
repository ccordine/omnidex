BEGIN;

LOCK TABLE jobs, job_steps, workspace_mutation_operations IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    authority_count INTEGER;
    authority_source TEXT;
    current_authority_count INTEGER;
    current_authority_source TEXT;
    invalid_operation_id TEXT;
BEGIN
    SELECT COUNT(*),MIN(procedure.prosrc)
      INTO authority_count,authority_source
    FROM pg_trigger AS trigger_row
    JOIN pg_class AS relation ON relation.oid=trigger_row.tgrelid
    JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND relation.relname='workspace_mutation_operations'
      AND relation.relkind='r'
      AND trigger_row.tgname='workspace_mutation_insert_validate'
      AND trigger_row.tgenabled='O'
      AND trigger_row.tgtype=7
      AND trigger_row.tgattr::TEXT=''
      AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0
      AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0
      AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred
      AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0
      AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL
      AND NOT trigger_row.tgisinternal
      AND procedure.pronamespace=namespace.oid
      AND procedure.proname='validate_workspace_mutation_insert'
      AND procedure.prokind='f'
      AND procedure.pronargs=0
      AND procedure.pronargdefaults=0
      AND procedure.prorettype='trigger'::regtype
      AND NOT procedure.proretset
      AND language.lanname='plpgsql'
      AND procedure.provolatile='v'
      AND procedure.proparallel='u'
      AND NOT procedure.proisstrict
      AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof
      AND procedure.proconfig IS NULL
      AND (
          SELECT COUNT(*)
          FROM pg_trigger AS function_trigger
          WHERE function_trigger.tgfoid=procedure.oid
            AND NOT function_trigger.tgisinternal
      )=1;
    IF authority_count<>1 OR authority_source IS NULL OR
       encode(digest(convert_to(authority_source,'UTF8'),'sha256'),'hex')<>
       '9f2b7387e90d9021089532021ab55c09d7177b81a3fd7bdd2358e362e2930294' THEN
        RAISE EXCEPTION
            'workspace mutation pipeline/action authority requires the exact prior insert guard';
    END IF;

    SELECT COUNT(*),MIN(procedure.prosrc)
      INTO current_authority_count,current_authority_source
    FROM pg_proc AS procedure
    JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND procedure.oid=to_regprocedure(
          current_schema() ||
          '.workspace_mutation_current_authority_valid(workspace_mutation_operations)'
      )
      AND procedure.prokind='f'
      AND procedure.pronargs=1
      AND procedure.pronargdefaults=0
      AND procedure.prorettype='boolean'::regtype
      AND NOT procedure.proretset
      AND language.lanname='sql'
      AND procedure.provolatile='v'
      AND procedure.proparallel='u'
      AND NOT procedure.proisstrict
      AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof
      AND procedure.proconfig IS NULL;
    IF current_authority_count<>1 OR current_authority_source IS NULL OR
       encode(digest(convert_to(current_authority_source,'UTF8'),'sha256'),'hex')<>
       'fc8209a7ce7b843458f1e3f2104a5de50ab8167fa104c4a106b6f6c4a8a67c53' THEN
        RAISE EXCEPTION
            'workspace mutation pipeline/action authority requires the exact prior transition guard';
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
            'workspace mutation % has stale pipeline/action authority',invalid_operation_id;
    END IF;
END $$;

CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()
RETURNS TRIGGER AS $$
DECLARE
    authority_valid BOOLEAN;
    source_binding_valid BOOLEAN;
    plan JSONB;
BEGIN
    IF NEW.status<>'prepared' OR NEW.sealed_at IS NOT NULL OR
       NEW.apply_attempt_count<>0 OR NEW.verification_attempt_count<>0 OR
       ROW(NEW.last_error,NEW.mutation_evidence_id,NEW.verification_succeeded,
           NEW.verification_receipt_json,NEW.verification_receipt_sha256,
           NEW.verification_evidence_id,NEW.applying_at,NEW.applied_at,
           NEW.verifying_at,NEW.terminal_at,NEW.verified_repository_snapshot_id)
       IS DISTINCT FROM ROW(NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL) OR
       ROW(NEW.creator_step_attempt,NEW.creator_worker_id) IS DISTINCT FROM
       ROW(NEW.current_step_attempt,NEW.current_worker_id) THEN
        RAISE EXCEPTION 'workspace mutation insert must be one unattempted prepared command';
    END IF;
    SELECT jobs.status='running' AND (
               (jobs.pipeline='chat' AND steps.action='objective_resolve') OR
               (jobs.pipeline='coding' AND steps.action='v3_coding') OR
               (jobs.pipeline='scrum' AND steps.action='v3_coding')
           ) AND
           jobs.project_id=NEW.project_id AND jobs.current_generation=NEW.generation AND
           projects.location=NEW.workspace_root AND
           steps.status='running' AND steps.generation=NEW.generation AND
           steps.current_attempt=NEW.current_step_attempt AND
           steps.worker_id=NEW.current_worker_id AND steps.superseded_at_generation IS NULL AND
           attempts.status='active' AND attempts.worker_id=NEW.current_worker_id AND
           attempts.expires_at>clock_timestamp()
      INTO authority_valid
    FROM jobs
    JOIN projects ON projects.id=jobs.project_id
    JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
    JOIN job_step_attempts AS attempts
      ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
         attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt
    WHERE jobs.id=NEW.job_id;
    IF authority_valid IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'workspace mutation requires the exact current active step attempt and root';
    END IF;
    IF NEW.source_repository_snapshot_id IS NOT NULL THEN
        SELECT snapshot.project_id=NEW.project_id AND snapshot.root=NEW.workspace_root
          INTO source_binding_valid
        FROM repository_snapshots AS snapshot
        WHERE snapshot.id=NEW.source_repository_snapshot_id;
        IF source_binding_valid IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'workspace mutation optional Git source binding differs from workspace authority';
        END IF;
    END IF;
    plan := NEW.verification_plan_json::JSONB;
    IF NOT workspace_mutation_exact_keys(plan,ARRAY['commands','schema']) OR
       plan->>'schema'<>'omnidex.workspace-mutation-verification-plan.v1' OR
       jsonb_typeof(plan->'commands')<>'array' OR
       jsonb_array_length(plan->'commands') NOT BETWEEN 1 AND 32 OR
       EXISTS (
           SELECT 1
           FROM jsonb_array_elements(plan->'commands') WITH ORDINALITY AS item(command,ordinal)
           WHERE NOT workspace_mutation_exact_keys(
                     command,ARRAY['command','command_sha256','kind','ordinal']
                 ) OR
                 command->>'kind' NOT IN ('command_output','test_result') OR
                 jsonb_typeof(command->'command')<>'string' OR
                 command->>'command'<>btrim(command->>'command') OR
                 octet_length(command->>'command') NOT BETWEEN 1 AND 16384 OR
                 command->>'command_sha256' !~ '^[0-9a-f]{64}$' OR
                 command->>'command_sha256'<>encode(
                     digest(convert_to(command->>'command','UTF8'),'sha256'),'hex'
                 ) OR
                 jsonb_typeof(command->'ordinal')<>'number' OR
                 command->>'ordinal'<>ordinal::TEXT
       ) THEN
        RAISE EXCEPTION 'workspace mutation verification plan is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION workspace_mutation_current_authority_valid(
    operation workspace_mutation_operations
)
RETURNS BOOLEAN AS $$
    SELECT COALESCE((
        SELECT jobs.status='running' AND jobs.current_generation=operation.generation AND
               jobs.project_id=operation.project_id AND (
                   (jobs.pipeline='chat' AND steps.action='objective_resolve') OR
                   (jobs.pipeline='coding' AND steps.action='v3_coding') OR
                   (jobs.pipeline='scrum' AND steps.action='v3_coding')
               ) AND
               steps.status='running' AND steps.generation=operation.generation AND
               steps.current_attempt=operation.current_step_attempt AND
               steps.worker_id=operation.current_worker_id AND
               steps.superseded_at_generation IS NULL AND attempts.status='active' AND
               attempts.worker_id=operation.current_worker_id AND
               attempts.expires_at>clock_timestamp()
        FROM jobs
        JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=operation.step_id
        JOIN job_step_attempts AS attempts
          ON attempts.job_id=operation.job_id AND attempts.generation=operation.generation AND
             attempts.step_id=operation.step_id AND attempts.attempt=operation.current_step_attempt
        WHERE jobs.id=operation.job_id
    ),FALSE);
$$ LANGUAGE SQL;

DO $$
DECLARE
    authority_count INTEGER;
    authority_source TEXT;
    current_authority_count INTEGER;
    current_authority_source TEXT;
BEGIN
    SELECT COUNT(*),MIN(procedure.prosrc)
      INTO authority_count,authority_source
    FROM pg_trigger AS trigger_row
    JOIN pg_class AS relation ON relation.oid=trigger_row.tgrelid
    JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND relation.relname='workspace_mutation_operations'
      AND relation.relkind='r'
      AND trigger_row.tgname='workspace_mutation_insert_validate'
      AND trigger_row.tgenabled='O'
      AND trigger_row.tgtype=7
      AND trigger_row.tgattr::TEXT=''
      AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0
      AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0
      AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred
      AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0
      AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL
      AND NOT trigger_row.tgisinternal
      AND procedure.pronamespace=namespace.oid
      AND procedure.proname='validate_workspace_mutation_insert'
      AND procedure.prokind='f'
      AND procedure.pronargs=0
      AND procedure.pronargdefaults=0
      AND procedure.prorettype='trigger'::regtype
      AND NOT procedure.proretset
      AND language.lanname='plpgsql'
      AND procedure.provolatile='v'
      AND procedure.proparallel='u'
      AND NOT procedure.proisstrict
      AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof
      AND procedure.proconfig IS NULL
      AND (
          SELECT COUNT(*)
          FROM pg_trigger AS function_trigger
          WHERE function_trigger.tgfoid=procedure.oid
            AND NOT function_trigger.tgisinternal
      )=1;
    IF authority_count<>1 OR authority_source IS NULL OR
       encode(digest(convert_to(authority_source,'UTF8'),'sha256'),'hex')<>
       '39c9598b0ecaad64aebd36391e71ba18406a23d75dc0c33606654789b2c15d2b' THEN
        RAISE EXCEPTION
            'workspace mutation pipeline/action authority postcondition failed';
    END IF;

    SELECT COUNT(*),MIN(procedure.prosrc)
      INTO current_authority_count,current_authority_source
    FROM pg_proc AS procedure
    JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND procedure.oid=to_regprocedure(
          current_schema() ||
          '.workspace_mutation_current_authority_valid(workspace_mutation_operations)'
      )
      AND procedure.prokind='f'
      AND procedure.pronargs=1
      AND procedure.pronargdefaults=0
      AND procedure.prorettype='boolean'::regtype
      AND NOT procedure.proretset
      AND language.lanname='sql'
      AND procedure.provolatile='v'
      AND procedure.proparallel='u'
      AND NOT procedure.proisstrict
      AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof
      AND procedure.proconfig IS NULL;
    IF current_authority_count<>1 OR current_authority_source IS NULL OR
       encode(digest(convert_to(current_authority_source,'UTF8'),'sha256'),'hex')<>
       '8312f18e7a7d2265bcf38f0103bfcd0d02fc8845a6edb197a7211830422d7a79' THEN
        RAISE EXCEPTION
            'workspace mutation transition pipeline/action authority postcondition failed';
    END IF;
END $$;

COMMIT;
