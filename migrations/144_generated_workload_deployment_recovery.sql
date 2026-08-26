BEGIN;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM generated_workload_deployments WHERE status IN ('prepared','applying','indeterminate')) THEN
  RAISE EXCEPTION 'deployment recovery rail requires no pre-rail nonterminal deployment journal';
 END IF;
END $$;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads WHERE candidate_deployment_id IS NOT NULL) THEN
  RAISE EXCEPTION 'deployment recovery rail requires explicit recovery of every pre-rail project candidate';
 END IF;
 IF EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment
  JOIN generated_workload_deployment_executions AS execution ON execution.operation_id=deployment.id
  WHERE deployment.status='applied' AND
   ROW(execution.step_attempt,execution.worker_id) IS DISTINCT FROM
   ROW(deployment.current_step_attempt,deployment.current_worker_id)) THEN
  RAISE EXCEPTION 'deployment recovery rail found applied predecessor execution ownership';
 END IF;
END $$;
CREATE TABLE generated_workload_deployment_rollback_plans (
 operation_id TEXT PRIMARY KEY REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 policy TEXT NOT NULL CHECK (policy='compose_destroy_first_deployment.v1'),
 max_attempts INTEGER NOT NULL CHECK (max_attempts=3),
 slot_name TEXT NOT NULL CHECK (slot_name='rollback'),
 slot_ordinal INTEGER NOT NULL CHECK (slot_ordinal=900),
 command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
 workspace_sha256 TEXT NOT NULL CHECK (workspace_sha256 ~ '^[0-9a-f]{64}$'),
 compose_project TEXT NOT NULL CHECK (compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
 resource_observation TEXT NOT NULL CHECK(resource_observation='docker_compose_project_resources.v1'),
 require_container_absence BOOLEAN NOT NULL CHECK(require_container_absence),
	 require_network_absence BOOLEAN NOT NULL CHECK(require_network_absence),
	 require_volume_absence BOOLEAN NOT NULL CHECK(require_volume_absence),
	 state_marker_sha256 TEXT CHECK(state_marker_sha256 ~ '^[0-9a-f]{64}$'),
	 postcondition_json TEXT NOT NULL CHECK(octet_length(postcondition_json) BETWEEN 2 AND 4096),
	 postcondition_sha256 TEXT NOT NULL CHECK(
	  postcondition_sha256 ~ '^[0-9a-f]{64}$' AND
	  postcondition_sha256=encode(digest(convert_to(postcondition_json,'UTF8'),'sha256'),'hex')),
	 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
	);
	CREATE FUNCTION validate_generated_deployment_rollback_plan_insert() RETURNS TRIGGER AS $$
	DECLARE deployment generated_workload_deployments; expected_postcondition TEXT;
	BEGIN
	 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	 expected_postcondition:='{"compose_project":"'||NEW.compose_project||'","policy":"'||NEW.policy||
	  '","require_container_absence":true,"require_network_absence":true,"require_volume_absence":true,'||
	  '"resource_observation":"'||NEW.resource_observation||'","state_marker_sha256":"'||
	  COALESCE(NEW.state_marker_sha256,'')||'"}';
	 IF deployment.id IS NULL OR deployment.prior_deployment_id IS NOT NULL OR deployment.status<>'prepared' OR
		    NEW.workspace_sha256<>deployment.command_json::JSONB->'command'->>'workspace_sha256' OR
		    NEW.compose_project<>deployment.compose_project OR NEW.postcondition_json<>expected_postcondition THEN
	  RAISE EXCEPTION 'deployment rollback plan differs from first-deployment authority';
	 END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_rollback_plan_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_plans FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_plan_insert();
CREATE TABLE generated_workload_deployment_rollback_attempts (
 operation_id TEXT NOT NULL REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 job_id BIGINT NOT NULL,generation BIGINT NOT NULL,step_id BIGINT NOT NULL,step_attempt BIGINT NOT NULL CHECK(step_attempt>0),
 worker_id TEXT NOT NULL CHECK(worker_id<>'' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256),
 command_sha256 TEXT NOT NULL CHECK(command_sha256 ~ '^[0-9a-f]{64}$'),
 workspace_sha256 TEXT NOT NULL CHECK(workspace_sha256 ~ '^[0-9a-f]{64}$'),
 status TEXT NOT NULL CHECK(status IN ('started','completed')),succeeded BOOLEAN,
 evidence_id BIGINT UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
 result_sha256 TEXT CHECK(result_sha256 ~ '^[0-9a-f]{64}$'),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),completed_at TIMESTAMPTZ,
 PRIMARY KEY(operation_id,step_attempt),
 FOREIGN KEY(job_id,generation,step_id,step_attempt) REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
 CHECK((status='completed')=(succeeded IS NOT NULL)),CHECK((status='completed')=(evidence_id IS NOT NULL)),
 CHECK((status='completed')=(result_sha256 IS NOT NULL)),CHECK((status='completed')=(completed_at IS NOT NULL))
);
CREATE FUNCTION validate_generated_deployment_rollback_attempt_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; plan generated_workload_deployment_rollback_plans;
 attempt_count INTEGER; latest_attempt BIGINT; latest_residual BOOLEAN; candidate_valid BOOLEAN; authority_valid BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT count(*) INTO attempt_count FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id;
 SELECT max(step_attempt) INTO latest_attempt FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id;
	 IF latest_attempt IS NOT NULL THEN
	  SELECT outcome='residual' INTO latest_residual FROM generated_workload_deployment_rollback_observations
	   WHERE operation_id=NEW.operation_id AND rollback_step_attempt=latest_attempt AND basis='command_attempt';
 END IF;
 SELECT head.candidate_deployment_id=deployment.id AND head.candidate_step_attempt=NEW.step_attempt AND
  head.candidate_worker_id=NEW.worker_id INTO candidate_valid FROM generated_workload_project_deployment_heads AS head
  WHERE head.project_id=deployment.project_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
  steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
  FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=NEW.job_id;
 IF deployment.status NOT IN ('applying','indeterminate') OR
    ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
    ROW(deployment.job_id,deployment.generation,deployment.step_id,deployment.current_step_attempt,deployment.current_worker_id) OR
    NEW.command_sha256<>plan.command_sha256 OR NEW.workspace_sha256<>plan.workspace_sha256 OR
    EXISTS(SELECT 1 FROM generated_workload_deployment_executions
     WHERE operation_id=NEW.operation_id AND status='started') OR
    attempt_count>=plan.max_attempts OR (latest_attempt IS NOT NULL AND
     (NEW.step_attempt<=latest_attempt OR latest_residual IS DISTINCT FROM TRUE)) OR
    candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment rollback attempt authority is invalid or exhausted';
 END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_rollback_attempt_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_attempt_insert();
