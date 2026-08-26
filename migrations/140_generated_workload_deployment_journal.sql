BEGIN;
CREATE TABLE generated_workload_deployments (
    id TEXT PRIMARY KEY CHECK (id ~ '^generated_workload_deployment_[0-9a-f]{64}$'),
    command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    command_json TEXT NOT NULL CHECK (octet_length(command_json) BETWEEN 2 AND 32768),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    creator_step_attempt BIGINT NOT NULL CHECK (creator_step_attempt>0),
    creator_worker_id TEXT NOT NULL CHECK (creator_worker_id<>'' AND creator_worker_id=BTRIM(creator_worker_id) AND octet_length(creator_worker_id)<=256),
    current_step_attempt BIGINT NOT NULL CHECK (current_step_attempt>0),
    current_worker_id TEXT NOT NULL CHECK (current_worker_id<>'' AND current_worker_id=BTRIM(current_worker_id) AND octet_length(current_worker_id)<=256),
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    compose_project TEXT NOT NULL UNIQUE CHECK (compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
    bind_host TEXT NOT NULL CHECK (bind_host IN ('loopback','all_interfaces')),
    endpoint_port_authority TEXT NOT NULL CHECK (endpoint_port_authority IN ('allocate','fixed')),
    requested_endpoint_port INTEGER NOT NULL CHECK (requested_endpoint_port BETWEEN 0 AND 65535),
    healthy_endpoint_port INTEGER CHECK (healthy_endpoint_port BETWEEN 1 AND 65535),
    prior_deployment_id TEXT REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('prepared','applying','applied','failed','indeterminate','rolled_back')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
    terminal_code TEXT CHECK (terminal_code ~ '^[a-z][a-z0-9_]{0,63}$'),
    terminal_detail_sha256 TEXT CHECK (terminal_detail_sha256 ~ '^[0-9a-f]{64}$'),
    receipt_json TEXT,
    receipt_sha256 TEXT CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
    evidence_id BIGINT,
    prepared_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    applying_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    terminal_at TIMESTAMPTZ,
    CHECK (id='generated_workload_deployment_' || command_sha256),
    CHECK (command_sha256=encode(digest(convert_to(command_json,'UTF8'),'sha256'),'hex')),
    CHECK ((terminal_code IS NULL)=(terminal_detail_sha256 IS NULL)),
    CHECK ((status IN ('failed','indeterminate','rolled_back'))=(terminal_code IS NOT NULL)),
    CHECK ((receipt_json IS NULL)=(receipt_sha256 IS NULL)),
    CHECK ((receipt_json IS NULL)=(evidence_id IS NULL)),
    CHECK ((receipt_json IS NULL)=(healthy_endpoint_port IS NULL)),
    CHECK ((receipt_json IS NULL)=(applied_at IS NULL)),
    CHECK ((receipt_json IS NULL)=(observed_at IS NULL)),
    CHECK (status<>'applied' OR receipt_json IS NOT NULL),
    CHECK (attempt_count=0 OR applying_at IS NOT NULL),
    CHECK (status NOT IN ('applying','applied','indeterminate') OR attempt_count>0),
    CHECK ((status IN ('failed','rolled_back'))=(terminal_at IS NOT NULL)),
    CHECK (observed_at IS NULL OR observed_at>=applied_at),
    UNIQUE (job_id,generation),
    UNIQUE (job_id,id),
    FOREIGN KEY (job_id,generation) REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,creator_step_attempt) REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,current_step_attempt) REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,evidence_id) REFERENCES evidence(job_id,id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX generated_deployments_endpoint_port_authority ON generated_workload_deployments(healthy_endpoint_port) WHERE status='applied';
CREATE FUNCTION generated_deployment_exact_keys(value JSONB, required TEXT[]) RETURNS BOOLEAN AS $$ SELECT
 jsonb_typeof(value)='object' AND COALESCE((SELECT array_agg(key ORDER BY key) FROM jsonb_object_keys(value) AS key),ARRAY[]::TEXT[])=COALESCE((SELECT array_agg(item ORDER BY item) FROM unnest(required) AS item),ARRAY[]::TEXT[]); $$ LANGUAGE SQL IMMUTABLE;
CREATE FUNCTION validate_generated_deployment_insert() RETURNS TRIGGER AS $$
DECLARE envelope JSONB := NEW.command_json::JSONB; command JSONB; authority JSONB;
    ordered_services TEXT[]; sorted_services TEXT[]; service_count INTEGER; distinct_services INTEGER; authority_valid BOOLEAN; prior_valid BOOLEAN;
BEGIN
    IF NOT generated_deployment_exact_keys(envelope,ARRAY['command','schema']) OR
       envelope->>'schema'<>'omnidex.generated-workload-deployment-command.v1' THEN
        RAISE EXCEPTION 'generated deployment command envelope is invalid';
    END IF;
    command := envelope->'command';
    IF NOT generated_deployment_exact_keys(command,ARRAY['adapter_id','adapter_version','authority','bind_host','compose_file_id',
        'compose_file_sha256','compose_project','config_sha256','deployment_intent_job_id','deployment_intent_response_sha256',
        'disposition','endpoint_host','endpoint_path','endpoint_port','endpoint_port_authority','endpoint_scheme','prior_deployment_id',
        'profile_id','profile_version','required_secret_names','secret_set_sha256','services','source_snapshot_sha256','workspace_sha256']) THEN RAISE EXCEPTION 'generated deployment command fields are invalid'; END IF;
    authority := command->'authority';
    IF NOT generated_deployment_exact_keys(authority,ARRAY['generation','job_id','project_id','step_id']) OR
       (authority->>'job_id')::BIGINT<>NEW.job_id OR
       (authority->>'generation')::BIGINT<>NEW.generation OR
       (authority->>'step_id')::BIGINT<>NEW.step_id OR
       (authority->>'project_id')::BIGINT<>NEW.project_id OR
       command->>'compose_project'<>NEW.compose_project OR
       command->>'bind_host'<>NEW.bind_host OR
       command->>'endpoint_port_authority'<>NEW.endpoint_port_authority OR
       (command->>'endpoint_port')::INTEGER<>NEW.requested_endpoint_port OR
       command->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') THEN
        RAISE EXCEPTION 'generated deployment redundant authority differs from command';
    END IF;
    IF command->>'disposition'<>'persist_current_host' OR
       (command->>'endpoint_port_authority'='allocate' AND NEW.requested_endpoint_port<>0) OR
       (command->>'endpoint_port_authority'='fixed' AND NEW.requested_endpoint_port=0) OR
       command->>'endpoint_scheme' NOT IN ('http','https') OR
       command->>'endpoint_host' !~ '^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$' OR position('..' IN command->>'endpoint_host')>0 OR
       command->>'endpoint_path' !~ '^/[^?#[:space:]]*$' OR position(chr(92) IN command->>'endpoint_path')>0 OR
       command->>'endpoint_path' ~ '(^|/)[.][.]?(/|$)|//' OR
       (command->>'endpoint_path'<>'/' AND right(command->>'endpoint_path',1)='/') OR
       command->>'compose_file_id' !~ '^file_[0-9a-f]{64}$' OR
       EXISTS (SELECT 1 FROM jsonb_each_text(command) AS pair
               WHERE pair.key IN ('compose_file_sha256','config_sha256',
                   'deployment_intent_job_id','deployment_intent_response_sha256',
                   'secret_set_sha256','source_snapshot_sha256','workspace_sha256')
                 AND pair.value !~ '^[0-9a-f]{64}$') THEN
        RAISE EXCEPTION 'generated deployment typed command authority is invalid';
    END IF;
    IF jsonb_typeof(command->'services')<>'array' OR
       jsonb_array_length(command->'services') NOT BETWEEN 1 AND 16 OR
       EXISTS (SELECT 1 FROM jsonb_array_elements(command->'services') AS item
               WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[a-z0-9][a-z0-9_.-]{0,62}$') THEN
        RAISE EXCEPTION 'generated deployment service authority is invalid';
    END IF;
    SELECT array_agg(value ORDER BY ordinal),array_agg(value ORDER BY value),
           count(*),count(DISTINCT value)
      INTO ordered_services,sorted_services,service_count,distinct_services
      FROM jsonb_array_elements_text(command->'services') WITH ORDINALITY AS item(value,ordinal);
    IF ordered_services<>sorted_services OR service_count<>distinct_services THEN
        RAISE EXCEPTION 'generated deployment services must be sorted and unique';
    END IF;
    IF jsonb_typeof(command->'required_secret_names')<>'array' OR
       jsonb_array_length(command->'required_secret_names')>16 OR
       EXISTS (SELECT 1 FROM jsonb_array_elements(command->'required_secret_names') AS item
               WHERE jsonb_typeof(item)<>'string' OR item#>>'{}' !~ '^[A-Z][A-Z0-9_]{0,127}$') OR
       (SELECT count(*) FROM jsonb_array_elements_text(command->'required_secret_names'))<>
       (SELECT count(DISTINCT value) FROM jsonb_array_elements_text(command->'required_secret_names') AS value) OR
       (SELECT array_agg(value ORDER BY ordinal) FROM jsonb_array_elements_text(
           command->'required_secret_names') WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
       (SELECT array_agg(value ORDER BY value) FROM jsonb_array_elements_text(
           command->'required_secret_names') AS item(value)) THEN
        RAISE EXCEPTION 'generated deployment secret-name authority is invalid';
    END IF;
    SELECT jobs.pipeline='coding' AND jobs.status='running' AND
           jobs.project_id=NEW.project_id AND jobs.current_generation=NEW.generation AND
           steps.status='running' AND steps.generation=NEW.generation AND
           steps.current_attempt=NEW.current_step_attempt AND steps.worker_id=NEW.current_worker_id AND
           steps.superseded_at_generation IS NULL AND attempts.status='active' AND
           attempts.worker_id=NEW.current_worker_id AND attempts.expires_at>clock_timestamp()
      INTO authority_valid FROM jobs
      JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
      JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND
           attempts.generation=NEW.generation AND attempts.step_id=NEW.step_id AND
           attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
    IF authority_valid IS DISTINCT FROM TRUE OR NEW.status<>'prepared' OR NEW.attempt_count<>0 OR
       ROW(NEW.creator_step_attempt,NEW.creator_worker_id) IS DISTINCT FROM
       ROW(NEW.current_step_attempt,NEW.current_worker_id) THEN
        RAISE EXCEPTION 'generated deployment requires the exact current active step attempt';
    END IF;
    IF NEW.prior_deployment_id IS NOT NULL THEN
        SELECT prior.job_id=NEW.job_id AND prior.generation<NEW.generation AND
               prior.receipt_json IS NOT NULL INTO prior_valid
          FROM generated_workload_deployments AS prior WHERE prior.id=NEW.prior_deployment_id;
        IF prior_valid IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'generated deployment rollback lineage is invalid';
        END IF;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_insert_validate BEFORE INSERT ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_insert();
CREATE FUNCTION validate_generated_deployment_update() RETURNS TRIGGER AS $$
DECLARE receipt JSONB; command JSONB := NEW.command_json::JSONB->'command';
    receipt_services TEXT[]; valid_verification_count INTEGER; valid_receipt_evidence BOOLEAN; authority_valid BOOLEAN;
BEGIN
    IF ROW(OLD.id,OLD.command_sha256,OLD.command_json,OLD.job_id,OLD.generation,OLD.step_id,
           OLD.creator_step_attempt,OLD.creator_worker_id,OLD.project_id,OLD.compose_project,OLD.bind_host,
           OLD.endpoint_port_authority,OLD.requested_endpoint_port,
           OLD.prior_deployment_id,OLD.prepared_at)
       IS DISTINCT FROM ROW(NEW.id,NEW.command_sha256,NEW.command_json,NEW.job_id,NEW.generation,
           NEW.step_id,NEW.creator_step_attempt,NEW.creator_worker_id,NEW.project_id,NEW.compose_project,
           NEW.bind_host,NEW.endpoint_port_authority,NEW.requested_endpoint_port,
           NEW.prior_deployment_id,NEW.prepared_at) THEN
        RAISE EXCEPTION 'generated deployment command authority is immutable';
    END IF;
    IF OLD.receipt_json IS NOT NULL AND ROW(OLD.receipt_json,OLD.receipt_sha256,OLD.evidence_id,
           OLD.healthy_endpoint_port,OLD.applied_at,OLD.observed_at) IS DISTINCT FROM
           ROW(NEW.receipt_json,NEW.receipt_sha256,NEW.evidence_id,
           NEW.healthy_endpoint_port,NEW.applied_at,NEW.observed_at) THEN
        RAISE EXCEPTION 'generated deployment receipt is immutable';
    END IF;
    IF OLD.status=NEW.status THEN
        IF OLD.status<>'applying' OR NEW.current_step_attempt<=OLD.current_step_attempt OR
           (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
           (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN
            RAISE EXCEPTION 'generated deployment replay cannot mutate state'; END IF;
    ELSE
        IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','failed')) OR
           (OLD.status='applying' AND NEW.status NOT IN ('applied','failed','indeterminate','rolled_back')) OR
           (OLD.status='indeterminate' AND NEW.status NOT IN ('applying','applied','failed','rolled_back')) OR
           (OLD.status='applied' AND NEW.status<>'rolled_back') OR OLD.status IN ('failed','rolled_back') THEN
            RAISE EXCEPTION 'generated deployment transition from % to % is invalid',OLD.status,NEW.status; END IF;
        IF (NEW.status='applying' AND NEW.attempt_count<>OLD.attempt_count+1) OR
           (NEW.status<>'applying' AND NEW.attempt_count<>OLD.attempt_count) THEN
            RAISE EXCEPTION 'generated deployment application attempt transition is invalid'; END IF;
    END IF;
    SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND
           steps.status='running' AND steps.current_attempt=NEW.current_step_attempt AND
           steps.worker_id=NEW.current_worker_id AND steps.superseded_at_generation IS NULL AND
           attempts.status='active' AND attempts.worker_id=NEW.current_worker_id AND
           attempts.expires_at>clock_timestamp()
      INTO authority_valid FROM jobs
      JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
      JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND
           attempts.generation=NEW.generation AND attempts.step_id=NEW.step_id AND
           attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
    IF authority_valid IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'generated deployment transition lost exact current step-attempt authority';
    END IF;
    IF OLD.status=NEW.status THEN RETURN NEW; END IF;
    IF NEW.status='applied' THEN
        receipt := NEW.receipt_json::JSONB;
        IF NEW.receipt_sha256<>encode(digest(convert_to(NEW.receipt_json,'UTF8'),'sha256'),'hex') OR
           NOT generated_deployment_exact_keys(receipt,ARRAY[
               'applied_at','compose_project','config_sha256','endpoint_host','endpoint_path',
               'endpoint_port','endpoint_scheme','observed_at','operation_id','prior_deployment_id',
               'schema','services','verification_evidence_ids']) OR
           receipt->>'schema'<>'omnidex.generated-workload-deployment-receipt.v1' OR
           receipt->>'operation_id'<>NEW.id OR receipt->>'config_sha256'<>command->>'config_sha256' OR
           receipt->>'compose_project'<>NEW.compose_project OR
           receipt->>'endpoint_scheme'<>command->>'endpoint_scheme' OR
           receipt->>'endpoint_host'<>command->>'endpoint_host' OR
           receipt->>'endpoint_path'<>command->>'endpoint_path' OR
           (receipt->>'endpoint_port')::INTEGER<>NEW.healthy_endpoint_port OR
           (NEW.endpoint_port_authority='fixed' AND
            NEW.healthy_endpoint_port<>NEW.requested_endpoint_port) OR
           receipt->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') OR
           (receipt->>'applied_at')::TIMESTAMPTZ<>NEW.applied_at OR
           (receipt->>'observed_at')::TIMESTAMPTZ<>NEW.observed_at THEN
            RAISE EXCEPTION 'applied generated deployment receipt differs from command authority';
        END IF;
        IF jsonb_typeof(receipt->'services')<>'array' OR
           EXISTS (SELECT 1 FROM jsonb_array_elements(receipt->'services') AS service
                   WHERE NOT generated_deployment_exact_keys(service,ARRAY[
                       'container_id','health','image_digest','restart_policy','service','state']) OR
                     service->>'container_id' !~ '^[0-9a-f]{64}$' OR
                     service->>'image_digest' !~ '^sha256:[0-9a-f]{64}$' OR
                     service->>'restart_policy' NOT IN ('no','always','on-failure','unless-stopped') OR
                     service->>'state'<>'running' OR service->>'health'<>'healthy') THEN
            RAISE EXCEPTION 'applied generated deployment requires exact healthy service receipts';
        END IF;
        SELECT array_agg(service->>'service' ORDER BY ordinal) INTO receipt_services
          FROM jsonb_array_elements(receipt->'services') WITH ORDINALITY AS item(service,ordinal);
        IF to_jsonb(receipt_services)<>command->'services' THEN
            RAISE EXCEPTION 'applied generated deployment service receipt set is incomplete';
        END IF;
        IF jsonb_typeof(receipt->'verification_evidence_ids')<>'array' OR
           jsonb_array_length(receipt->'verification_evidence_ids')=0 OR
           EXISTS (SELECT 1 FROM jsonb_array_elements(receipt->'verification_evidence_ids') AS item
                   WHERE jsonb_typeof(item)<>'number' OR item#>>'{}' !~ '^[1-9][0-9]*$') OR
           (SELECT array_agg(value::BIGINT ORDER BY ordinal) FROM jsonb_array_elements_text(
               receipt->'verification_evidence_ids') WITH ORDINALITY AS item(value,ordinal)) IS DISTINCT FROM
           (SELECT array_agg(DISTINCT value::BIGINT ORDER BY value::BIGINT) FROM jsonb_array_elements_text(
               receipt->'verification_evidence_ids') AS item(value)) THEN
            RAISE EXCEPTION 'applied generated deployment verification evidence set is invalid';
        END IF;
        SELECT count(*) INTO valid_verification_count
          FROM jsonb_array_elements_text(receipt->'verification_evidence_ids') AS item(value)
          JOIN evidence ON evidence.id=item.value::BIGINT AND evidence.job_id=NEW.job_id AND
               evidence.step_id=NEW.step_id AND evidence.kind IN ('test_result','command_output') AND
               evidence.payload_json->'metadata'->>'succeeded'='true';
        IF valid_verification_count<>jsonb_array_length(receipt->'verification_evidence_ids') THEN
            RAISE EXCEPTION 'applied generated deployment verification evidence is not exact';
        END IF;
        SELECT evidence.kind='deployment_receipt' AND
               evidence.source_type='docker_compose_deployment' AND evidence.source_ref=NEW.id AND
               evidence.payload_json=jsonb_build_object(
                   'job_id',NEW.job_id,'step_id',NEW.step_id,'kind','deployment_receipt',
                   'source_type','docker_compose_deployment','source_ref',NEW.id,
                   'excerpt',NEW.receipt_json,
                   'summary','Applied one generated-workload deployment with a healthy observed service set.',
                   'hash',NEW.receipt_sha256,'confidence',1,'metadata',jsonb_build_object(
                       'deployment_operation_id',NEW.id,'compose_project',receipt->'compose_project',
                       'config_sha256',receipt->'config_sha256','endpoint_scheme',receipt->'endpoint_scheme',
                       'endpoint_host',receipt->'endpoint_host','endpoint_port',receipt->'endpoint_port',
                       'endpoint_path',receipt->'endpoint_path','applied_at',receipt->'applied_at',
                       'observed_at',receipt->'observed_at','verification_evidence_ids',
                       receipt->'verification_evidence_ids','succeeded',TRUE))
          INTO valid_receipt_evidence FROM evidence
          WHERE evidence.id=NEW.evidence_id AND evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id;
        IF valid_receipt_evidence IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'applied generated deployment receipt evidence is invalid';
        END IF;
    ELSIF OLD.receipt_json IS NULL AND NEW.receipt_json IS NOT NULL THEN
        RAISE EXCEPTION 'generated deployment receipt can be sealed only by healthy applied transition';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_update_validate BEFORE UPDATE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_update();
CREATE FUNCTION prevent_generated_deployment_removal() RETURNS TRIGGER AS $$
BEGIN RAISE EXCEPTION 'generated deployment journal is immutable'; END; $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_delete_immutable BEFORE DELETE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_removal();
CREATE TRIGGER generated_deployment_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployments FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_removal();
CREATE FUNCTION prevent_generated_deployment_evidence_change() RETURNS TRIGGER AS $$ BEGIN
    IF EXISTS (SELECT 1 FROM generated_workload_deployments WHERE evidence_id=OLD.id) THEN
        RAISE EXCEPTION 'sealed generated deployment receipt evidence is immutable'; END IF;
    IF EXISTS (SELECT 1 FROM generated_workload_deployments AS deployment WHERE deployment.receipt_json IS NOT NULL AND EXISTS (
        SELECT 1 FROM jsonb_array_elements_text(deployment.receipt_json::JSONB->'verification_evidence_ids') AS item(value)
        WHERE item.value::BIGINT=OLD.id)) THEN
        RAISE EXCEPTION 'sealed generated deployment verification evidence is immutable'; END IF;
    IF TG_OP='UPDATE' THEN RETURN NEW; END IF; RETURN OLD;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_evidence_immutable BEFORE UPDATE OR DELETE ON evidence FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_evidence_change();
COMMIT;
