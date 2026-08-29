BEGIN;

LOCK TABLE projects, jobs, job_steps, job_step_attempts,
    workspace_mutation_operations IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    insert_source TEXT;
    current_source TEXT;
    update_source TEXT;
    insert_trigger_count INTEGER;
    update_trigger_count INTEGER;
    invalid_operation_id TEXT;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema=current_schema()
          AND table_name='workspace_mutation_operations'
          AND column_name='project_location'
    ) THEN
        RAISE EXCEPTION 'workspace mutation project-location authority already exists';
    END IF;

    SELECT procedure.prosrc INTO insert_source
    FROM pg_proc AS procedure
    JOIN pg_namespace AS namespace ON namespace.oid=procedure.pronamespace
    JOIN pg_language AS language ON language.oid=procedure.prolang
    WHERE namespace.nspname=current_schema()
      AND procedure.oid=to_regprocedure(
          current_schema() || '.validate_workspace_mutation_insert()'
      )
      AND procedure.prokind='f' AND procedure.pronargs=0
      AND procedure.pronargdefaults=0 AND procedure.prorettype='trigger'::regtype
      AND NOT procedure.proretset AND language.lanname='plpgsql'
      AND procedure.provolatile='v' AND procedure.proparallel='u'
      AND NOT procedure.proisstrict AND NOT procedure.prosecdef
      AND NOT procedure.proleakproof AND procedure.proconfig IS NULL;
    SELECT COUNT(*) INTO insert_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='workspace_mutation_operations'::regclass
      AND trigger_row.tgname='workspace_mutation_insert_validate'
      AND trigger_row.tgenabled='O' AND trigger_row.tgtype=7
      AND trigger_row.tgattr::TEXT='' AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.validate_workspace_mutation_insert()'
      );
    IF insert_trigger_count<>1 OR insert_source IS NULL OR
       encode(digest(convert_to(insert_source,'UTF8'),'sha256'),'hex')<>
       '39c9598b0ecaad64aebd36391e71ba18406a23d75dc0c33606654789b2c15d2b' THEN
        RAISE EXCEPTION 'workspace mutation project-location authority requires the exact prior insert guard';
    END IF;

    SELECT procedure.prosrc INTO current_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema() ||
        '.workspace_mutation_current_authority_valid(workspace_mutation_operations)'
    );
    SELECT procedure.prosrc INTO update_source
    FROM pg_proc AS procedure
    WHERE procedure.oid=to_regprocedure(
        current_schema() || '.validate_workspace_mutation_update()'
    );
    SELECT COUNT(*) INTO update_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='workspace_mutation_operations'::regclass
      AND trigger_row.tgname='workspace_mutation_update_validate'
      AND trigger_row.tgenabled='O' AND trigger_row.tgtype=19
      AND trigger_row.tgattr::TEXT='' AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.validate_workspace_mutation_update()'
      );
    IF current_source IS NULL OR
       encode(digest(convert_to(current_source,'UTF8'),'sha256'),'hex')<>
       '8312f18e7a7d2265bcf38f0103bfcd0d02fc8845a6edb197a7211830422d7a79' OR
       update_trigger_count<>1 OR update_source IS NULL OR
       encode(digest(convert_to(update_source,'UTF8'),'sha256'),'hex')<>
       '3f2b6f2d24f2ce88260f46d7cc2c27377d4c8d2150d78dc22751569608b92d5b' THEN
        RAISE EXCEPTION 'workspace mutation project-location authority requires exact prior transition guards';
    END IF;

    SELECT operation.id INTO invalid_operation_id
    FROM workspace_mutation_operations AS operation
    LEFT JOIN jobs ON jobs.id=operation.job_id
    LEFT JOIN projects ON projects.id=operation.project_id
    WHERE jobs.id IS NULL OR projects.id IS NULL OR
       jobs.project_id IS DISTINCT FROM operation.project_id
       OR (
           operation.status NOT IN ('verified','verification_failed') AND
           projects.location IS DISTINCT FROM operation.workspace_root
       )
    ORDER BY operation.id
    LIMIT 1;
    IF invalid_operation_id IS NOT NULL THEN
        RAISE EXCEPTION
            'workspace mutation project-location authority found stale operation %',
            invalid_operation_id;
    END IF;
END $$;

ALTER TABLE workspace_mutation_operations
    ADD COLUMN project_location TEXT;

ALTER TABLE workspace_mutation_operations
    DISABLE TRIGGER workspace_mutation_update_validate;

UPDATE workspace_mutation_operations AS operation
SET project_location=operation.workspace_root;

ALTER TABLE workspace_mutation_operations
    ENABLE TRIGGER workspace_mutation_update_validate;

ALTER TABLE workspace_mutation_operations
    ALTER COLUMN project_location SET NOT NULL,
    ADD CONSTRAINT workspace_mutation_project_location_valid CHECK (
        project_location<>'' AND project_location=BTRIM(project_location) AND
        octet_length(project_location)<=4096 AND left(project_location,1)='/' AND
        project_location<>'/' AND right(project_location,1)<>'/' AND
        position(chr(92) IN project_location)=0 AND
        project_location !~ E'[\r\n]' AND position('//' IN project_location)=0 AND
        project_location !~ '(^|/)[.][.]?(/|$)'
    );

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
           projects.location=NEW.project_location AND
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
        RAISE EXCEPTION 'workspace mutation requires the exact current active step attempt and project location';
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
               jobs.project_id=operation.project_id AND
               projects.location=operation.project_location AND (
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
        JOIN projects ON projects.id=jobs.project_id
        JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=operation.step_id
        JOIN job_step_attempts AS attempts
          ON attempts.job_id=operation.job_id AND attempts.generation=operation.generation AND
             attempts.step_id=operation.step_id AND attempts.attempt=operation.current_step_attempt
        WHERE jobs.id=operation.job_id
    ),FALSE);