CREATE FUNCTION validate_generated_deployment_rollback_attempt_update() RETURNS TRIGGER AS $$
DECLARE valid_evidence BOOLEAN; candidate_valid BOOLEAN; authority_valid BOOLEAN;
	 deployment generated_workload_deployments;
BEGIN
 IF OLD.status<>'started' OR NEW.status<>'completed' OR
  (to_jsonb(OLD)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) THEN
  RAISE EXCEPTION 'deployment rollback attempt transition is invalid';
 END IF;
	SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	SELECT head.candidate_deployment_id=deployment.id AND head.candidate_job_id=NEW.job_id AND
	 head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
	 head.candidate_step_attempt=NEW.step_attempt AND head.candidate_worker_id=NEW.worker_id INTO candidate_valid
	 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
	SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
	 steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
	 attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
	 attempts.step_id=NEW.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=NEW.job_id;
	IF deployment.status NOT IN ('applying','indeterminate') OR
	 ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
	 ROW(deployment.job_id,deployment.generation,deployment.step_id,deployment.current_step_attempt,deployment.current_worker_id) OR
	 candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
	 RAISE EXCEPTION 'deployment rollback attempt lost current candidate authority'; END IF;
 SELECT evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND evidence.kind IN ('command_output','test_result') AND
  evidence.source_type='generated_workload_deployment_rollback' AND evidence.source_ref=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'succeeded'=NEW.succeeded::TEXT AND evidence.payload_json->'metadata'->>'execution'='true' AND
  evidence.payload_json->'metadata'->>'side_effect_possible'='true' AND
  (evidence.payload_json->'metadata'->>'step_attempt')::BIGINT=NEW.step_attempt AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
  evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=NEW.command_sha256 INTO valid_evidence
  FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment rollback attempt evidence is invalid'; END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_rollback_attempt_update_validate BEFORE UPDATE ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_attempt_update();
