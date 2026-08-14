DO $install$
DECLARE
    installed_schema TEXT := current_schema();
    authority_schema TEXT := 'omnidex_host_authority_' || md5(current_schema());
BEGIN
    EXECUTE format($definition$
        CREATE FUNCTION %1$I.omnidex_authorize_step_attempt_transaction_v1(
            requested_job_id BIGINT,
            requested_generation BIGINT,
            requested_step_id BIGINT,
            requested_attempt BIGINT,
            requested_worker_id TEXT
        ) RETURNS BOOLEAN AS $body$
        DECLARE
            locked_job_status TEXT;
            locked_job_generation BIGINT;
            locked_step_status TEXT;
            locked_step_generation BIGINT;
            locked_step_superseded BIGINT;
            locked_step_attempt BIGINT;
            locked_step_worker TEXT;
            locked_attempt_status TEXT;
            locked_attempt_worker TEXT;
            locked_attempt_expires TIMESTAMPTZ;
        BEGIN
            IF requested_job_id<=0 OR requested_generation<=0 OR requested_step_id<=0 OR
               requested_attempt<=0 OR requested_worker_id IS NULL OR
               requested_worker_id='' OR requested_worker_id<>BTRIM(requested_worker_id) OR
               octet_length(requested_worker_id)>256 THEN
                RETURN FALSE;
            END IF;

            SELECT status,current_generation
              INTO locked_job_status,locked_job_generation
              FROM %1$I.jobs WHERE id=requested_job_id FOR UPDATE;
            IF NOT FOUND OR locked_job_status<>'running' OR
               locked_job_generation<>requested_generation THEN
                RETURN FALSE;
            END IF;

            SELECT status,generation,superseded_at_generation,current_attempt,worker_id
              INTO locked_step_status,locked_step_generation,locked_step_superseded,
                   locked_step_attempt,locked_step_worker
              FROM %1$I.job_steps
              WHERE job_id=requested_job_id AND id=requested_step_id
              FOR UPDATE;
            IF NOT FOUND OR locked_step_status<>'running' OR
               locked_step_generation<>requested_generation OR
               locked_step_superseded IS NOT NULL OR
               locked_step_attempt<>requested_attempt OR
               locked_step_worker IS DISTINCT FROM requested_worker_id THEN
                RETURN FALSE;
            END IF;

            SELECT status,worker_id,expires_at
              INTO locked_attempt_status,locked_attempt_worker,locked_attempt_expires
              FROM %1$I.job_step_attempts
              WHERE job_id=requested_job_id AND generation=requested_generation AND
                    step_id=requested_step_id AND attempt=requested_attempt
              FOR UPDATE;
            IF NOT FOUND OR locked_attempt_status<>'active' OR
               locked_attempt_worker<>requested_worker_id OR
               locked_attempt_expires<=clock_timestamp() THEN
                RETURN FALSE;
            END IF;
            RETURN TRUE;
        END;
        $body$ LANGUAGE plpgsql SECURITY DEFINER VOLATILE
    $definition$,installed_schema);
    EXECUTE format(
        'ALTER FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) SET search_path TO pg_catalog, %I',
        installed_schema,installed_schema
    );
    EXECUTE format(
        'CREATE SCHEMA %I AUTHORIZATION %I',authority_schema,current_user
    );
    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC',authority_schema);
    EXECUTE format(
        'ALTER FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) SET SCHEMA %I',
        installed_schema,authority_schema
    );
    EXECUTE format(
        'REVOKE ALL ON FUNCTION %I.omnidex_authorize_step_attempt_transaction_v1(bigint,bigint,bigint,bigint,text) FROM PUBLIC',
        authority_schema
    );
END;
$install$;

CREATE TABLE step_attempt_transaction_fence_authority (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    authority_schema TEXT NOT NULL CHECK (
        authority_schema='omnidex_host_authority_' || md5(current_schema())
    ),
    function_name TEXT NOT NULL CHECK (
        function_name='omnidex_authorize_step_attempt_transaction_v1'
    ),
    function_arguments TEXT NOT NULL CHECK (
        function_arguments='bigint, bigint, bigint, bigint, text'
    )
);

INSERT INTO step_attempt_transaction_fence_authority (
    singleton,authority_schema,function_name,function_arguments
) VALUES (
    TRUE,
    'omnidex_host_authority_' || md5(current_schema()),
    'omnidex_authorize_step_attempt_transaction_v1',
    'bigint, bigint, bigint, bigint, text'
);

CREATE OR REPLACE FUNCTION prevent_step_attempt_fence_authority_change()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'step-attempt transaction fence authority is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER step_attempt_fence_authority_update_immutable
BEFORE UPDATE OR DELETE ON step_attempt_transaction_fence_authority
FOR EACH ROW EXECUTE FUNCTION prevent_step_attempt_fence_authority_change();
CREATE TRIGGER step_attempt_fence_authority_truncate_immutable
BEFORE TRUNCATE ON step_attempt_transaction_fence_authority
FOR EACH STATEMENT EXECUTE FUNCTION prevent_step_attempt_fence_authority_change();
