LOCK TABLE jobs IN ACCESS EXCLUSIVE MODE;

DO $$
DECLARE
    invalid_job BIGINT;
    invalid_pipeline TEXT;
BEGIN
    SELECT id,pipeline INTO invalid_job,invalid_pipeline
    FROM jobs
    WHERE pipeline NOT IN ('chat','coding','scrum')
      AND status NOT IN ('completed','failed','canceled')
    ORDER BY id
    LIMIT 1;
    IF invalid_job IS NOT NULL THEN
        RAISE EXCEPTION
            'executable pipeline cutover refuses nonterminal retired or unregistered pipeline % on job %',
            invalid_pipeline,
            invalid_job;
    END IF;
END
$$;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_executable_pipeline_authority CHECK (
        pipeline IN ('chat','coding','scrum')
        OR status IN ('completed','failed','canceled')
    );

CREATE FUNCTION enforce_jobs_executable_pipeline_authority()
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

CREATE TRIGGER jobs_executable_pipeline_authority
BEFORE INSERT OR UPDATE OR DELETE ON jobs
FOR EACH ROW EXECUTE FUNCTION enforce_jobs_executable_pipeline_authority();

CREATE TRIGGER jobs_history_truncate_immutable
BEFORE TRUNCATE ON jobs
FOR EACH STATEMENT EXECUTE FUNCTION enforce_jobs_executable_pipeline_authority();

DO $$
DECLARE
    authority_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO authority_count
    FROM pg_constraint
    WHERE conrelid='jobs'::regclass
      AND conname='jobs_executable_pipeline_authority'
      AND contype='c'
      AND convalidated
      AND NOT connoinherit;
    IF authority_count <> 1 THEN
        RAISE EXCEPTION 'executable pipeline constraint is unavailable';
    END IF;

    SELECT COUNT(*) INTO authority_count
    FROM pg_trigger AS trigger
    JOIN pg_class AS relation ON relation.oid=trigger.tgrelid
    JOIN pg_namespace AS namespace ON namespace.oid=relation.relnamespace
    JOIN pg_proc AS procedure ON procedure.oid=trigger.tgfoid
    WHERE namespace.nspname=current_schema()
      AND relation.relname='jobs'
      AND trigger.tgname IN (
          'jobs_executable_pipeline_authority',
          'jobs_history_truncate_immutable'
      )
      AND procedure.proname='enforce_jobs_executable_pipeline_authority'
      AND trigger.tgenabled='O'
      AND NOT trigger.tgisinternal;
    IF authority_count <> 2 THEN
        RAISE EXCEPTION 'executable pipeline triggers are unavailable';
    END IF;
END
$$;
