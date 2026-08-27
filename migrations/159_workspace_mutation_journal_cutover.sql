BEGIN;

LOCK TABLE repository_mutation_operations, repository_mutation_files,
    evidence, jobs, job_steps, job_step_attempts, projects, repository_snapshots
    IN ACCESS EXCLUSIVE MODE;

DO $preflight$
DECLARE
    unresolved RECORD;
BEGIN
    SELECT id,job_id,generation,status INTO unresolved
    FROM repository_mutation_operations
    WHERE status <> 'applied'
    ORDER BY job_id,generation,id
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION
            'workspace mutation cutover rejects unresolved legacy repository mutation % for job % generation % in state %',
            unresolved.id,unresolved.job_id,unresolved.generation,unresolved.status;
    END IF;
END $preflight$;

DROP TRIGGER repository_mutation_source_validate ON repository_mutation_operations;
DROP TRIGGER repository_mutation_requires_files ON repository_mutation_operations;
DROP TRIGGER repository_mutation_identity_immutable ON repository_mutation_operations;
DROP TRIGGER repository_mutation_operations_delete_immutable ON repository_mutation_operations;
DROP TRIGGER repository_mutation_operations_truncate_immutable ON repository_mutation_operations;
DROP TRIGGER repository_mutation_file_source_validate ON repository_mutation_files;
DROP TRIGGER repository_mutation_files_immutable ON repository_mutation_files;
DROP TRIGGER repository_mutation_files_truncate_immutable ON repository_mutation_files;
DROP TRIGGER repository_mutation_evidence_immutable ON evidence;
DROP INDEX idx_repository_mutations_unresolved_generation;

DROP FUNCTION validate_repository_mutation_source();
DROP FUNCTION validate_repository_mutation_file_source();
DROP FUNCTION require_repository_mutation_files();
DROP FUNCTION prevent_repository_mutation_identity_change();
DROP FUNCTION prevent_repository_mutation_file_change();
DROP FUNCTION prevent_repository_mutation_delete();
DROP FUNCTION prevent_repository_mutation_evidence_change();

ALTER TABLE repository_mutation_operations RENAME TO retired_repository_mutation_operations;
ALTER TABLE repository_mutation_files RENAME TO retired_repository_mutation_files;

CREATE FUNCTION prevent_retired_repository_mutation_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'retired repository mutation journal is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER retired_repository_mutation_operations_immutable
BEFORE INSERT OR UPDATE OR DELETE ON retired_repository_mutation_operations
FOR EACH ROW EXECUTE FUNCTION prevent_retired_repository_mutation_change();
CREATE TRIGGER retired_repository_mutation_operations_truncate_immutable
BEFORE TRUNCATE ON retired_repository_mutation_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_retired_repository_mutation_change();
CREATE TRIGGER retired_repository_mutation_files_immutable
BEFORE INSERT OR UPDATE OR DELETE ON retired_repository_mutation_files
FOR EACH ROW EXECUTE FUNCTION prevent_retired_repository_mutation_change();
CREATE TRIGGER retired_repository_mutation_files_truncate_immutable
BEFORE TRUNCATE ON retired_repository_mutation_files
FOR EACH STATEMENT EXECUTE FUNCTION prevent_retired_repository_mutation_change();