CREATE TABLE generated_workload_deployment_rollback_observations (
 operation_id TEXT NOT NULL,rollback_step_attempt BIGINT NOT NULL,
 observer_job_id BIGINT NOT NULL,observer_generation BIGINT NOT NULL,observer_step_id BIGINT NOT NULL,
	 observer_step_attempt BIGINT NOT NULL,observer_worker_id TEXT NOT NULL,
	 basis TEXT NOT NULL CHECK(basis IN ('command_attempt','pre_attempt')),
	 outcome TEXT NOT NULL CHECK(outcome IN ('clean','residual')),
 observation_json TEXT NOT NULL CHECK(octet_length(observation_json) BETWEEN 2 AND 32768),
	 observation_sha256 TEXT NOT NULL CHECK(
	  observation_sha256 ~ '^[0-9a-f]{64}$' AND
	  observation_sha256=encode(digest(convert_to(observation_json,'UTF8'),'sha256'),'hex')),
 evidence_id BIGINT NOT NULL UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
 observed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(operation_id,rollback_step_attempt),
	 FOREIGN KEY(observer_job_id,observer_generation,observer_step_id,observer_step_attempt)
  REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT
);
CREATE FUNCTION validate_generated_deployment_rollback_observation_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; plan generated_workload_deployment_rollback_plans;
	 observation JSONB:=NEW.observation_json::JSONB; candidate_valid BOOLEAN; authority_valid BOOLEAN; valid_evidence BOOLEAN;
	 expected_observation TEXT; invalid_resources BOOLEAN; basis_valid BOOLEAN;
	BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT head.candidate_deployment_id=deployment.id AND head.candidate_step_attempt=NEW.observer_step_attempt AND
  head.candidate_worker_id=NEW.observer_worker_id INTO candidate_valid FROM generated_workload_project_deployment_heads AS head
  WHERE head.project_id=deployment.project_id;
	 SELECT jobs.status='running' AND jobs.current_generation=NEW.observer_generation AND steps.status='running' AND
	  steps.generation=NEW.observer_generation AND steps.superseded_at_generation IS NULL AND
  steps.current_attempt=NEW.observer_step_attempt AND steps.worker_id=NEW.observer_worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.observer_worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
  FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.observer_step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.observer_job_id AND attempts.generation=NEW.observer_generation AND
	  attempts.step_id=NEW.observer_step_id AND attempts.attempt=NEW.observer_step_attempt WHERE jobs.id=NEW.observer_job_id;
	 SELECT CASE NEW.basis
	  WHEN 'command_attempt' THEN NEW.rollback_step_attempt>0 AND EXISTS(
	   SELECT 1 FROM generated_workload_deployment_rollback_attempts AS attempt
	   WHERE attempt.operation_id=NEW.operation_id AND attempt.step_attempt=NEW.rollback_step_attempt AND
	    attempt.command_sha256=plan.command_sha256 AND attempt.workspace_sha256=plan.workspace_sha256)
	  WHEN 'pre_attempt' THEN NEW.rollback_step_attempt=-NEW.observer_step_attempt AND NOT EXISTS(
	   SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) AND
	   EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id) AND
	   (SELECT count(*) FROM generated_workload_deployment_rollback_observations
	    WHERE operation_id=NEW.operation_id AND basis='pre_attempt')<plan.max_attempts
	  ELSE FALSE END INTO basis_valid;
	 SELECT EXISTS(
	  SELECT 1 FROM (
	   SELECT item,ordinality,LAG(item #>> '{}') OVER (PARTITION BY resource ORDER BY ordinality) AS previous,resource
	   FROM (VALUES ('container',observation->'container_ids'),('network',observation->'network_ids'),
	    ('volume',observation->'volume_names')) AS resources(resource,items)
	   CROSS JOIN LATERAL jsonb_array_elements(resources.items) WITH ORDINALITY AS values(item,ordinality)
	  ) AS entries WHERE jsonb_typeof(item)<>'string' OR
	   (resource IN ('container','network') AND (item #>> '{}') !~ '^[0-9a-f]{64}$') OR
	   (resource='volume' AND (item #>> '{}') !~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$') OR
	   (previous IS NOT NULL AND (item #>> '{}')<=previous)
	 ) INTO invalid_resources;
	 expected_observation:='{"compose_project":"'||plan.compose_project||'","container_ids":'||
	  replace((observation->'container_ids')::TEXT,', ',',')||',"network_ids":'||
	  replace((observation->'network_ids')::TEXT,', ',',')||',"postcondition_sha256":"'||
	  plan.postcondition_sha256||'","schema":"omnidex.generated-deployment-rollback-observation.v1","volume_names":'||
	  replace((observation->'volume_names')::TEXT,', ',',')||'}';
	 IF deployment.status NOT IN ('applying','indeterminate') OR
	  ROW(NEW.observer_job_id,NEW.observer_generation,NEW.observer_step_id,NEW.observer_step_attempt,NEW.observer_worker_id) IS DISTINCT FROM
	  ROW(deployment.job_id,deployment.generation,deployment.step_id,deployment.current_step_attempt,deployment.current_worker_id) OR
	  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR basis_valid IS DISTINCT FROM TRUE OR
	  (NEW.basis='command_attempt' AND EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts AS later
	   WHERE later.operation_id=NEW.operation_id AND later.step_attempt>NEW.rollback_step_attempt)) OR
	  NOT generated_deployment_exact_keys(observation,ARRAY['compose_project','container_ids','network_ids','postcondition_sha256','schema','volume_names']) OR
	  observation->>'schema'<>'omnidex.generated-deployment-rollback-observation.v1' OR
	  observation->>'compose_project'<>plan.compose_project OR observation->>'postcondition_sha256'<>plan.postcondition_sha256 OR
	  jsonb_typeof(observation->'container_ids')<>'array' OR jsonb_typeof(observation->'network_ids')<>'array' OR
	  jsonb_typeof(observation->'volume_names')<>'array' OR
	  jsonb_array_length(observation->'container_ids')>1024 OR jsonb_array_length(observation->'network_ids')>1024 OR
	  jsonb_array_length(observation->'volume_names')>1024 OR invalid_resources OR NEW.observation_json<>expected_observation OR
	  (NEW.outcome='clean') IS DISTINCT FROM
	   (plan.require_container_absence AND plan.require_network_absence AND plan.require_volume_absence AND
	    jsonb_array_length(observation->'container_ids')=0 AND jsonb_array_length(observation->'network_ids')=0 AND
	    jsonb_array_length(observation->'volume_names')=0) THEN
  RAISE EXCEPTION 'deployment rollback observation authority is invalid'; END IF;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind='deployment_observation' AND evidence.source_type='generated_workload_deployment_rollback_observation' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->>'hash'=NEW.observation_sha256 AND
	  evidence.payload_json->>'excerpt'=NEW.observation_json AND evidence.payload_json->'metadata'->>'outcome'=NEW.outcome AND
	  evidence.payload_json->'metadata'->>'basis'=NEW.basis AND
  (evidence.payload_json->'metadata'->>'rollback_step_attempt')::BIGINT=NEW.rollback_step_attempt AND
  evidence.payload_json->'metadata'->>'postcondition_sha256'=plan.postcondition_sha256 INTO valid_evidence
  FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment rollback observation evidence is invalid'; END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_rollback_observation_insert_validate BEFORE INSERT ON generated_workload_deployment_rollback_observations FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_rollback_observation_insert();
CREATE TRIGGER generated_deployment_rollback_plan_change_immutable BEFORE UPDATE OR DELETE ON generated_workload_deployment_rollback_plans FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_rollback_attempt_delete_immutable BEFORE DELETE ON generated_workload_deployment_rollback_attempts FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_rollback_observation_change_immutable BEFORE UPDATE OR DELETE ON generated_workload_deployment_rollback_observations FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_rollback_plan_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_plans FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_rollback_attempt_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_attempts FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
CREATE TRIGGER generated_deployment_rollback_observation_truncate_immutable BEFORE TRUNCATE ON generated_workload_deployment_rollback_observations FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();
	CREATE OR REPLACE FUNCTION prevent_generated_deployment_evidence_change() RETURNS TRIGGER AS $$ BEGIN
	 IF EXISTS (SELECT 1 FROM generated_workload_deployments WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_verifications WHERE evidence_id=OLD.id OR OLD.id=ANY(command_evidence_ids)) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_executions WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_observations WHERE evidence_id=OLD.id OR command_evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE evidence_id=OLD.id) OR
	  EXISTS (SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE evidence_id=OLD.id) THEN
	  RAISE EXCEPTION 'generated deployment cited evidence is immutable'; END IF;
	 IF TG_OP='UPDATE' THEN RETURN NEW; END IF; RETURN OLD;
	END $$ LANGUAGE plpgsql;
	CREATE OR REPLACE FUNCTION validate_generated_deployment_execution_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; binding generated_workload_deployment_verifications;
	 manifest_entry JSONB; target_index INTEGER; authority_valid BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
	SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND steps.status='running' AND
	 steps.generation=deployment.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
	 attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND attempts.generation=deployment.generation AND
	 attempts.step_id=deployment.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=deployment.job_id;
	SELECT entry,ordinality::INTEGER INTO manifest_entry,target_index
	 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands') WITH ORDINALITY AS item(entry,ordinality)
  WHERE (entry->'slot'->>'ordinal')::INTEGER=NEW.slot_ordinal;
 IF deployment.status<>'applying' OR NEW.slot_name='rollback' OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.operation_id) OR
	 (SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id)<>target_index-1 OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_executions AS execution
	  WHERE execution.operation_id=NEW.operation_id AND
	   (execution.step_attempt<>NEW.step_attempt OR execution.worker_id<>NEW.worker_id OR
	    execution.status<>'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR NOT EXISTS(
	     SELECT 1 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands') WITH ORDINALITY AS prior(entry,ordinality)
	     WHERE (prior.entry->'slot'->>'ordinal')::INTEGER=execution.slot_ordinal AND prior.ordinality<target_index))) OR
  NEW.workspace_sha256<>binding.workspace_sha256 OR NEW.step_attempt<>deployment.current_step_attempt OR
  NEW.worker_id<>deployment.current_worker_id OR authority_valid IS DISTINCT FROM TRUE OR
  NEW.status<>'started' OR manifest_entry IS NULL OR
  manifest_entry->'slot'->>'name'<>NEW.slot_name OR manifest_entry->>'command_sha256'<>NEW.command_sha256 THEN
  RAISE EXCEPTION 'deployment execution start authority is invalid'; END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION validate_generated_deployment_execution_update() RETURNS TRIGGER AS $$
DECLARE valid_evidence BOOLEAN; authority_valid BOOLEAN; deployment generated_workload_deployments;
BEGIN
 IF OLD.status<>'started' OR NEW.status<>'completed' OR
  (to_jsonb(OLD)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) IS DISTINCT FROM
  (to_jsonb(NEW)-ARRAY['status','succeeded','evidence_id','result_sha256','completed_at']) THEN
  RAISE EXCEPTION 'deployment execution transition is invalid'; END IF;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
	SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND steps.status='running' AND
	 steps.generation=deployment.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=NEW.step_attempt AND steps.worker_id=NEW.worker_id AND attempts.status='active' AND
	 attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND attempts.generation=deployment.generation AND
	 attempts.step_id=deployment.step_id AND attempts.attempt=NEW.step_attempt WHERE jobs.id=deployment.job_id;
 IF deployment.status<>'applying' OR NEW.step_attempt<>deployment.current_step_attempt OR
	 NEW.worker_id<>deployment.current_worker_id OR authority_valid IS DISTINCT FROM TRUE OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) OR
	 EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.operation_id) THEN
	 RAISE EXCEPTION 'deployment execution cannot complete after cleanup reconciliation'; END IF;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind IN ('command_output','test_result') AND evidence.source_type='generated_workload_deployment_execution' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->'metadata'->>'succeeded'=NEW.succeeded::TEXT AND
  evidence.payload_json->'metadata'->>'execution'='true' AND evidence.payload_json->'metadata'->>'side_effect_possible'='true' AND
  evidence.payload_json->'metadata'->>'deployment_operation_id'=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'slot'=NEW.slot_name AND
	 (evidence.payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
	 evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  encode(digest(convert_to(evidence.payload_json->>'command','UTF8'),'sha256'),'hex')=NEW.command_sha256 INTO valid_evidence
 FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment execution evidence is invalid'; END IF;
 RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION validate_generated_deployment_observation_insert() RETURNS TRIGGER AS $$
DECLARE observation JSONB:=NEW.observation_json::JSONB; execution generated_workload_deployment_executions;
	 deployment generated_workload_deployments; valid_evidence BOOLEAN; candidate_valid BOOLEAN; authority_valid BOOLEAN;
BEGIN
	SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO execution FROM generated_workload_deployment_executions
	 WHERE operation_id=NEW.operation_id AND slot_ordinal=NEW.slot_ordinal;
	SELECT head.candidate_deployment_id=deployment.id AND head.candidate_job_id=deployment.job_id AND
	 head.candidate_generation=deployment.generation AND head.candidate_step_id=deployment.step_id AND
	 head.candidate_step_attempt=execution.step_attempt AND head.candidate_worker_id=execution.worker_id INTO candidate_valid
	 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
	SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND steps.status='running' AND
	 steps.generation=deployment.generation AND steps.superseded_at_generation IS NULL AND
	 steps.current_attempt=execution.step_attempt AND steps.worker_id=execution.worker_id AND attempts.status='active' AND
	 attempts.worker_id=execution.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
	 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
	 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND attempts.generation=deployment.generation AND
	 attempts.step_id=deployment.step_id AND attempts.attempt=execution.step_attempt WHERE jobs.id=deployment.job_id;
 IF deployment.status<>'applying' OR execution.status<>'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR
	 execution.evidence_id<>NEW.command_evidence_id OR execution.step_attempt<>deployment.current_step_attempt OR
	 execution.worker_id<>deployment.current_worker_id OR candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  observation->>'schema'<>'omnidex.generated-service-observation.v1' OR observation->>'sha256'<>NEW.observation_sha256 OR
  observation->>'services_sha256'<>NEW.services_sha256 OR observation->>'endpoint_sha256'<>NEW.endpoint_sha256 THEN
  RAISE EXCEPTION 'deployment observation execution binding is invalid'; END IF;
 SELECT job_id=deployment.job_id AND step_id=deployment.step_id AND kind='deployment_observation' AND
	 source_type='docker_compose_observation' AND source_ref=NEW.operation_id AND
  payload_json->>'hash'=NEW.observation_sha256 AND payload_json->>'excerpt'=NEW.observation_json AND
  payload_json->'metadata'->>'slot'=NEW.slot_name AND
	 (payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  (payload_json->'metadata'->>'compose_ps_evidence_id')::BIGINT=NEW.command_evidence_id AND
  payload_json->'metadata'->>'command_sha256'=execution.command_sha256 AND
	 payload_json->'metadata'->>'workspace_sha256'=execution.workspace_sha256 AND
  payload_json->'metadata'->>'observation_sha256'=NEW.observation_sha256 AND
	 payload_json->'metadata'->>'services_sha256'=NEW.services_sha256 AND
  payload_json->'metadata'->>'endpoint_sha256'=NEW.endpoint_sha256 AND
	 payload_json->'metadata'->>'succeeded'='true' INTO valid_evidence FROM evidence WHERE id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'deployment observation evidence is invalid'; END IF;
	RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION validate_generated_deployment_update() RETURNS TRIGGER AS $$
DECLARE receipt JSONB; command JSONB:=NEW.command_json::JSONB->'command'; authority_valid BOOLEAN;
 expected_exec BIGINT[]; expected_obs BIGINT[]; valid_receipt_evidence BOOLEAN; manifest_count INTEGER; complete_count INTEGER;
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
	  IF OLD.status='indeterminate' AND NEW.terminal_code='external_quiescence_unproven' AND
	   EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id AND status='started') AND
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id) AND
	   (to_jsonb(OLD)-ARRAY['terminal_code','terminal_detail_sha256','updated_at']) IS NOT DISTINCT FROM
	   (to_jsonb(NEW)-ARRAY['terminal_code','terminal_detail_sha256','updated_at']) THEN NULL;
	  ELSIF OLD.status NOT IN ('prepared','applying','indeterminate') OR NEW.current_step_attempt<=OLD.current_step_attempt OR
	   (to_jsonb(OLD)-ARRAY['current_step_attempt','current_worker_id','updated_at']) IS DISTINCT FROM
	   (to_jsonb(NEW)-ARRAY['current_step_attempt','current_worker_id','updated_at']) THEN
	   RAISE EXCEPTION 'generated deployment replay cannot mutate state'; END IF;
	 ELSE
	  IF (OLD.status='prepared' AND NEW.status NOT IN ('applying','failed')) OR
   (OLD.status='applying' AND NEW.status NOT IN ('applied','failed','indeterminate','rolled_back')) OR
	   (OLD.status='indeterminate' AND NEW.status NOT IN ('applying','failed','rolled_back')) OR OLD.status IN ('applied','failed','rolled_back') THEN
   RAISE EXCEPTION 'generated deployment transition from % to % is invalid',OLD.status,NEW.status; END IF;
	  IF (NEW.status='applying' AND NEW.attempt_count<>OLD.attempt_count+1) OR
	   (NEW.status<>'applying' AND NEW.attempt_count<>OLD.attempt_count) THEN RAISE EXCEPTION 'generated deployment application attempt transition is invalid'; END IF;
	  IF NEW.status='applying' AND (EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id AND basis='pre_attempt') OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id)) THEN
	   RAISE EXCEPTION 'deployment has entered one-way cleanup reconciliation'; END IF;
	  IF NEW.status='failed' AND OLD.status IN ('applying','indeterminate') AND
	   (EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	    EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id AND basis='pre_attempt') OR
	    EXISTS(SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.id)) THEN
	   RAISE EXCEPTION 'side-effect-possible deployment failure requires observe-first cleanup'; END IF;
	 END IF;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND steps.status='running' AND
  steps.generation=NEW.generation AND steps.superseded_at_generation IS NULL AND
  steps.current_attempt=NEW.current_step_attempt AND steps.worker_id=NEW.current_worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.current_worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
  FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
  JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND attempts.generation=NEW.generation AND
  attempts.step_id=NEW.step_id AND attempts.attempt=NEW.current_step_attempt WHERE jobs.id=NEW.job_id;
 IF authority_valid IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'generated deployment transition lost exact current step-attempt authority'; END IF;
 IF OLD.status=NEW.status THEN RETURN NEW; END IF;
	IF NEW.status='applied' THEN
	  IF EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.id) OR
	   EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.id) THEN
	   RAISE EXCEPTION 'applied deployment cannot have rollback attempts'; END IF;
  receipt:=NEW.receipt_json::JSONB;
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
  SELECT array_agg(evidence_id ORDER BY evidence_id),count(*) FILTER(WHERE status='completed' AND succeeded AND
   step_attempt=NEW.current_step_attempt AND worker_id=NEW.current_worker_id)
   INTO expected_exec,complete_count FROM generated_workload_deployment_executions WHERE operation_id=NEW.id;
  SELECT jsonb_array_length(binding.lifecycle_manifest_json::JSONB->'commands') INTO manifest_count FROM generated_workload_deployment_verifications AS binding
   WHERE binding.operation_id=NEW.id AND binding.verification_id=receipt->>'workspace_verification_receipt_id';
  SELECT array_agg(evidence_id ORDER BY evidence_id) INTO expected_obs FROM generated_workload_deployment_observations WHERE operation_id=NEW.id;
  IF manifest_count NOT BETWEEN 6 AND 9 OR complete_count<>manifest_count OR to_jsonb(expected_exec)<>receipt->'execution_evidence_ids' OR
   cardinality(expected_obs)<>2 OR to_jsonb(expected_obs)<>receipt->'observation_evidence_ids' THEN RAISE EXCEPTION 'applied deployment evidence rail is incomplete'; END IF;
  SELECT kind='deployment_receipt' AND source_type='docker_compose_deployment' AND source_ref=NEW.id AND
   payload_json->>'hash'=NEW.receipt_sha256 AND payload_json->>'excerpt'=NEW.receipt_json INTO valid_receipt_evidence
   FROM evidence WHERE id=NEW.evidence_id AND job_id=NEW.job_id AND step_id=NEW.step_id;
  IF valid_receipt_evidence IS DISTINCT FROM TRUE THEN RAISE EXCEPTION 'applied deployment receipt evidence is invalid'; END IF;
 ELSIF NEW.status='rolled_back' AND (NOT EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations
  WHERE operation_id=NEW.id AND outcome='clean') OR EXISTS(SELECT 1 FROM generated_workload_deployment_executions
  WHERE operation_id=NEW.id AND status='started')) THEN
  RAISE EXCEPTION 'rolled-back deployment lacks clean rollback observation or forward-command quiescence';
 ELSIF OLD.receipt_json IS NULL AND NEW.receipt_json IS NOT NULL THEN RAISE EXCEPTION 'deployment receipt can be sealed only by applied transition'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE FUNCTION validate_generated_deployment_head_consistency() RETURNS TRIGGER AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads AS head
  LEFT JOIN generated_workload_deployments AS deployment ON deployment.id=head.candidate_deployment_id
  WHERE head.candidate_deployment_id IS NOT NULL AND (deployment.id IS NULL OR deployment.status NOT IN ('prepared','applying','indeterminate') OR
   deployment.current_step_attempt<>head.candidate_step_attempt OR deployment.current_worker_id<>head.candidate_worker_id OR
   deployment.job_id<>head.candidate_job_id OR deployment.generation<>head.candidate_generation OR deployment.step_id<>head.candidate_step_id)) THEN
  RAISE EXCEPTION 'committed project deployment candidate authority is inconsistent'; END IF;
 IF EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment WHERE deployment.status IN ('prepared','applying','indeterminate') AND
  NOT EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_plans AS plan WHERE plan.operation_id=deployment.id AND
   plan.workspace_sha256=deployment.command_json::JSONB->'command'->>'workspace_sha256')) THEN
  RAISE EXCEPTION 'committed nonterminal deployment lacks exact rollback plan'; END IF;
 IF EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment WHERE deployment.status IN ('applying','indeterminate') AND
  NOT EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads AS head WHERE head.candidate_deployment_id=deployment.id AND
   head.candidate_step_attempt=deployment.current_step_attempt AND head.candidate_worker_id=deployment.current_worker_id)) THEN
  RAISE EXCEPTION 'committed active deployment lacks exact project candidate'; END IF;
	IF EXISTS(
	 SELECT 1 FROM generated_workload_deployments AS deployment
	 LEFT JOIN jobs AS job ON job.id=deployment.job_id
	 LEFT JOIN job_steps AS step ON step.job_id=deployment.job_id AND step.id=deployment.step_id
	 WHERE deployment.status IN ('prepared','applying','indeterminate') AND
	  (job.id IS NULL OR job.status<>'running' OR job.current_generation<>deployment.generation OR
	   step.id IS NULL OR step.status<>'running' OR step.generation<>deployment.generation OR
	   step.superseded_at_generation IS NOT NULL)
	) THEN RAISE EXCEPTION 'committed nonterminal deployment lacks live job and step authority'; END IF;
	IF EXISTS(
	 SELECT 1 FROM generated_workload_deployments AS deployment
	 JOIN LATERAL (
	  SELECT observation.outcome FROM generated_workload_deployment_rollback_observations AS observation
	  WHERE observation.operation_id=deployment.id
	  ORDER BY (observation.basis='command_attempt') DESC,
	   CASE WHEN observation.basis='command_attempt' THEN observation.rollback_step_attempt
	        ELSE observation.observer_step_attempt END DESC LIMIT 1
	 ) AS latest ON TRUE
	 WHERE CASE WHEN EXISTS(SELECT 1 FROM generated_workload_deployment_executions AS execution
	                         WHERE execution.operation_id=deployment.id AND execution.status='started')
	  THEN deployment.status<>'indeterminate' OR deployment.terminal_code<>'external_quiescence_unproven'
	  WHEN latest.outcome='clean' THEN deployment.status<>'rolled_back'
	  ELSE deployment.status<>'indeterminate' END
	) THEN RAISE EXCEPTION 'committed rollback observation differs from deployment convergence state'; END IF;
 RETURN NULL;