$$ LANGUAGE SQL;

CREATE FUNCTION prevent_workspace_mutation_project_location_change()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.project_location IS DISTINCT FROM NEW.project_location THEN
        RAISE EXCEPTION 'workspace mutation project location is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_mutation_project_location_immutable
BEFORE UPDATE ON workspace_mutation_operations
FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_project_location_change();

CREATE FUNCTION prevent_project_location_change_during_active_work()
RETURNS TRIGGER AS $$
DECLARE
    active_job_id BIGINT;
    active_job_status TEXT;
BEGIN
    IF OLD.location IS NOT DISTINCT FROM NEW.location THEN
        RETURN NEW;
    END IF;
    SELECT id,status INTO active_job_id,active_job_status
    FROM jobs
    WHERE project_id=OLD.id AND status NOT IN ('completed','failed','canceled')
    ORDER BY id
    LIMIT 1;
    IF active_job_id IS NOT NULL THEN
        RAISE EXCEPTION 'project location cannot change while job % remains %',
            active_job_id,active_job_status;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER projects_active_work_location_guard
BEFORE UPDATE ON projects
FOR EACH ROW EXECUTE FUNCTION prevent_project_location_change_during_active_work();

DO $$
DECLARE
    insert_digest TEXT;
    current_digest TEXT;
    immutable_digest TEXT;
    immutable_trigger_count INTEGER;
    project_guard_digest TEXT;
    project_guard_trigger_count INTEGER;
    invalid_operation_id TEXT;
BEGIN
    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
      INTO insert_digest
    FROM pg_proc
    WHERE oid=to_regprocedure(current_schema() || '.validate_workspace_mutation_insert()');
    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
      INTO current_digest
    FROM pg_proc
    WHERE oid=to_regprocedure(
        current_schema() ||
        '.workspace_mutation_current_authority_valid(workspace_mutation_operations)'
    );
    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
      INTO immutable_digest
    FROM pg_proc
    WHERE oid=to_regprocedure(
        current_schema() || '.prevent_workspace_mutation_project_location_change()'
    );
    SELECT COUNT(*) INTO immutable_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='workspace_mutation_operations'::regclass
      AND trigger_row.tgname='workspace_mutation_project_location_immutable'
      AND trigger_row.tgenabled='O' AND trigger_row.tgtype=19
      AND trigger_row.tgattr::TEXT='' AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.prevent_workspace_mutation_project_location_change()'
      );
    SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
      INTO project_guard_digest
    FROM pg_proc
    WHERE oid=to_regprocedure(
        current_schema() || '.prevent_project_location_change_during_active_work()'
    );
    SELECT COUNT(*) INTO project_guard_trigger_count
    FROM pg_trigger AS trigger_row
    WHERE trigger_row.tgrelid='projects'::regclass
      AND trigger_row.tgname='projects_active_work_location_guard'
      AND trigger_row.tgenabled='O' AND trigger_row.tgtype=19
      AND trigger_row.tgattr::TEXT='' AND trigger_row.tgqual IS NULL
      AND trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0
      AND trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable
      AND NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0
      AND octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL
      AND trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal
      AND trigger_row.tgfoid=to_regprocedure(
          current_schema() || '.prevent_project_location_change_during_active_work()'
      );
    SELECT operation.id INTO invalid_operation_id
    FROM workspace_mutation_operations AS operation
    LEFT JOIN jobs ON jobs.id=operation.job_id
    LEFT JOIN projects ON projects.id=operation.project_id
    WHERE jobs.id IS NULL OR projects.id IS NULL OR
       jobs.project_id IS DISTINCT FROM operation.project_id
       OR (
           operation.status NOT IN ('verified','verification_failed') AND
           projects.location IS DISTINCT FROM operation.project_location
       )
    ORDER BY operation.id
    LIMIT 1;
    IF insert_digest IS DISTINCT FROM
       'e76c9d7ad78aad3ca57375b9c914f100be8ce16395db7624111f6982a39f3352' OR
       current_digest IS DISTINCT FROM
       'f53f54994b7c4bd61d40831af93607b6f2053bc923af58672f548d22bd997941' OR
       immutable_digest IS DISTINCT FROM
       'fdcea1012ffbb9b150e31f9f01ef49819e07ec2996c7d330de002b993d9c7e8d' OR
       immutable_trigger_count<>1 OR
       project_guard_digest IS DISTINCT FROM
       '07ab38f846278a6f1385127428e02244ec7e898e2e6a2609de719ef89b793a25' OR
       project_guard_trigger_count<>1 OR invalid_operation_id IS NOT NULL THEN
        RAISE EXCEPTION
            'workspace mutation project-location authority postcondition failed (insert=%, current=%, immutable=%/%, project_guard=%/%, invalid=%)',
            insert_digest,current_digest,immutable_digest,immutable_trigger_count,
            project_guard_digest,project_guard_trigger_count,invalid_operation_id;
    END IF;
END $$;

COMMIT;
