BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM generated_workload_deployments) THEN
  RAISE EXCEPTION 'project deployment head authority requires no pre-head deployment history';
 END IF;
END $$;
ALTER TABLE generated_workload_deployments
 DROP CONSTRAINT generated_workload_deployments_compose_project_key;
DROP INDEX generated_deployments_endpoint_port_authority;

CREATE TABLE generated_workload_project_deployment_heads (
 project_id BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE RESTRICT,
 compose_project TEXT NOT NULL UNIQUE CHECK (compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
 secret_generation BIGINT NOT NULL CHECK (secret_generation>0),
 deployment_key_fingerprint_sha256 TEXT NOT NULL CHECK (deployment_key_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
 active_deployment_id TEXT UNIQUE REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 active_endpoint_scheme TEXT CHECK (active_endpoint_scheme IN ('http','https')),
 active_endpoint_host TEXT CHECK (active_endpoint_host ~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$'),
 active_endpoint_port INTEGER CHECK (active_endpoint_port BETWEEN 1 AND 65535),
 active_endpoint_path TEXT CHECK (active_endpoint_path ~ '^/[^?#[:space:]]*$' AND position(chr(92) IN active_endpoint_path)=0),
 revision BIGINT NOT NULL CHECK (revision>=0),
 fence BIGINT NOT NULL CHECK (fence>0),
 candidate_deployment_id TEXT UNIQUE REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 candidate_job_id BIGINT,
 candidate_generation BIGINT,
 candidate_step_id BIGINT,
 candidate_step_attempt BIGINT,
 candidate_worker_id TEXT CHECK (candidate_worker_id<>'' AND candidate_worker_id=BTRIM(candidate_worker_id) AND octet_length(candidate_worker_id)<=256),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK ((active_deployment_id IS NULL)=(active_endpoint_scheme IS NULL) AND
        (active_deployment_id IS NULL)=(active_endpoint_host IS NULL) AND
        (active_deployment_id IS NULL)=(active_endpoint_port IS NULL) AND
        (active_deployment_id IS NULL)=(active_endpoint_path IS NULL)),
 CHECK (active_endpoint_host IS NULL OR position('..' IN active_endpoint_host)=0),
 CHECK (active_endpoint_path IS NULL OR (active_endpoint_path='/' OR right(active_endpoint_path,1)<>'/')),
 CHECK ((candidate_deployment_id IS NULL)=(candidate_job_id IS NULL) AND
        (candidate_deployment_id IS NULL)=(candidate_generation IS NULL) AND
        (candidate_deployment_id IS NULL)=(candidate_step_id IS NULL) AND
        (candidate_deployment_id IS NULL)=(candidate_step_attempt IS NULL) AND
        (candidate_deployment_id IS NULL)=(candidate_worker_id IS NULL)),
 CHECK (candidate_generation IS NULL OR candidate_generation>0),
 CHECK (candidate_step_attempt IS NULL OR candidate_step_attempt>0),
 FOREIGN KEY(candidate_job_id,candidate_deployment_id)
  REFERENCES generated_workload_deployments(job_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(candidate_job_id,candidate_generation,candidate_step_id,candidate_step_attempt)
  REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX generated_project_deployment_active_port
 ON generated_workload_project_deployment_heads(active_endpoint_port)
 WHERE active_deployment_id IS NOT NULL;

CREATE TABLE generated_workload_project_deployment_head_history (
 id BIGSERIAL PRIMARY KEY,
 project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
 revision BIGINT NOT NULL CHECK (revision>=0),
 fence BIGINT NOT NULL CHECK (fence>0),
 event TEXT NOT NULL CHECK (event IN ('reserved','promoted','released')),
 compose_project TEXT NOT NULL,
 secret_generation BIGINT NOT NULL CHECK (secret_generation>0),
 deployment_key_fingerprint_sha256 TEXT NOT NULL CHECK (deployment_key_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
 active_deployment_id TEXT REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 active_endpoint_scheme TEXT,
 active_endpoint_host TEXT,
 active_endpoint_port INTEGER,
 active_endpoint_path TEXT,
 candidate_deployment_id TEXT REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 candidate_job_id BIGINT,
 candidate_generation BIGINT,
 candidate_step_id BIGINT,
 candidate_step_attempt BIGINT,
 candidate_worker_id TEXT,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,revision,fence,event)
);

CREATE FUNCTION validate_generated_project_deployment_head_change() RETURNS TRIGGER AS $$
DECLARE candidate_valid BOOLEAN; terminal_valid BOOLEAN; promoted_valid BOOLEAN;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'project deployment head is immutable'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>0 OR NEW.fence<>1 OR NEW.active_deployment_id IS NOT NULL OR
     NEW.candidate_deployment_id IS NULL THEN
   RAISE EXCEPTION 'first project deployment head requires one fenced candidate'; END IF;
 ELSE
  IF ROW(OLD.project_id,OLD.compose_project,OLD.secret_generation,OLD.deployment_key_fingerprint_sha256,OLD.created_at)
     IS DISTINCT FROM ROW(NEW.project_id,NEW.compose_project,NEW.secret_generation,NEW.deployment_key_fingerprint_sha256,NEW.created_at) THEN
   RAISE EXCEPTION 'project deployment stable authority is immutable'; END IF;
  IF NEW.updated_at<=OLD.updated_at THEN
   RAISE EXCEPTION 'project deployment head update time must advance'; END IF;
  IF OLD.candidate_deployment_id IS NULL AND NEW.candidate_deployment_id IS NOT NULL THEN
   IF NEW.revision<>OLD.revision OR NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_deployment_id,NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_deployment_id,OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment reservation transition is invalid'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NOT NULL THEN
   IF NEW.candidate_deployment_id<>OLD.candidate_deployment_id OR NEW.revision<>OLD.revision OR
      NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_deployment_id,NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_deployment_id,OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment candidate takeover transition is invalid'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NULL AND
        NEW.active_deployment_id IS NOT DISTINCT FROM OLD.active_deployment_id THEN
   IF NEW.revision<>OLD.revision OR NEW.fence<>OLD.fence+1 OR
      ROW(NEW.active_endpoint_scheme,NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path)
      IS DISTINCT FROM ROW(OLD.active_endpoint_scheme,OLD.active_endpoint_host,OLD.active_endpoint_port,OLD.active_endpoint_path) THEN
    RAISE EXCEPTION 'project deployment candidate release transition is invalid'; END IF;
   SELECT status IN ('failed','rolled_back') INTO terminal_valid
    FROM generated_workload_deployments WHERE id=OLD.candidate_deployment_id;
   IF terminal_valid IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'project deployment candidate release requires terminal failure'; END IF;
  ELSIF OLD.candidate_deployment_id IS NOT NULL AND NEW.candidate_deployment_id IS NULL AND
        NEW.active_deployment_id=OLD.candidate_deployment_id THEN
   IF NEW.revision<>OLD.revision+1 OR NEW.fence<>OLD.fence THEN
    RAISE EXCEPTION 'project deployment promotion revision is invalid'; END IF;
   SELECT deployment.status='applied' AND deployment.project_id=NEW.project_id AND
          deployment.compose_project=NEW.compose_project AND
          deployment.healthy_endpoint_port=NEW.active_endpoint_port AND
          deployment.receipt_json::JSONB->>'endpoint_scheme'=NEW.active_endpoint_scheme AND
          deployment.receipt_json::JSONB->>'endpoint_host'=NEW.active_endpoint_host AND
          deployment.receipt_json::JSONB->>'endpoint_path'=NEW.active_endpoint_path
    INTO promoted_valid FROM generated_workload_deployments AS deployment
    WHERE deployment.id=NEW.active_deployment_id;
   IF promoted_valid IS DISTINCT FROM TRUE THEN
    RAISE EXCEPTION 'project deployment promotion lacks exact applied receipt authority'; END IF;
  ELSE RAISE EXCEPTION 'project deployment head transition is invalid'; END IF;
 END IF;
 IF NEW.candidate_deployment_id IS NOT NULL THEN
  SELECT deployment.project_id=NEW.project_id AND deployment.compose_project=NEW.compose_project AND
         deployment.job_id=NEW.candidate_job_id AND deployment.generation=NEW.candidate_generation AND
         deployment.step_id=NEW.candidate_step_id AND deployment.current_step_attempt=NEW.candidate_step_attempt AND
         deployment.current_worker_id=NEW.candidate_worker_id AND
         deployment.status IN ('prepared','applying','indeterminate') AND
         COALESCE(deployment.prior_deployment_id,'')=COALESCE(NEW.active_deployment_id,'')
   INTO candidate_valid FROM generated_workload_deployments AS deployment
   WHERE deployment.id=NEW.candidate_deployment_id;
  IF candidate_valid IS DISTINCT FROM TRUE THEN
   RAISE EXCEPTION 'project deployment candidate authority is invalid'; END IF;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_project_deployment_head_change_validate
 BEFORE INSERT OR UPDATE OR DELETE ON generated_workload_project_deployment_heads
 FOR EACH ROW EXECUTE FUNCTION validate_generated_project_deployment_head_change();

CREATE FUNCTION record_generated_project_deployment_head_history() RETURNS TRIGGER AS $$
DECLARE head_event TEXT;
BEGIN
 IF TG_OP='INSERT' OR NEW.candidate_deployment_id IS NOT NULL THEN head_event:='reserved';
 ELSIF NEW.revision>OLD.revision THEN head_event:='promoted'; ELSE head_event:='released'; END IF;
 INSERT INTO generated_workload_project_deployment_head_history(
  project_id,revision,fence,event,compose_project,secret_generation,deployment_key_fingerprint_sha256,
  active_deployment_id,active_endpoint_scheme,active_endpoint_host,active_endpoint_port,active_endpoint_path,
  candidate_deployment_id,candidate_job_id,candidate_generation,candidate_step_id,candidate_step_attempt,candidate_worker_id)
 VALUES(NEW.project_id,NEW.revision,NEW.fence,head_event,NEW.compose_project,NEW.secret_generation,
  NEW.deployment_key_fingerprint_sha256,NEW.active_deployment_id,NEW.active_endpoint_scheme,
  NEW.active_endpoint_host,NEW.active_endpoint_port,NEW.active_endpoint_path,NEW.candidate_deployment_id,
  NEW.candidate_job_id,NEW.candidate_generation,NEW.candidate_step_id,NEW.candidate_step_attempt,NEW.candidate_worker_id);
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_project_deployment_head_history_record
 AFTER INSERT OR UPDATE ON generated_workload_project_deployment_heads
 FOR EACH ROW EXECUTE FUNCTION record_generated_project_deployment_head_history();
CREATE FUNCTION prevent_generated_project_deployment_history_change() RETURNS TRIGGER AS $$ BEGIN
 IF TG_OP='INSERT' AND pg_trigger_depth()=2 THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'project deployment head history is immutable';
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_project_deployment_history_change_immutable
 BEFORE INSERT OR UPDATE OR DELETE ON generated_workload_project_deployment_head_history
 FOR EACH ROW EXECUTE FUNCTION prevent_generated_project_deployment_history_change();
CREATE TRIGGER generated_project_deployment_history_truncate_immutable
 BEFORE TRUNCATE ON generated_workload_project_deployment_head_history
 FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_project_deployment_history_change();
CREATE TRIGGER generated_project_deployment_head_truncate_immutable
 BEFORE TRUNCATE ON generated_workload_project_deployment_heads
 FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_project_deployment_history_change();
CREATE FUNCTION prevent_active_project_deployment_retirement() RETURNS TRIGGER AS $$ BEGIN
 IF OLD.status='applied' AND NEW.status<>'applied' AND EXISTS(
  SELECT 1 FROM generated_workload_project_deployment_heads WHERE active_deployment_id=OLD.id) THEN
  RAISE EXCEPTION 'active project deployment cannot leave applied state'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_active_project_deployment_retirement_prevent
 BEFORE UPDATE ON generated_workload_deployments
 FOR EACH ROW EXECUTE FUNCTION prevent_active_project_deployment_retirement();
CREATE FUNCTION require_generated_project_deployment_candidate_transition() RETURNS TRIGGER AS $$
DECLARE candidate_valid BOOLEAN;
BEGIN
 IF NEW.status IN ('applying','applied') AND NEW.status<>OLD.status THEN
  SELECT head.candidate_deployment_id=NEW.id AND head.active_deployment_id IS NOT DISTINCT FROM NEW.prior_deployment_id AND
         head.compose_project=NEW.compose_project AND head.candidate_job_id=NEW.job_id AND
         head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
         head.candidate_step_attempt=NEW.current_step_attempt AND head.candidate_worker_id=NEW.current_worker_id
   INTO candidate_valid FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=NEW.project_id;
  IF candidate_valid IS DISTINCT FROM TRUE THEN
   RAISE EXCEPTION 'deployment transition lacks exact fenced project candidate authority'; END IF;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_project_deployment_candidate_transition_require
 BEFORE UPDATE ON generated_workload_deployments
 FOR EACH ROW EXECUTE FUNCTION require_generated_project_deployment_candidate_transition();

CREATE OR REPLACE FUNCTION validate_generated_deployment_insert() RETURNS TRIGGER AS $$
DECLARE envelope JSONB:=NEW.command_json::JSONB; command JSONB; authority JSONB;
 ordered_services TEXT[]; sorted_services TEXT[]; service_count INTEGER; distinct_services INTEGER; authority_valid BOOLEAN; prior_valid BOOLEAN;
BEGIN
 IF NOT generated_deployment_exact_keys(envelope,ARRAY['command','schema']) OR envelope->>'schema'<>'omnidex.generated-workload-deployment-command.v1' THEN
  RAISE EXCEPTION 'generated deployment command envelope is invalid'; END IF;
 command:=envelope->'command';
 IF NOT generated_deployment_exact_keys(command,ARRAY['adapter_id','adapter_version','authority','bind_host','compose_file_id','compose_file_sha256','compose_project','config_sha256','deployment_intent_job_id','deployment_intent_response_sha256','disposition','endpoint_host','endpoint_path','endpoint_port','endpoint_port_authority','endpoint_scheme','prior_deployment_id','profile_id','profile_version','required_secret_names','secret_set_sha256','services','source_snapshot_sha256','workspace_sha256']) THEN
  RAISE EXCEPTION 'generated deployment command fields are invalid'; END IF;
 authority:=command->'authority';
 IF NOT generated_deployment_exact_keys(authority,ARRAY['generation','job_id','project_id','step_id']) OR
    (authority->>'job_id')::BIGINT<>NEW.job_id OR (authority->>'generation')::BIGINT<>NEW.generation OR
    (authority->>'step_id')::BIGINT<>NEW.step_id OR (authority->>'project_id')::BIGINT<>NEW.project_id OR
    command->>'compose_project'<>NEW.compose_project OR command->>'bind_host'<>NEW.bind_host OR
    command->>'endpoint_port_authority'<>NEW.endpoint_port_authority OR
    (command->>'endpoint_port')::INTEGER<>NEW.requested_endpoint_port OR
    command->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') THEN
  RAISE EXCEPTION 'generated deployment redundant authority differs from command'; END IF;
 IF command->>'disposition'<>'persist_current_host' OR
    (command->>'endpoint_port_authority'='allocate' AND NEW.requested_endpoint_port<>0) OR
    (command->>'endpoint_port_authority'='fixed' AND NEW.requested_endpoint_port=0) OR
    command->>'endpoint_scheme' NOT IN ('http','https') OR
    command->>'endpoint_host' !~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$' OR position('..' IN command->>'endpoint_host')>0 OR
    command->>'endpoint_path' !~ '^/[^?#[:space:]]*$' OR position(chr(92) IN command->>'endpoint_path')>0 OR
    command->>'endpoint_path' ~ '(^|/)[.][.]?(/|$)|//' OR
    (command->>'endpoint_path'<>'/' AND right(command->>'endpoint_path',1)='/') OR
    command->>'compose_file_id' !~ '^file_[0-9a-f]{64}$' OR
    EXISTS (SELECT 1 FROM jsonb_each_text(command) AS pair WHERE pair.key IN ('compose_file_sha256','config_sha256','deployment_intent_job_id','deployment_intent_response_sha256','secret_set_sha256','source_snapshot_sha256','workspace_sha256') AND pair.value !~ '^[0-9a-f]{64}$') THEN
  RAISE EXCEPTION 'generated deployment typed command authority is invalid'; END IF;
 IF jsonb_typeof(command->'services')<>'array' OR jsonb_array_length(command->'services') NOT BETWEEN 1 AND 16 OR
    EXISTS (SELECT 1 FROM jsonb_array_elements(command->'services') AS item WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[a-z0-9][a-z0-9_.-]{0,62}$') THEN
  RAISE EXCEPTION 'generated deployment service authority is invalid'; END IF;
 SELECT array_agg(value ORDER BY ordinal),array_agg(value ORDER BY value),count(*),count(DISTINCT value)
  INTO ordered_services,sorted_services,service_count,distinct_services
  FROM jsonb_array_elements_text(command->'services') WITH ORDINALITY AS item(value,ordinal);
 IF ordered_services<>sorted_services OR service_count<>distinct_services THEN
  RAISE EXCEPTION 'generated deployment services must be sorted and unique'; END IF;
 IF jsonb_typeof(command->'required_secret_names')<>'array' OR jsonb_array_length(command->'required_secret_names')>16 OR
    EXISTS (SELECT 1 FROM jsonb_array_elements(command->'required_secret_names') AS item WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[A-Z][A-Z0-9_]{0,127}$') OR
    (SELECT count(*) FROM jsonb_array_elements_text(command->'required_secret_names'))<>(SELECT count(DISTINCT value) FROM jsonb_array_elements_text(command->'required_secret_names') AS value) OR
    (SELECT array_agg(value ORDER BY ordinal) FROM jsonb_array_elements_text(command->'required_secret_names') WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
    (SELECT array_agg(value ORDER BY value) FROM jsonb_array_elements_text(command->'required_secret_names') AS item(value)) THEN
  RAISE EXCEPTION 'generated deployment secret-name authority is invalid'; END IF;
 SELECT jobs.pipeline='coding' AND jobs.status='running' AND jobs.project_id=NEW.project_id AND
        jobs.current_generation=NEW.generation AND steps.status='running' AND steps.generation=NEW.generation AND
        steps.current_attempt=NEW.current_step_attempt AND steps.worker_id=NEW.current_worker_id AND
        steps.superseded_at_generation IS NULL AND attempts.status='active' AND
        attempts.worker_id=NEW.current_worker_id AND attempts.expires_at>clock_timestamp()
  INTO authority_valid FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
   attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
 IF authority_valid IS DISTINCT FROM TRUE OR NEW.status<>'prepared' OR NEW.attempt_count<>0 OR
    ROW(NEW.creator_step_attempt,NEW.creator_worker_id) IS DISTINCT FROM ROW(NEW.current_step_attempt,NEW.current_worker_id) THEN
  RAISE EXCEPTION 'generated deployment requires the exact current active step attempt'; END IF;
 IF NEW.prior_deployment_id IS NULL THEN
  SELECT NOT EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads
   WHERE project_id=NEW.project_id AND active_deployment_id IS NOT NULL) INTO prior_valid;
 ELSE
  SELECT prior.project_id=NEW.project_id AND prior.receipt_json IS NOT NULL AND
         head.active_deployment_id=NEW.prior_deployment_id INTO prior_valid
   FROM generated_workload_deployments AS prior
   JOIN generated_workload_project_deployment_heads AS head ON head.project_id=NEW.project_id
   WHERE prior.id=NEW.prior_deployment_id;
 END IF;
 IF prior_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'generated deployment predecessor must equal the current same-project head'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
COMMIT;