END $$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_deployment AFTER INSERT OR UPDATE ON generated_workload_deployments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_head AFTER INSERT OR UPDATE ON generated_workload_project_deployment_heads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_observation AFTER INSERT ON generated_workload_deployment_rollback_observations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_plan AFTER INSERT ON generated_workload_deployment_rollback_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_rollback_attempt AFTER INSERT OR UPDATE ON generated_workload_deployment_rollback_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_execution AFTER INSERT OR UPDATE ON generated_workload_deployment_executions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_observation AFTER INSERT ON generated_workload_deployment_observations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_job AFTER INSERT OR UPDATE ON jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_step AFTER INSERT OR UPDATE ON job_steps DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_head_consistency();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM generated_workload_project_deployment_heads AS head
  LEFT JOIN generated_workload_deployments AS deployment ON deployment.id=head.candidate_deployment_id
  WHERE head.candidate_deployment_id IS NOT NULL AND (deployment.id IS NULL OR
   deployment.status NOT IN ('prepared','applying','indeterminate') OR
   deployment.current_step_attempt<>head.candidate_step_attempt OR deployment.current_worker_id<>head.candidate_worker_id)) OR
  EXISTS(SELECT 1 FROM generated_workload_deployments WHERE status IN ('prepared','applying','indeterminate')) OR
  EXISTS(SELECT 1 FROM generated_workload_deployments AS deployment
   JOIN generated_workload_deployment_executions AS execution ON execution.operation_id=deployment.id
   WHERE deployment.status='applied' AND ROW(execution.step_attempt,execution.worker_id) IS DISTINCT FROM
    ROW(deployment.current_step_attempt,deployment.current_worker_id)) THEN
  RAISE EXCEPTION 'deployment recovery rail postcondition is inconsistent';
 END IF;
END $$;
COMMIT;
