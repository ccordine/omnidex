BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM generated_workload_deployments) THEN
  RAISE EXCEPTION 'deployment evidence rail requires no pre-rail generated deployments';
 END IF;
END $$;
ALTER TABLE generated_workload_deployments ADD CONSTRAINT generated_deployment_resolved_config_distinct
 CHECK (command_json::JSONB->'command'->>'config_sha256'<>command_json::JSONB->'command'->>'compose_file_sha256');
CREATE TABLE generated_workload_verifications (
 id TEXT PRIMARY KEY CHECK (id ~ '^generated_workload_verification_[0-9a-f]{64}$'),
 receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
 receipt_json TEXT NOT NULL CHECK (octet_length(receipt_json) BETWEEN 2 AND 32768),
 job_id BIGINT NOT NULL, generation BIGINT NOT NULL CHECK (generation>0), step_id BIGINT NOT NULL,
 workspace_sha256 TEXT NOT NULL CHECK (workspace_sha256 ~ '^[0-9a-f]{64}$'),
 command_evidence_ids BIGINT[] NOT NULL CHECK (cardinality(command_evidence_ids) BETWEEN 1 AND 128),
 evidence_id BIGINT NOT NULL, creator_step_attempt BIGINT NOT NULL CHECK (creator_step_attempt>0),
 creator_worker_id TEXT NOT NULL CHECK (creator_worker_id<>'' AND creator_worker_id=BTRIM(creator_worker_id) AND octet_length(creator_worker_id)<=256),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK (id='generated_workload_verification_' || receipt_sha256),
 CHECK (receipt_sha256=encode(digest(convert_to(receipt_json,'UTF8'),'sha256'),'hex')),
 UNIQUE(job_id,evidence_id),
 FOREIGN KEY(job_id,generation) REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT,
 FOREIGN KEY(job_id,generation,step_id,creator_step_attempt) REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
 FOREIGN KEY(job_id,evidence_id) REFERENCES evidence(job_id,id) ON DELETE RESTRICT
);
CREATE INDEX generated_workload_verifications_generation ON generated_workload_verifications(job_id,generation);
CREATE TABLE generated_workload_deployment_verifications (
 operation_id TEXT PRIMARY KEY REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 verification_id TEXT NOT NULL UNIQUE REFERENCES generated_workload_verifications(id) ON DELETE RESTRICT,
 workspace_sha256 TEXT NOT NULL CHECK (workspace_sha256 ~ '^[0-9a-f]{64}$'),
 lifecycle_manifest_json TEXT NOT NULL CHECK (octet_length(lifecycle_manifest_json) BETWEEN 2 AND 8192),
 lifecycle_manifest_sha256 TEXT NOT NULL CHECK (lifecycle_manifest_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK (lifecycle_manifest_sha256=encode(digest(convert_to(lifecycle_manifest_json,'UTF8'),'sha256'),'hex'))
);
CREATE TABLE generated_workload_deployment_executions (
 operation_id TEXT NOT NULL REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 slot_name TEXT NOT NULL, slot_ordinal INTEGER NOT NULL,
 command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
 workspace_sha256 TEXT NOT NULL CHECK (workspace_sha256 ~ '^[0-9a-f]{64}$'),
 status TEXT NOT NULL CHECK (status IN ('started','completed')), succeeded BOOLEAN,
 evidence_id BIGINT UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
 result_sha256 TEXT CHECK (result_sha256 ~ '^[0-9a-f]{64}$'),
 step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
 worker_id TEXT NOT NULL CHECK (worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), completed_at TIMESTAMPTZ,
 PRIMARY KEY(operation_id,slot_ordinal), UNIQUE(operation_id,slot_name),
 CHECK ((slot_name,slot_ordinal) IN (('config',10),('build',20),('initial_start',30),('migrate',40),
  ('initial_observe',50),('state_write',60),('restart',70),('restart_start',80),
  ('final_observe',90),('state_read',100),('rollback',900))),
 CHECK ((status='completed')=(succeeded IS NOT NULL)), CHECK ((status='completed')=(evidence_id IS NOT NULL)),
 CHECK ((status='completed')=(result_sha256 IS NOT NULL)), CHECK ((status='completed')=(completed_at IS NOT NULL))
);
CREATE TABLE generated_workload_deployment_observations (
 operation_id TEXT NOT NULL, slot_name TEXT NOT NULL CHECK (slot_name IN ('initial_observe','final_observe')),
 slot_ordinal INTEGER NOT NULL CHECK ((slot_name,slot_ordinal) IN (('initial_observe',50),('final_observe',90))),
 command_evidence_id BIGINT NOT NULL REFERENCES evidence(id) ON DELETE RESTRICT,
 observation_json TEXT NOT NULL CHECK (octet_length(observation_json) BETWEEN 2 AND 32768),
 observation_sha256 TEXT NOT NULL CHECK (observation_sha256 ~ '^[0-9a-f]{64}$'),
 services_sha256 TEXT NOT NULL CHECK (services_sha256 ~ '^[0-9a-f]{64}$'),
 endpoint_sha256 TEXT NOT NULL CHECK (endpoint_sha256 ~ '^[0-9a-f]{64}$'),
 evidence_id BIGINT NOT NULL UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(operation_id,slot_ordinal),
 FOREIGN KEY(operation_id,slot_ordinal) REFERENCES generated_workload_deployment_executions(operation_id,slot_ordinal) ON DELETE RESTRICT
);
CREATE FUNCTION validate_generated_workload_verification_insert() RETURNS TRIGGER AS $$
DECLARE receipt JSONB:=NEW.receipt_json::JSONB; command_ids BIGINT[]; valid_commands INTEGER; valid_aggregate BOOLEAN; authority_valid BOOLEAN;
BEGIN
 IF NOT generated_deployment_exact_keys(receipt,ARRAY['commands','generation','job_id','schema','step_id','workspace_sha256']) OR
    receipt->>'schema'<>'omnidex.generated-workload-verification-receipt.v1' OR
    (receipt->>'job_id')::BIGINT<>NEW.job_id OR (receipt->>'generation')::BIGINT<>NEW.generation OR
    (receipt->>'step_id')::BIGINT<>NEW.step_id OR receipt->>'workspace_sha256'<>NEW.workspace_sha256 OR
    jsonb_typeof(receipt->'commands')<>'array' OR jsonb_array_length(receipt->'commands') NOT BETWEEN 1 AND 128 THEN
  RAISE EXCEPTION 'workspace verification receipt authority is invalid'; END IF;
 IF EXISTS (SELECT 1 FROM jsonb_array_elements(receipt->'commands') WITH ORDINALITY AS item(command,ordinal)
  WHERE NOT generated_deployment_exact_keys(command,ARRAY['command_sha256','evidence_id','kind','ordinal']) OR
  (command->>'ordinal')::INTEGER<>ordinal OR (command->>'evidence_id')::BIGINT<=0 OR
  command->>'kind' NOT IN ('command_output','test_result') OR command->>'command_sha256' !~ '^[0-9a-f]{64}$') THEN
  RAISE EXCEPTION 'workspace verification command manifest is invalid'; END IF;
 SELECT array_agg((command->>'evidence_id')::BIGINT ORDER BY ordinal) INTO command_ids
  FROM jsonb_array_elements(receipt->'commands') WITH ORDINALITY AS item(command,ordinal);
 IF command_ids<>NEW.command_evidence_ids OR command_ids IS DISTINCT FROM
    (SELECT array_agg(DISTINCT value ORDER BY value) FROM unnest(command_ids) AS value) THEN
  RAISE EXCEPTION 'workspace verification evidence identities are not exact ordered identities'; END IF;
 SELECT count(*) INTO valid_commands FROM jsonb_array_elements(receipt->'commands') AS item(command)
  JOIN evidence ON evidence.id=(command->>'evidence_id')::BIGINT AND evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
   evidence.kind=command->>'kind' AND evidence.payload_json->'metadata'->>'succeeded'='true' AND
   encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=command->>'command_sha256';
 SELECT kind='workspace_verification_receipt' AND source_type='workspace_verification' AND source_ref=NEW.id AND
  payload_json->>'hash'=NEW.receipt_sha256 AND payload_json->>'excerpt'=NEW.receipt_json AND
  payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  payload_json->'metadata'->'commands'=receipt->'commands' INTO valid_aggregate FROM evidence
  WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.current_attempt=NEW.creator_step_attempt AND steps.worker_id=NEW.creator_worker_id AND
  attempts.status='active' AND attempts.worker_id=NEW.creator_worker_id AND attempts.expires_at>clock_timestamp()
  INTO authority_valid FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.creator_step_attempt WHERE jobs.id=NEW.job_id;
 IF valid_commands<>cardinality(command_ids) OR valid_aggregate IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'workspace verification evidence or attempt authority is invalid'; END IF; RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_workload_verification_insert_validate BEFORE INSERT ON generated_workload_verifications FOR EACH ROW EXECUTE FUNCTION validate_generated_workload_verification_insert();
CREATE FUNCTION validate_generated_deployment_binding_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; verification generated_workload_verifications; manifest JSONB:=NEW.lifecycle_manifest_json::JSONB; command JSONB; valid_config BOOLEAN; config_metadata JSONB; expected_env JSONB;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id; command:=deployment.command_json::JSONB->'command';
 SELECT * INTO verification FROM generated_workload_verifications WHERE id=NEW.verification_id;
 IF verification.job_id<>deployment.job_id OR verification.generation<>deployment.generation OR verification.step_id<>deployment.step_id OR
  verification.workspace_sha256<>command->>'workspace_sha256' OR NEW.workspace_sha256<>verification.workspace_sha256 OR
  NOT generated_deployment_exact_keys(manifest,ARRAY['commands','schema']) OR
  manifest->>'schema'<>'omnidex.generated-workload-deployment-lifecycle-manifest.v1' OR
  jsonb_typeof(manifest->'commands')<>'array' OR jsonb_array_length(manifest->'commands') NOT BETWEEN 6 AND 9 THEN
  RAISE EXCEPTION 'deployment verification binding authority is invalid'; END IF;
 IF EXISTS (SELECT 1 FROM jsonb_array_elements(manifest->'commands') WITH ORDINALITY AS item(entry,ordinal)
  WHERE NOT generated_deployment_exact_keys(entry,ARRAY['command_sha256','slot','workspace_sha256']) OR
   NOT generated_deployment_exact_keys(entry->'slot',ARRAY['name','ordinal']) OR entry->>'workspace_sha256'<>NEW.workspace_sha256 OR
   entry->>'command_sha256' !~ '^[0-9a-f]{64}$' OR
   (entry->'slot'->>'name',(entry->'slot'->>'ordinal')::INTEGER) NOT IN
    (('build',20),('initial_start',30),('migrate',40),('initial_observe',50),('state_write',60),
     ('restart',70),('restart_start',80),('final_observe',90),('state_read',100)) OR
   (ordinal>1 AND (entry->'slot'->>'ordinal')::INTEGER<=
    (manifest->'commands'->(ordinal::INTEGER-2)->'slot'->>'ordinal')::INTEGER)) THEN RAISE EXCEPTION 'deployment lifecycle manifest is invalid'; END IF;
 IF (SELECT count(*) FROM jsonb_array_elements(manifest->'commands') AS item(entry) WHERE entry->'slot'->>'name' IN
  ('build','initial_start','initial_observe','restart','restart_start','final_observe'))<>6 OR
  ((manifest->'commands') @> '[{"slot":{"name":"migrate","ordinal":40}}]'::JSONB) <>
  ((manifest->'commands') @> '[{"slot":{"name":"state_write","ordinal":60}}]'::JSONB) OR
  ((manifest->'commands') @> '[{"slot":{"name":"migrate","ordinal":40}}]'::JSONB) <>
  ((manifest->'commands') @> '[{"slot":{"name":"state_read","ordinal":100}}]'::JSONB) THEN
  RAISE EXCEPTION 'deployment lifecycle manifest required slots are incomplete'; END IF;
 SELECT source_type='docker_compose_resolved_config' AND payload_json->'metadata'->>'resolved_config_sha256'=command->>'config_sha256' AND
  payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND payload_json->'metadata'->>'secret_set_sha256'=command->>'secret_set_sha256'
  AND payload_json->'metadata'->>'succeeded'='true' AND payload_json->'metadata'->>'implicit_env_disabled'='true',payload_json->'metadata'
  INTO valid_config,config_metadata FROM evidence WHERE id=verification.command_evidence_ids[cardinality(verification.command_evidence_ids)];
 SELECT to_jsonb(array_agg(name ORDER BY name)) INTO expected_env FROM
  (SELECT jsonb_array_elements_text(command->'required_secret_names') AS name UNION ALL
   SELECT 'HOST_BIND_ADDRESS' UNION ALL SELECT 'HOST_HTTP_PORT') AS names;
 IF valid_config IS DISTINCT FROM TRUE OR config_metadata->'environment_names' IS DISTINCT FROM expected_env OR
  jsonb_typeof(config_metadata->'service_hashes') IS DISTINCT FROM 'array' OR
  jsonb_array_length(config_metadata->'service_hashes')<>jsonb_array_length(command->'services') OR
  EXISTS (SELECT 1 FROM jsonb_array_elements(config_metadata->'service_hashes') WITH ORDINALITY AS item(service,ordinal)
   WHERE NOT generated_deployment_exact_keys(service,ARRAY['service','sha256']) OR
    service->>'service'<>command->'services'->>(ordinal::INTEGER-1) OR service->>'sha256' !~ '^[0-9a-f]{64}$') THEN
  RAISE EXCEPTION 'deployment resolved config proof is invalid'; END IF; RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_binding_insert_validate BEFORE INSERT ON generated_workload_deployment_verifications FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_binding_insert();
CREATE FUNCTION validate_generated_deployment_execution_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; binding generated_workload_deployment_verifications; manifest_entry JSONB;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
 SELECT entry INTO manifest_entry FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands') AS item(entry)
  WHERE (entry->'slot'->>'ordinal')::INTEGER=NEW.slot_ordinal;
 IF deployment.status NOT IN ('applying','indeterminate') OR NEW.workspace_sha256<>binding.workspace_sha256 OR
  NEW.step_attempt<>deployment.current_step_attempt OR NEW.worker_id<>deployment.current_worker_id OR NEW.status<>'started' OR
  (NEW.slot_name<>'rollback' AND (manifest_entry IS NULL OR manifest_entry->'slot'->>'name'<>NEW.slot_name OR
   manifest_entry->>'command_sha256'<>NEW.command_sha256)) THEN RAISE EXCEPTION 'deployment execution start authority is invalid'; END IF; RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_execution_insert_validate BEFORE INSERT ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_execution_insert();
CREATE FUNCTION validate_generated_deployment_execution_update() RETURNS TRIGGER AS $$
DECLARE valid_evidence BOOLEAN; deployment generated_workload_deployments;
BEGIN
 IF OLD.status<>'started' OR NEW.status<>'completed' OR
  (to_jsonb(OLD)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) THEN RAISE EXCEPTION 'deployment execution transition is invalid'; END IF;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind IN ('command_output','test_result') AND evidence.source_type='generated_workload_deployment_execution' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->'metadata'->>'succeeded'=NEW.succeeded::TEXT AND
  evidence.payload_json->'metadata'->>'execution'='true' AND evidence.payload_json->'metadata'->>'side_effect_possible'='true' AND
  evidence.payload_json->'metadata'->>'deployment_operation_id'=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'slot'=NEW.slot_name AND (evidence.payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=NEW.command_sha256 INTO valid_evidence
  FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment execution evidence is invalid'; END IF; RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_execution_update_validate BEFORE UPDATE ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_execution_update();
CREATE FUNCTION validate_generated_deployment_observation_insert() RETURNS TRIGGER AS $$
DECLARE observation JSONB:=NEW.observation_json::JSONB; execution generated_workload_deployment_executions; valid_evidence BOOLEAN;
BEGIN
 SELECT * INTO execution FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id AND slot_ordinal=NEW.slot_ordinal;
 IF execution.status<>'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR execution.evidence_id<>NEW.command_evidence_id OR
  observation->>'schema'<>'omnidex.generated-service-observation.v1' OR observation->>'sha256'<>NEW.observation_sha256 OR
  observation->>'services_sha256'<>NEW.services_sha256 OR observation->>'endpoint_sha256'<>NEW.endpoint_sha256 THEN
  RAISE EXCEPTION 'deployment observation execution binding is invalid'; END IF;
 SELECT job_id=(SELECT job_id FROM generated_workload_deployments WHERE id=NEW.operation_id) AND
  step_id=(SELECT step_id FROM generated_workload_deployments WHERE id=NEW.operation_id) AND
  kind='deployment_observation' AND source_type='docker_compose_observation' AND source_ref=NEW.operation_id AND
  payload_json->>'hash'=NEW.observation_sha256 AND payload_json->>'excerpt'=NEW.observation_json AND
  payload_json->'metadata'->>'slot'=NEW.slot_name AND (payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  (payload_json->'metadata'->>'compose_ps_evidence_id')::BIGINT=NEW.command_evidence_id AND
  payload_json->'metadata'->>'command_sha256'=execution.command_sha256 AND
  payload_json->'metadata'->>'workspace_sha256'=execution.workspace_sha256 AND
  payload_json->'metadata'->>'observation_sha256'=NEW.observation_sha256 AND
  payload_json->'metadata'->>'services_sha256'=NEW.services_sha256 AND
  payload_json->'metadata'->>'endpoint_sha256'=NEW.endpoint_sha256 AND
  payload_json->'metadata'->>'succeeded'='true' INTO valid_evidence FROM evidence WHERE id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment observation evidence is invalid'; END IF; RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_observation_insert_validate BEFORE INSERT ON generated_workload_deployment_observations FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_observation_insert();
DROP TRIGGER generated_deployment_update_validate ON generated_workload_deployments;
DROP FUNCTION validate_generated_deployment_update();
CREATE FUNCTION validate_generated_deployment_update() RETURNS TRIGGER AS $$
DECLARE receipt JSONB; command JSONB:=NEW.command_json::JSONB->'command'; authority_valid BOOLEAN; expected_exec BIGINT[]; expected_obs BIGINT[]; valid_receipt_evidence BOOLEAN; manifest_count INTEGER; complete_count INTEGER;
BEGIN
 IF ROW(OLD.id,OLD.command_sha256,OLD.command_json,OLD.job_id,OLD.generation,OLD.step_id,OLD.creator_step_attempt,OLD.creator_worker_id,
  OLD.project_id,OLD.compose_project,OLD.bind_host,OLD.endpoint_port_authority,OLD.requested_endpoint_port,OLD.prior_deployment_id,OLD.prepared_at)
  IS DISTINCT FROM ROW(NEW.id,NEW.command_sha256,NEW.command_json,NEW.job_id,NEW.generation,NEW.step_id,NEW.creator_step_attempt,NEW.creator_worker_id,
  NEW.project_id,NEW.compose_project,NEW.bind_host,NEW.endpoint_port_authority,NEW.requested_endpoint_port,NEW.prior_deployment_id,NEW.prepared_at) THEN
  RAISE EXCEPTION 'generated deployment command authority is immutable'; END IF;
 IF OLD.receipt_json IS NOT NULL AND ROW(OLD.receipt_json,OLD.receipt_sha256,OLD.evidence_id,OLD.healthy_endpoint_port,OLD.applied_at,OLD.observed_at)
  IS DISTINCT FROM ROW(NEW.receipt_json,NEW.receipt_sha256,NEW.evidence_id,NEW.healthy_endpoint_port,NEW.applied_at,NEW.observed_at) THEN
  RAISE EXCEPTION 'generated deployment receipt is immutable'; END IF;
 IF OLD.status=NEW.status THEN
  IF OLD.status<>'applying' OR NEW.current_step_attempt<=OLD.current_step_attempt OR
   (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
   (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN RAISE EXCEPTION 'generated deployment replay cannot mutate state'; END IF;
 ELSE
  IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','failed')) OR (OLD.status='applying' AND NEW.status NOT IN ('applied','failed','indeterminate','rolled_back')) OR
   (OLD.status='indeterminate' AND NEW.status NOT IN ('applying','applied','failed','rolled_back')) OR (OLD.status='applied' AND NEW.status<>'rolled_back') OR OLD.status IN ('failed','rolled_back') THEN
   RAISE EXCEPTION 'generated deployment transition from % to % is invalid',OLD.status,NEW.status; END IF;
  IF (NEW.status='applying' AND NEW.attempt_count<>OLD.attempt_count+1) OR (NEW.status<>'applying' AND NEW.attempt_count<>OLD.attempt_count) THEN
   RAISE EXCEPTION 'generated deployment application attempt transition is invalid'; END IF;
 END IF;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND steps.current_attempt=NEW.current_step_attempt AND
  steps.worker_id=NEW.current_worker_id AND steps.superseded_at_generation IS NULL AND attempts.status='active' AND attempts.worker_id=NEW.current_worker_id AND
  attempts.expires_at>clock_timestamp() INTO authority_valid FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND attempts.step_id=NEW.step_id AND
  attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
 IF authority_valid IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'generated deployment transition lost exact current step-attempt authority'; END IF;
 IF OLD.status=NEW.status THEN RETURN NEW; END IF;
 IF NEW.status='applied' THEN receipt:=NEW.receipt_json::JSONB;
  IF NEW.receipt_sha256<>encode(digest(convert_to(NEW.receipt_json,'UTF8'),'sha256'),'hex') OR
   NOT generated_deployment_exact_keys(receipt,ARRAY['applied_at','compose_project','config_sha256','endpoint_host','endpoint_path','endpoint_port','endpoint_scheme',
    'execution_evidence_ids','observation_evidence_ids','observed_at','operation_id','prior_deployment_id','schema','services','workspace_verification_receipt_id']) OR
   receipt->>'schema'<>'omnidex.generated-workload-deployment-receipt.v2' OR receipt->>'operation_id'<>NEW.id OR
   receipt->>'config_sha256'<>command->>'config_sha256' OR receipt->>'compose_project'<>NEW.compose_project OR
   receipt->>'endpoint_scheme'<>command->>'endpoint_scheme' OR receipt->>'endpoint_host'<>command->>'endpoint_host' OR
   receipt->>'endpoint_path'<>command->>'endpoint_path' OR (receipt->>'endpoint_port')::INTEGER<>NEW.healthy_endpoint_port OR
   (NEW.endpoint_port_authority='fixed' AND NEW.healthy_endpoint_port<>NEW.requested_endpoint_port) OR
   receipt->>'prior_deployment_id'<>COALESCE(NEW.prior_deployment_id,'') OR (receipt->>'applied_at')::TIMESTAMPTZ<>NEW.applied_at OR
   (receipt->>'observed_at')::TIMESTAMPTZ<>NEW.observed_at THEN RAISE EXCEPTION 'applied deployment receipt differs from command authority'; END IF;
  SELECT array_agg(evidence_id ORDER BY evidence_id),count(*) FILTER(WHERE status='completed' AND succeeded)
   INTO expected_exec,complete_count FROM generated_workload_deployment_executions WHERE operation_id=NEW.id AND slot_name<>'rollback';
  SELECT jsonb_array_length(binding.lifecycle_manifest_json::JSONB->'commands') INTO manifest_count FROM generated_workload_deployment_verifications AS binding
   WHERE binding.operation_id=NEW.id AND binding.verification_id=receipt->>'workspace_verification_receipt_id';
  SELECT array_agg(evidence_id ORDER BY evidence_id) INTO expected_obs FROM generated_workload_deployment_observations WHERE operation_id=NEW.id;
  IF manifest_count NOT BETWEEN 6 AND 9 OR complete_count<>manifest_count OR to_jsonb(expected_exec)<>receipt->'execution_evidence_ids' OR
   cardinality(expected_obs)<>2 OR to_jsonb(expected_obs)<>receipt->'observation_evidence_ids' THEN RAISE EXCEPTION 'applied deployment evidence rail is incomplete'; END IF;
  SELECT kind='deployment_receipt' AND source_type='docker_compose_deployment' AND source_ref=NEW.id AND
   payload_json->>'hash'=NEW.receipt_sha256 AND payload_json->>'excerpt'=NEW.receipt_json INTO valid_receipt_evidence
   FROM evidence WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id;
  IF valid_receipt_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'applied deployment receipt evidence is invalid'; END IF;
 ELSIF OLD.receipt_json IS NULL AND NEW.receipt_json IS NOT NULL THEN RAISE EXCEPTION 'deployment receipt can be sealed only by applied transition'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_update_validate BEFORE UPDATE ON generated_workload_deployments FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_update();
CREATE FUNCTION prevent_generated_deployment_rail_change() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'generated deployment evidence rail is immutable'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_verification_change_immutable BEFORE UPDATE OR DELETE ON generated_workload_verifications FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_verification_truncate_immutable BEFORE TRUNCATE ON generated_workload_verifications FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_verification_change_immutable BEFORE UPDATE OR DELETE ON generated_workload_deployment_verifications FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_execution_delete_immutable BEFORE DELETE ON generated_workload_deployment_executions FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_observation_change_immutable BEFORE UPDATE OR DELETE ON generated_workload_deployment_observations FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_verification_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_verifications FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_execution_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_executions FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_observation_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_observations FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE OR REPLACE FUNCTION prevent_generated_deployment_evidence_change() RETURNS TRIGGER AS $$ BEGIN
 IF EXISTS (SELECT 1 FROM generated_workload_deployments WHERE evidence_id=OLD.id) OR
  EXISTS (SELECT 1 FROM generated_workload_verifications WHERE evidence_id=OLD.id OR OLD.id=ANY(command_evidence_ids)) OR
  EXISTS (SELECT 1 FROM generated_workload_deployment_executions WHERE evidence_id=OLD.id) OR
  EXISTS (SELECT 1 FROM generated_workload_deployment_observations WHERE evidence_id=OLD.id OR command_evidence_id=OLD.id) THEN
  RAISE EXCEPTION 'generated deployment cited evidence is immutable'; END IF;
 IF TG_OP='UPDATE' THEN RETURN NEW; END IF; RETURN OLD;
END $$ LANGUAGE plpgsql;
COMMIT;