CREATE TABLE workspace_mutation_operations (
    id TEXT PRIMARY KEY CHECK (id ~ '^workspace_mutation_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    creator_step_attempt BIGINT NOT NULL CHECK (creator_step_attempt>0),
    creator_worker_id TEXT NOT NULL CHECK (
        creator_worker_id<>'' AND creator_worker_id=BTRIM(creator_worker_id) AND
        octet_length(creator_worker_id)<=256
    ),
    current_step_attempt BIGINT NOT NULL CHECK (current_step_attempt>0),
    current_worker_id TEXT NOT NULL CHECK (
        current_worker_id<>'' AND current_worker_id=BTRIM(current_worker_id) AND
        octet_length(current_worker_id)<=256
    ),
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    owner_id TEXT NOT NULL CHECK (
        owner_id ~ '^[a-z][a-z0-9_]{0,63}_[0-9a-f]{64}$'
    ),
    stage_id TEXT NOT NULL CHECK (
        stage_id ~ '^[a-z][a-z0-9_]{0,63}_[0-9a-f]{64}$'
    ),
    workspace_id TEXT NOT NULL CHECK (workspace_id ~ '^workspace_[0-9a-f]{64}$'),
    workspace_root TEXT NOT NULL CHECK (
        workspace_root<>'' AND workspace_root=BTRIM(workspace_root) AND
        octet_length(workspace_root)<=4096 AND left(workspace_root,1)='/' AND
        workspace_root<>'/' AND right(workspace_root,1)<>'/' AND
        position(chr(92) IN workspace_root)=0 AND
        workspace_root !~ E'[\r\n]' AND position('//' IN workspace_root)=0 AND
        workspace_root !~ '(^|/)[.][.]?(/|$)'
    ),
    source_state_id TEXT NOT NULL CHECK (source_state_id ~ '^workspace_state_[0-9a-f]{64}$'),
    expected_state_id TEXT NOT NULL CHECK (expected_state_id ~ '^workspace_state_[0-9a-f]{64}$'),
    source_repository_snapshot_id TEXT,
    verified_repository_snapshot_id TEXT,
    patch TEXT NOT NULL CHECK (octet_length(patch) BETWEEN 1 AND 33554432),
    patch_sha256 TEXT NOT NULL CHECK (patch_sha256 ~ '^[0-9a-f]{64}$'),
    verification_plan_json TEXT NOT NULL CHECK (
        octet_length(verification_plan_json) BETWEEN 2 AND 262144
    ),
    verification_plan_sha256 TEXT NOT NULL CHECK (
        verification_plan_sha256 ~ '^[0-9a-f]{64}$'
    ),
    status TEXT NOT NULL CHECK (status IN (
        'prepared','applying','applied','verifying','verified','verification_failed','indeterminate'
    )),
    indeterminate_phase TEXT CHECK (indeterminate_phase IN ('apply','verification')),
    apply_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (apply_attempt_count>=0),
    verification_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (verification_attempt_count>=0),
    last_error TEXT CHECK (
        last_error IS NULL OR (
            last_error<>'' AND last_error=BTRIM(last_error) AND
            octet_length(last_error)<=65536
        )
    ),
    mutation_evidence_id BIGINT,
    verification_succeeded BOOLEAN,
    verification_receipt_json TEXT CHECK (
        verification_receipt_json IS NULL OR
        octet_length(verification_receipt_json) BETWEEN 2 AND 262144
    ),
    verification_receipt_sha256 TEXT CHECK (
        verification_receipt_sha256 IS NULL OR
        verification_receipt_sha256 ~ '^[0-9a-f]{64}$'
    ),
    verification_evidence_id BIGINT,
    prepared_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    sealed_at TIMESTAMPTZ,
    applying_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    verifying_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    terminal_at TIMESTAMPTZ,
    CHECK (id='workspace_mutation_' || command_sha256),
    CHECK (workspace_id='workspace_' || encode(digest(
        convert_to(workspace_root,'UTF8') || decode('00','hex'),'sha256'
    ),'hex')),
    CHECK (source_state_id<>expected_state_id),
    CHECK (patch_sha256=encode(digest(convert_to(patch,'UTF8'),'sha256'),'hex')),
    CHECK (verification_plan_sha256=encode(digest(
        convert_to(verification_plan_json,'UTF8'),'sha256'
    ),'hex')),
    CHECK ((status='indeterminate')=(indeterminate_phase IS NOT NULL)),
    CHECK (status<>'indeterminate' OR last_error IS NOT NULL),
    CHECK (status<>'verification_failed' OR last_error IS NOT NULL),
    CHECK ((apply_attempt_count=0)=(applying_at IS NULL)),
    CHECK ((verification_attempt_count=0)=(verifying_at IS NULL)),
    CHECK (
        (status IN ('applied','verifying','verified','verification_failed') OR
         (status='indeterminate' AND indeterminate_phase='verification'))
        =(mutation_evidence_id IS NOT NULL)
    ),
    CHECK (
        (status IN ('applied','verifying','verified','verification_failed') OR
         (status='indeterminate' AND indeterminate_phase='verification'))
        =(applied_at IS NOT NULL)
    ),
    CHECK ((verification_receipt_json IS NULL)=(verification_receipt_sha256 IS NULL)),
    CHECK ((verification_receipt_json IS NULL)=(verification_evidence_id IS NULL)),
    CHECK ((verification_receipt_json IS NULL)=(verification_succeeded IS NULL)),
    CHECK (
        (status IN ('verified','verification_failed'))
        =(verification_receipt_json IS NOT NULL)
    ),
    CHECK ((status='verified')=(verification_succeeded IS TRUE)),
    CHECK ((status IN ('verified','verification_failed'))=(terminal_at IS NOT NULL)),
    CHECK (verified_repository_snapshot_id IS NULL OR source_repository_snapshot_id IS NOT NULL),
    CHECK (
        verified_repository_snapshot_id IS NULL OR
        status IN ('verified','verification_failed')
    ),
    CHECK (
        status NOT IN ('verified','verification_failed') OR
        source_repository_snapshot_id IS NULL OR
        verified_repository_snapshot_id IS NOT NULL
    ),
    UNIQUE (job_id,stage_id),
    UNIQUE (job_id,id),
    FOREIGN KEY (job_id,generation)
        REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id)
        REFERENCES job_steps(job_id,generation,id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,creator_step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,current_step_attempt)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    FOREIGN KEY (project_id,source_repository_snapshot_id)
        REFERENCES repository_snapshots(project_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (project_id,verified_repository_snapshot_id)
        REFERENCES repository_snapshots(project_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,mutation_evidence_id)
        REFERENCES evidence(job_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,verification_evidence_id)
        REFERENCES evidence(job_id,id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX idx_workspace_mutations_unresolved_generation
    ON workspace_mutation_operations(job_id,generation)
    WHERE status NOT IN ('verified','verification_failed');
CREATE UNIQUE INDEX idx_workspace_mutations_unresolved_workspace
    ON workspace_mutation_operations(workspace_id)
    WHERE status NOT IN ('verified','verification_failed');
CREATE INDEX idx_workspace_mutations_recovery
    ON workspace_mutation_operations(job_id,generation,status,id);

CREATE TABLE workspace_mutation_files (
    operation_id TEXT NOT NULL REFERENCES workspace_mutation_operations(id) ON DELETE RESTRICT,
    ordinal INTEGER NOT NULL CHECK (ordinal BETWEEN 0 AND 31),
    file_id TEXT NOT NULL CHECK (file_id ~ '^workspace_file_[0-9a-f]{64}$'),
    path TEXT NOT NULL CHECK (
        path<>'' AND path=BTRIM(path) AND octet_length(path)<=4096 AND
        left(path,1)<>'/' AND right(path,1)<>'/' AND
        position(chr(92) IN path)=0 AND path !~ E'[\r\n]' AND
        path !~ '(^|/)[.][.]?(/|$)' AND position('//' IN path)=0 AND
        ('/' || path || '/') !~ '/[.](git|omni)/'
    ),
    source_present BOOLEAN NOT NULL,
    source_kind TEXT CHECK (source_kind IN ('regular','symlink')),
    source_sha256 TEXT CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    source_size BIGINT CHECK (source_size>=0),
    source_mode INTEGER CHECK (source_mode BETWEEN 0 AND 511),
    source_link_target TEXT CHECK (
        source_link_target IS NULL OR
        (source_link_target<>'' AND octet_length(source_link_target)<=16384)
    ),
    expected_present BOOLEAN NOT NULL,
    expected_kind TEXT CHECK (expected_kind IN ('regular','symlink')),
    expected_sha256 TEXT CHECK (expected_sha256 ~ '^[0-9a-f]{64}$'),
    expected_size BIGINT CHECK (expected_size>=0),
    expected_mode INTEGER CHECK (expected_mode BETWEEN 0 AND 511),
    expected_link_target TEXT CHECK (
        expected_link_target IS NULL OR
        (expected_link_target<>'' AND octet_length(expected_link_target)<=16384)
    ),
    PRIMARY KEY (operation_id,ordinal),
    UNIQUE (operation_id,file_id),
    UNIQUE (operation_id,path),
    CHECK (source_present OR expected_present),
    CHECK (
        source_present=(source_kind IS NOT NULL) AND
        source_present=(source_sha256 IS NOT NULL) AND
        source_present=(source_size IS NOT NULL) AND
        source_present=(source_mode IS NOT NULL) AND
        (NOT source_present OR
         (source_kind='regular' AND source_link_target IS NULL) OR
         (source_kind='symlink' AND source_link_target IS NOT NULL))
    ),
    CHECK (
        expected_present=(expected_kind IS NOT NULL) AND
        expected_present=(expected_sha256 IS NOT NULL) AND
        expected_present=(expected_size IS NOT NULL) AND
        expected_present=(expected_mode IS NOT NULL) AND
        (NOT expected_present OR
         (expected_kind='regular' AND expected_link_target IS NULL) OR
         (expected_kind='symlink' AND expected_link_target IS NOT NULL))
    ),
    CHECK (
        source_kind<>'symlink' OR (
            source_size=octet_length(source_link_target) AND
            source_sha256=encode(digest(
                convert_to('symlink','UTF8') || decode('00','hex') ||
                convert_to(source_link_target,'UTF8'),'sha256'
            ),'hex')
        )
    ),
    CHECK (
        expected_kind<>'symlink' OR (
            expected_size=octet_length(expected_link_target) AND
            expected_sha256=encode(digest(
                convert_to('symlink','UTF8') || decode('00','hex') ||
                convert_to(expected_link_target,'UTF8'),'sha256'
            ),'hex')
        )
    ),
    CHECK (
        ROW(source_present,source_kind,source_sha256,source_size,source_mode,source_link_target)
        IS DISTINCT FROM
        ROW(expected_present,expected_kind,expected_sha256,expected_size,expected_mode,expected_link_target)
    )
);

CREATE FUNCTION workspace_mutation_exact_keys(value JSONB,required TEXT[])
RETURNS BOOLEAN AS $$
    SELECT jsonb_typeof(value)='object' AND
        COALESCE((SELECT array_agg(key ORDER BY key) FROM jsonb_object_keys(value) AS key),ARRAY[]::TEXT[])=
        COALESCE((SELECT array_agg(item ORDER BY item) FROM unnest(required) AS item),ARRAY[]::TEXT[]);
$$ LANGUAGE SQL IMMUTABLE;

CREATE FUNCTION validate_workspace_mutation_insert()
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
    SELECT jobs.status='running' AND jobs.pipeline='coding' AND
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

CREATE TRIGGER workspace_mutation_insert_validate
BEFORE INSERT ON workspace_mutation_operations
FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_insert();

CREATE FUNCTION validate_workspace_mutation_file_insert()
RETURNS TRIGGER AS $$
DECLARE
    operation_workspace_id TEXT;
    operation_status TEXT;
    operation_sealed_at TIMESTAMPTZ;
BEGIN
    SELECT workspace_id,status,sealed_at
      INTO operation_workspace_id,operation_status,operation_sealed_at
    FROM workspace_mutation_operations WHERE id=NEW.operation_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'workspace mutation file has no operation';
    END IF;
    IF operation_status<>'prepared' OR operation_sealed_at IS NOT NULL THEN
        RAISE EXCEPTION 'workspace mutation file authority is sealed';
    END IF;
    IF NEW.file_id<>'workspace_file_' || encode(digest(
        convert_to(operation_workspace_id,'UTF8') || decode('00','hex') ||
        convert_to(NEW.path,'UTF8') || decode('00','hex'),'sha256'
    ),'hex') THEN
        RAISE EXCEPTION 'workspace mutation file identity differs from workspace path authority';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_mutation_file_insert_validate
BEFORE INSERT ON workspace_mutation_files
FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_file_insert();

CREATE FUNCTION require_workspace_mutation_files()
RETURNS TRIGGER AS $$
DECLARE
    file_count INTEGER;
    first_ordinal INTEGER;
    last_ordinal INTEGER;
    operation_sealed_at TIMESTAMPTZ;
BEGIN
    SELECT sealed_at INTO operation_sealed_at
    FROM workspace_mutation_operations WHERE id=NEW.id;
    SELECT COUNT(*),MIN(ordinal),MAX(ordinal)
      INTO file_count,first_ordinal,last_ordinal
    FROM workspace_mutation_files WHERE operation_id=NEW.id;
    IF operation_sealed_at IS NULL OR file_count NOT BETWEEN 1 AND 32 OR
       first_ordinal<>0 OR last_ordinal<>file_count-1 THEN
        RAISE EXCEPTION 'workspace mutation must be sealed with 1-32 exact files';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER workspace_mutation_requires_files
AFTER INSERT ON workspace_mutation_operations
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_workspace_mutation_files();

CREATE FUNCTION workspace_mutation_current_authority_valid(operation workspace_mutation_operations)
RETURNS BOOLEAN AS $$
    SELECT COALESCE((
        SELECT jobs.status='running' AND jobs.current_generation=operation.generation AND
               jobs.project_id=operation.project_id AND
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

CREATE FUNCTION validate_workspace_mutation_update()
RETURNS TRIGGER AS $$
DECLARE
    valid_mutation_evidence BOOLEAN;
    valid_verification_evidence BOOLEAN;
    receipt JSONB;
    plan JSONB := NEW.verification_plan_json::JSONB;
    planned_count INTEGER;
    valid_command_count INTEGER;
    all_commands_succeeded BOOLEAN;
    any_command_failed BOOLEAN;
BEGIN
    IF ROW(OLD.id,OLD.command_sha256,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.creator_step_attempt,OLD.creator_worker_id,OLD.project_id,
           OLD.owner_id,OLD.stage_id,OLD.workspace_id,OLD.workspace_root,
           OLD.source_state_id,OLD.expected_state_id,OLD.source_repository_snapshot_id,
           OLD.patch,OLD.patch_sha256,OLD.verification_plan_json,
           OLD.verification_plan_sha256,OLD.prepared_at)
       IS DISTINCT FROM
       ROW(NEW.id,NEW.command_sha256,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.creator_step_attempt,NEW.creator_worker_id,NEW.project_id,
           NEW.owner_id,NEW.stage_id,NEW.workspace_id,NEW.workspace_root,
           NEW.source_state_id,NEW.expected_state_id,NEW.source_repository_snapshot_id,
           NEW.patch,NEW.patch_sha256,NEW.verification_plan_json,
           NEW.verification_plan_sha256,NEW.prepared_at) THEN
        RAISE EXCEPTION 'workspace mutation command authority is immutable';
    END IF;
    IF OLD.sealed_at IS NULL AND NEW.sealed_at IS NOT NULL THEN
        IF OLD.status<>'prepared' OR NEW.status<>'prepared' OR
           OLD.apply_attempt_count<>0 OR NEW.apply_attempt_count<>0 OR
           OLD.verification_attempt_count<>0 OR NEW.verification_attempt_count<>0 OR
           (to_jsonb(OLD)-ARRAY['sealed_at','updated_at']) IS DISTINCT FROM
           (to_jsonb(NEW)-ARRAY['sealed_at','updated_at']) THEN
            RAISE EXCEPTION 'workspace mutation sealing transition is invalid';
        END IF;
        IF NOT workspace_mutation_current_authority_valid(NEW) THEN
            RAISE EXCEPTION 'workspace mutation sealing lost exact current step-attempt authority';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.sealed_at IS DISTINCT FROM OLD.sealed_at OR NEW.sealed_at IS NULL THEN
        RAISE EXCEPTION 'workspace mutation file seal is immutable';
    END IF;
    IF OLD.status IN ('verified','verification_failed') THEN
        RAISE EXCEPTION '% workspace mutation is terminal',OLD.status;
    END IF;
    IF NOT workspace_mutation_current_authority_valid(NEW) THEN
        RAISE EXCEPTION 'workspace mutation transition lost exact current step-attempt authority';
    END IF;
    IF OLD.status=NEW.status AND OLD.status NOT IN ('applying','verifying') THEN
        IF (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
           (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN
            RAISE EXCEPTION 'workspace mutation authority replay cannot change journal state';
        END IF;
        RETURN NEW;
    END IF;
    IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','applied','indeterminate')) OR
       (OLD.status='applying' AND NEW.status NOT IN ('applying','prepared','applied','indeterminate')) OR
       (OLD.status='applied' AND NEW.status NOT IN ('verifying','indeterminate')) OR
       (OLD.status='verifying' AND NEW.status NOT IN ('verifying','applied','verified','verification_failed','indeterminate')) OR
       (OLD.status='indeterminate' AND OLD.indeterminate_phase='apply' AND
        NEW.status NOT IN ('applying','applied')) OR
       (OLD.status='indeterminate' AND OLD.indeterminate_phase='verification' AND
        NEW.status NOT IN ('verifying','applied','verified','verification_failed')) THEN
        RAISE EXCEPTION 'workspace mutation transition from %/% to %/% is invalid',
            OLD.status,OLD.indeterminate_phase,NEW.status,NEW.indeterminate_phase;
    END IF;
    IF (NEW.status='applying' AND NEW.apply_attempt_count<>OLD.apply_attempt_count+1) OR
       (NEW.status<>'applying' AND NEW.apply_attempt_count<>OLD.apply_attempt_count) OR
       (NEW.status='applying' AND
        (NEW.applying_at IS NULL OR NEW.applying_at IS NOT DISTINCT FROM OLD.applying_at)) OR
       (NEW.status<>'applying' AND NEW.applying_at IS DISTINCT FROM OLD.applying_at) THEN
        RAISE EXCEPTION 'workspace mutation application attempt transition is invalid';
    END IF;
    IF (NEW.status='verifying' AND
        NEW.verification_attempt_count<>OLD.verification_attempt_count+1) OR
       (NEW.status<>'verifying' AND
        NEW.verification_attempt_count<>OLD.verification_attempt_count) OR
       (NEW.status='verifying' AND
        (NEW.verifying_at IS NULL OR NEW.verifying_at IS NOT DISTINCT FROM OLD.verifying_at)) OR
       (NEW.status<>'verifying' AND NEW.verifying_at IS DISTINCT FROM OLD.verifying_at) THEN
        RAISE EXCEPTION 'workspace mutation verification attempt transition is invalid';
    END IF;
    IF NEW.mutation_evidence_id IS NOT NULL THEN
        SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
               evidence.kind='generated_diff' AND evidence.source_type='workspace' AND
               evidence.source_ref=NEW.stage_id AND
               evidence.payload_json->>'hash'=NEW.patch_sha256 AND
               evidence.payload_json->'metadata'->>'workspace_mutation_operation_id'=NEW.id AND
               evidence.payload_json->'metadata'->>'source_state_id'=NEW.source_state_id AND
               evidence.payload_json->'metadata'->>'expected_state_id'=NEW.expected_state_id
          INTO valid_mutation_evidence
        FROM evidence WHERE evidence.id=NEW.mutation_evidence_id;
        IF valid_mutation_evidence IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'applied workspace mutation has invalid generated-diff evidence';
        END IF;
    END IF;
    IF NEW.status NOT IN ('verified','verification_failed') THEN
        RETURN NEW;
    END IF;
    receipt := NEW.verification_receipt_json::JSONB;
    IF NEW.verification_receipt_sha256<>encode(digest(
           convert_to(NEW.verification_receipt_json,'UTF8'),'sha256'),'hex') OR
       NOT workspace_mutation_exact_keys(receipt,ARRAY[
           'command_evidence_ids','expected_state_id','observed_state_id','operation_id',
           'schema','source_state_id','succeeded'
       ]) OR
       receipt->>'schema'<>'omnidex.workspace-mutation-verification-receipt.v1' OR
       receipt->>'operation_id'<>NEW.id OR
       receipt->>'source_state_id'<>NEW.source_state_id OR
       receipt->>'expected_state_id'<>NEW.expected_state_id OR
       receipt->>'observed_state_id'<>NEW.expected_state_id OR
       jsonb_typeof(receipt->'succeeded')<>'boolean' OR
       (receipt->>'succeeded')::BOOLEAN<>NEW.verification_succeeded OR
       jsonb_typeof(receipt->'command_evidence_ids')<>'array' THEN
        RAISE EXCEPTION 'workspace mutation verification receipt differs from command authority';
    END IF;
    planned_count := jsonb_array_length(plan->'commands');
    IF jsonb_array_length(receipt->'command_evidence_ids')<>planned_count OR
       EXISTS (
           SELECT 1 FROM jsonb_array_elements(receipt->'command_evidence_ids') AS item
           WHERE jsonb_typeof(item)<>'number' OR item#>>'{}' !~ '^[1-9][0-9]*$'
       ) OR
       (SELECT array_agg(value::BIGINT ORDER BY ordinal)
          FROM jsonb_array_elements_text(receipt->'command_evidence_ids')
               WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
       (SELECT array_agg(DISTINCT value::BIGINT ORDER BY value::BIGINT)
          FROM jsonb_array_elements_text(receipt->'command_evidence_ids') AS item(value)) THEN
        RAISE EXCEPTION 'workspace mutation verification receipt evidence set is invalid';
    END IF;
    SELECT COUNT(*),BOOL_AND((evidence.payload_json->'metadata'->>'succeeded')::BOOLEAN),
           BOOL_OR(NOT (evidence.payload_json->'metadata'->>'succeeded')::BOOLEAN)
      INTO valid_command_count,all_commands_succeeded,any_command_failed
    FROM jsonb_array_elements(plan->'commands') WITH ORDINALITY AS planned(command,ordinal)
    JOIN jsonb_array_elements_text(receipt->'command_evidence_ids')
         WITH ORDINALITY AS cited(evidence_id,ordinal) USING (ordinal)
    JOIN evidence ON evidence.id=cited.evidence_id::BIGINT AND
         evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
         evidence.kind=planned.command->>'kind' AND
         evidence.source_type='workspace_verification' AND evidence.source_ref=NEW.id AND
         evidence.payload_json->>'command'<>'' AND
         encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=
             planned.command->>'command_sha256' AND
         evidence.payload_json->'metadata'->>'succeeded' IN ('true','false');
    IF valid_command_count<>planned_count OR
       (NEW.status='verified' AND all_commands_succeeded IS DISTINCT FROM TRUE) OR
       (NEW.status='verification_failed' AND any_command_failed IS DISTINCT FROM TRUE) THEN
        RAISE EXCEPTION 'workspace mutation verification receipt command evidence is not exact';
    END IF;
    SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
           evidence.kind='workspace_verification_receipt' AND
           evidence.source_type='workspace_mutation' AND evidence.source_ref=NEW.id AND
           evidence.payload_json->>'hash'=NEW.verification_receipt_sha256 AND
           evidence.payload_json->>'excerpt'=NEW.verification_receipt_json AND
           evidence.payload_json->'metadata'->>'workspace_mutation_operation_id'=NEW.id AND
           evidence.payload_json->'metadata'->>'observed_state_id'=NEW.expected_state_id AND
           evidence.payload_json->'metadata'->>'succeeded'=NEW.verification_succeeded::TEXT
      INTO valid_verification_evidence
    FROM evidence WHERE evidence.id=NEW.verification_evidence_id;
    IF valid_verification_evidence IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'workspace mutation verification receipt evidence is invalid';
    END IF;
    IF NEW.verified_repository_snapshot_id IS NOT NULL THEN
        PERFORM 1 FROM repository_snapshots AS verified
        JOIN repository_snapshots AS source
          ON source.id=NEW.source_repository_snapshot_id AND
             source.project_id=NEW.project_id AND source.root=NEW.workspace_root
        WHERE verified.id=NEW.verified_repository_snapshot_id AND
              verified.project_id=NEW.project_id AND verified.root=NEW.workspace_root AND
              verified.repository_id=source.repository_id;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'workspace mutation verified Git binding differs from source authority';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_mutation_update_validate
BEFORE UPDATE ON workspace_mutation_operations
FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_update();

CREATE FUNCTION prevent_workspace_mutation_removal()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'workspace mutation journal is immutable';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER workspace_mutation_operations_delete_immutable
BEFORE DELETE ON workspace_mutation_operations
FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_removal();
CREATE TRIGGER workspace_mutation_operations_truncate_immutable
BEFORE TRUNCATE ON workspace_mutation_operations
FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_mutation_removal();

CREATE FUNCTION prevent_workspace_mutation_file_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'workspace mutation file authority is immutable';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER workspace_mutation_files_immutable
BEFORE UPDATE OR DELETE ON workspace_mutation_files
FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_file_change();
CREATE TRIGGER workspace_mutation_files_truncate_immutable
BEFORE TRUNCATE ON workspace_mutation_files
FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_mutation_file_change();

CREATE FUNCTION prevent_workspace_mutation_evidence_change()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM retired_repository_mutation_operations
        WHERE evidence_id=OLD.id
    ) OR EXISTS (
        SELECT 1 FROM workspace_mutation_operations
        WHERE mutation_evidence_id=OLD.id OR verification_evidence_id=OLD.id
    ) OR EXISTS (
        SELECT 1
        FROM workspace_mutation_operations AS operation,
             jsonb_array_elements_text(
                 operation.verification_receipt_json::JSONB->'command_evidence_ids'
             ) AS cited(evidence_id)
        WHERE operation.verification_receipt_json IS NOT NULL AND
              cited.evidence_id::BIGINT=OLD.id
    ) THEN
        RAISE EXCEPTION 'workspace mutation cited evidence is immutable';
    END IF;
    IF TG_OP='UPDATE' THEN
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_mutation_evidence_immutable
BEFORE UPDATE OR DELETE ON evidence
FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_evidence_change();

DO $postcondition$
BEGIN
    IF to_regclass('repository_mutation_operations') IS NOT NULL OR
       to_regclass('repository_mutation_files') IS NOT NULL OR
       to_regclass('retired_repository_mutation_operations') IS NULL OR
       to_regclass('retired_repository_mutation_files') IS NULL OR
       to_regclass('workspace_mutation_operations') IS NULL OR
       to_regclass('workspace_mutation_files') IS NULL OR
       to_regclass('idx_workspace_mutations_unresolved_generation') IS NULL OR
       to_regclass('idx_workspace_mutations_unresolved_workspace') IS NULL OR
       to_regprocedure('validate_repository_mutation_source()') IS NOT NULL OR
       to_regprocedure('prevent_repository_mutation_identity_change()') IS NOT NULL THEN
        RAISE EXCEPTION 'workspace mutation journal cutover postcondition failed';
    END IF;
END $postcondition$;

COMMIT;
