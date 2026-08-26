BEGIN;

LOCK TABLE generated_workload_deployment_verifications IN ACCESS EXCLUSIVE MODE;
LOCK TABLE generated_workload_deployment_executions IN ACCESS EXCLUSIVE MODE;
LOCK TABLE generated_workload_deployment_rollback_attempts IN ACCESS EXCLUSIVE MODE;
LOCK TABLE generated_workload_deployment_rollback_observations IN ACCESS EXCLUSIVE MODE;

CREATE FUNCTION generated_deployment_lifecycle_manifest_valid(
 manifest_text TEXT,expected_workspace_sha256 TEXT
) RETURNS BOOLEAN AS $function$
DECLARE
 manifest JSONB;
 entry JSONB;
 command_count INTEGER;
 expected_names TEXT[];
 expected_ordinals INTEGER[];
 item_index INTEGER;
BEGIN
 IF manifest_text IS NULL OR expected_workspace_sha256 IS NULL THEN RETURN FALSE; END IF;
 manifest:=manifest_text::JSONB;
 IF jsonb_typeof(manifest) IS DISTINCT FROM 'object' OR
  generated_deployment_exact_keys(manifest,ARRAY['commands','schema']) IS DISTINCT FROM TRUE OR
  manifest->>'schema' IS DISTINCT FROM 'omnidex.generated-workload-deployment-lifecycle-manifest.v1' OR
  jsonb_typeof(manifest->'commands') IS DISTINCT FROM 'array' THEN RETURN FALSE; END IF;
 command_count:=jsonb_array_length(manifest->'commands');
 IF command_count=6 THEN
  expected_names:=ARRAY['build','initial_start','initial_observe','restart','restart_start','final_observe'];
  expected_ordinals:=ARRAY[20,30,50,70,80,90];
 ELSIF command_count=9 THEN
  expected_names:=ARRAY['build','initial_start','migrate','initial_observe','state_write','restart','restart_start','final_observe','state_read'];
  expected_ordinals:=ARRAY[20,30,40,50,60,70,80,90,100];
 ELSE
  RETURN FALSE;
 END IF;
 FOR item_index IN 0..command_count-1 LOOP
  entry:=manifest->'commands'->item_index;
  IF jsonb_typeof(entry) IS DISTINCT FROM 'object' OR
   generated_deployment_exact_keys(entry,ARRAY['command_sha256','slot','workspace_sha256']) IS DISTINCT FROM TRUE OR
   jsonb_typeof(entry->'slot') IS DISTINCT FROM 'object' OR
   generated_deployment_exact_keys(entry->'slot',ARRAY['name','ordinal']) IS DISTINCT FROM TRUE OR
   entry->'slot' IS DISTINCT FROM jsonb_build_object(
    'name',expected_names[item_index+1],'ordinal',expected_ordinals[item_index+1]
   ) OR jsonb_typeof(entry->'workspace_sha256') IS DISTINCT FROM 'string' OR
   entry->>'workspace_sha256' IS DISTINCT FROM expected_workspace_sha256 OR
   jsonb_typeof(entry->'command_sha256') IS DISTINCT FROM 'string' OR
   (entry->>'command_sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE THEN
   RETURN FALSE;
  END IF;
 END LOOP;
 RETURN TRUE;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$function$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION generated_deployment_vacant_namespace_preflight_valid(
 project TEXT,config_source TEXT,config_metadata JSONB
) RETURNS BOOLEAN AS $function$
DECLARE
 proof JSONB;
 expected TEXT;
BEGIN
 IF (project ~ '^[a-z0-9][a-z0-9_-]{0,62}$') IS DISTINCT FROM TRUE OR
  config_source IS DISTINCT FROM 'docker_compose_resolved_config' OR
  jsonb_typeof(config_metadata) IS DISTINCT FROM 'object' THEN RETURN FALSE; END IF;
 proof:=config_metadata->'namespace_preflight';
 IF jsonb_typeof(proof) IS DISTINCT FROM 'object' OR
  generated_deployment_exact_keys(proof,ARRAY[
   'compose_project','container_ids','network_ids','schema','sha256','volume_names'
  ]) IS DISTINCT FROM TRUE OR jsonb_typeof(proof->'schema') IS DISTINCT FROM 'string' OR
  proof->>'schema' IS DISTINCT FROM 'omnidex.generated-deployment-namespace-preflight.v1' OR
  jsonb_typeof(proof->'compose_project') IS DISTINCT FROM 'string' OR
  proof->>'compose_project' IS DISTINCT FROM project OR proof->'container_ids' IS DISTINCT FROM '[]'::JSONB OR
  proof->'network_ids' IS DISTINCT FROM '[]'::JSONB OR proof->'volume_names' IS DISTINCT FROM '[]'::JSONB OR
  jsonb_typeof(proof->'sha256') IS DISTINCT FROM 'string' OR
  (proof->>'sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE THEN RETURN FALSE; END IF;
 expected:='{"compose_project":'||to_jsonb(project)::TEXT||
  ',"container_ids":[],"network_ids":[],"schema":"omnidex.generated-deployment-namespace-preflight.v1","volume_names":[]}';
 RETURN proof->>'sha256' IS NOT DISTINCT FROM
  encode(digest(convert_to(expected,'UTF8'),'sha256'),'hex');
END;
$function$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION generated_deployment_resolved_config_metadata_types_valid(
 metadata JSONB
) RETURNS BOOLEAN AS $function$
BEGIN
 IF jsonb_typeof(metadata) IS DISTINCT FROM 'object' OR
  jsonb_typeof(metadata->'resolved_config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(metadata->'succeeded') IS DISTINCT FROM 'boolean' OR metadata->'succeeded' IS DISTINCT FROM 'true'::JSONB OR
  jsonb_typeof(metadata->'implicit_env_disabled') IS DISTINCT FROM 'boolean' OR
  metadata->'implicit_env_disabled' IS DISTINCT FROM 'true'::JSONB OR
  jsonb_typeof(metadata->'environment_names') IS DISTINCT FROM 'array' OR
  jsonb_typeof(metadata->'service_hashes') IS DISTINCT FROM 'array' OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'environment_names') AS item(name)
   WHERE jsonb_typeof(name) IS DISTINCT FROM 'string'
  ) OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'service_hashes') AS item(service)
   WHERE jsonb_typeof(service) IS DISTINCT FROM 'object' OR
    generated_deployment_exact_keys(service,ARRAY['service','sha256']) IS DISTINCT FROM TRUE OR
    jsonb_typeof(service->'service') IS DISTINCT FROM 'string' OR
    jsonb_typeof(service->'sha256') IS DISTINCT FROM 'string' OR
    (service->>'sha256' ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE
  ) THEN RETURN FALSE; END IF;
 RETURN TRUE;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$function$ LANGUAGE plpgsql IMMUTABLE;

CREATE FUNCTION generated_deployment_resolved_config_binding_valid(
 command JSONB,expected_workspace_sha256 TEXT,compose_project TEXT,
 config_source TEXT,metadata JSONB
) RETURNS BOOLEAN AS $function$
DECLARE
 expected_env JSONB;
BEGIN
 IF jsonb_typeof(command) IS DISTINCT FROM 'object' OR
  jsonb_typeof(command->'config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'services') IS DISTINCT FROM 'array' OR
  jsonb_typeof(command->'required_secret_names') IS DISTINCT FROM 'array' OR
  expected_workspace_sha256 IS DISTINCT FROM command->>'workspace_sha256' OR
  generated_deployment_resolved_config_metadata_types_valid(metadata) IS DISTINCT FROM TRUE OR
  config_source IS DISTINCT FROM 'docker_compose_resolved_config' OR
  metadata->>'resolved_config_sha256' IS DISTINCT FROM command->>'config_sha256' OR
  metadata->>'workspace_sha256' IS DISTINCT FROM expected_workspace_sha256 OR
  metadata->>'secret_set_sha256' IS DISTINCT FROM command->>'secret_set_sha256' OR
  generated_deployment_vacant_namespace_preflight_valid(
   compose_project,config_source,metadata
  ) IS DISTINCT FROM TRUE OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(command->'services') AS item(service)
   WHERE jsonb_typeof(service) IS DISTINCT FROM 'string'
  ) OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(command->'required_secret_names') AS item(name)
   WHERE jsonb_typeof(name) IS DISTINCT FROM 'string'
  ) OR jsonb_array_length(metadata->'service_hashes') IS DISTINCT FROM
   jsonb_array_length(command->'services') OR EXISTS(
   SELECT 1 FROM jsonb_array_elements(metadata->'service_hashes')
    WITH ORDINALITY AS item(service,ordinal)
   WHERE service->>'service' IS DISTINCT FROM command->'services'->>(ordinal::INTEGER-1)
  ) THEN RETURN FALSE; END IF;
 SELECT to_jsonb(array_agg(name ORDER BY name)) INTO expected_env FROM (
  SELECT jsonb_array_elements_text(command->'required_secret_names') AS name
  UNION ALL SELECT 'HOST_BIND_ADDRESS' UNION ALL SELECT 'HOST_HTTP_PORT'
 ) AS names;
 RETURN metadata->'environment_names' IS NOT DISTINCT FROM expected_env;
EXCEPTION WHEN others THEN
 RETURN FALSE;
END;
$function$ LANGUAGE plpgsql IMMUTABLE;

DO $preflight$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_verifications
  WHERE generated_deployment_lifecycle_manifest_valid(
   lifecycle_manifest_json,workspace_sha256
  ) IS DISTINCT FROM TRUE
 ) THEN
  RAISE EXCEPTION 'deployment authority hardening requires every existing lifecycle manifest to be exactly constructible';
 END IF;
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_verifications AS binding
  JOIN generated_workload_deployments AS deployment ON deployment.id=binding.operation_id
  LEFT JOIN generated_workload_verifications AS verification ON verification.id=binding.verification_id
  LEFT JOIN evidence AS config ON config.id=verification.command_evidence_ids[
   cardinality(verification.command_evidence_ids)
  ]
  WHERE config.id IS NULL OR generated_deployment_resolved_config_binding_valid(
    deployment.command_json::JSONB->'command',binding.workspace_sha256,
    deployment.compose_project,config.source_type,config.payload_json->'metadata'
   ) IS DISTINCT FROM TRUE
 ) THEN
  RAISE EXCEPTION 'deployment authority hardening requires exactly typed resolved-config evidence for every existing binding';
 END IF;
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions AS execution
  LEFT JOIN generated_workload_deployment_verifications AS binding ON binding.operation_id=execution.operation_id
  LEFT JOIN LATERAL (
   SELECT entry FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands') AS item(entry)
   WHERE entry->'slot'->>'ordinal' IS NOT DISTINCT FROM execution.slot_ordinal::TEXT
  ) AS manifest ON TRUE
  WHERE manifest.entry IS NULL OR manifest.entry->'slot'->>'name' IS DISTINCT FROM execution.slot_name OR
   manifest.entry->>'command_sha256' IS DISTINCT FROM execution.command_sha256 OR
   manifest.entry->>'workspace_sha256' IS DISTINCT FROM execution.workspace_sha256
 ) THEN
  RAISE EXCEPTION 'deployment authority hardening requires every existing execution to match its exact lifecycle manifest entry';
 END IF;
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_rollback_attempts AS rollback
  WHERE NOT EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions AS execution
   WHERE execution.operation_id=rollback.operation_id AND
    execution.slot_name='initial_start' AND execution.slot_ordinal=30
  )
 ) THEN
  RAISE EXCEPTION 'deployment authority hardening requires initial_start execution ownership for every existing rollback attempt';
 END IF;
END;
$preflight$;

CREATE OR REPLACE FUNCTION validate_generated_deployment_binding_insert() RETURNS TRIGGER AS $function$
DECLARE
 deployment generated_workload_deployments;
 verification generated_workload_verifications;
 command JSONB;
 valid_config BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id;
 command:=deployment.command_json::JSONB->'command';
 SELECT * INTO verification FROM generated_workload_verifications WHERE id=NEW.verification_id;
 IF deployment.id IS NULL OR verification.id IS NULL OR
  ROW(verification.job_id,verification.generation,verification.step_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id) OR
  verification.workspace_sha256 IS DISTINCT FROM command->>'workspace_sha256' OR
  NEW.workspace_sha256 IS DISTINCT FROM verification.workspace_sha256 OR
  jsonb_typeof(command->'config_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'workspace_sha256') IS DISTINCT FROM 'string' OR
  jsonb_typeof(command->'secret_set_sha256') IS DISTINCT FROM 'string' OR
  generated_deployment_lifecycle_manifest_valid(
   NEW.lifecycle_manifest_json,NEW.workspace_sha256
  ) IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment verification binding authority is invalid';
 END IF;
 SELECT generated_deployment_resolved_config_binding_valid(
  command,NEW.workspace_sha256,deployment.compose_project,source_type,payload_json->'metadata'
 ) INTO valid_config FROM evidence
 WHERE id=verification.command_evidence_ids[cardinality(verification.command_evidence_ids)];
 IF valid_config IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment resolved config proof is invalid';
 END IF;
 RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_generated_deployment_execution_insert() RETURNS TRIGGER AS $function$
DECLARE
 deployment generated_workload_deployments;
 binding generated_workload_deployment_verifications;
 manifest_entry JSONB;
 target_index INTEGER;
 authority_valid BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
 SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND
  steps.status='running' AND steps.generation=deployment.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND
  attempts.generation=deployment.generation AND attempts.step_id=deployment.step_id AND
  attempts.attempt=NEW.step_attempt WHERE jobs.id=deployment.job_id;
 SELECT entry,ordinality::INTEGER INTO manifest_entry,target_index
 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
  WITH ORDINALITY AS item(entry,ordinality)
 WHERE entry->'slot'->>'ordinal' IS NOT DISTINCT FROM NEW.slot_ordinal::TEXT;
 IF deployment.status IS DISTINCT FROM 'applying' OR NEW.slot_name IS NOT DISTINCT FROM 'rollback' OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=NEW.operation_id) OR
  (SELECT count(*) FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id)
   IS DISTINCT FROM target_index-1 OR
  EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions AS execution
   WHERE execution.operation_id=NEW.operation_id AND
    (execution.step_attempt IS DISTINCT FROM NEW.step_attempt OR
     execution.worker_id IS DISTINCT FROM NEW.worker_id OR
     execution.status IS DISTINCT FROM 'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
       WITH ORDINALITY AS prior(entry,ordinality)
      WHERE prior.entry->'slot'->>'ordinal' IS NOT DISTINCT FROM execution.slot_ordinal::TEXT AND
       prior.ordinality<target_index
     ))
  ) OR NEW.workspace_sha256 IS DISTINCT FROM binding.workspace_sha256 OR
  NEW.step_attempt IS DISTINCT FROM deployment.current_step_attempt OR
  NEW.worker_id IS DISTINCT FROM deployment.current_worker_id OR authority_valid IS DISTINCT FROM TRUE OR
  NEW.status IS DISTINCT FROM 'started' OR manifest_entry IS NULL OR
  manifest_entry->'slot'->>'name' IS DISTINCT FROM NEW.slot_name OR
  manifest_entry->>'command_sha256' IS DISTINCT FROM NEW.command_sha256 THEN
  RAISE EXCEPTION 'deployment execution start authority is invalid';
 END IF;
 RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_generated_deployment_rollback_attempt_insert() RETURNS TRIGGER AS $function$
DECLARE
 deployment generated_workload_deployments;
 plan generated_workload_deployment_rollback_plans;
 attempt_count INTEGER;
 latest_attempt BIGINT;
 latest_residual BOOLEAN;
 candidate_valid BOOLEAN;
 authority_valid BOOLEAN;
 initial_start_exists BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT count(*),max(step_attempt) INTO attempt_count,latest_attempt
 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id;
 IF latest_attempt IS NOT NULL THEN
  SELECT outcome='residual' INTO latest_residual FROM generated_workload_deployment_rollback_observations
  WHERE operation_id=NEW.operation_id AND rollback_step_attempt=latest_attempt AND basis='command_attempt';
 END IF;
 SELECT EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions
  WHERE operation_id=NEW.operation_id AND slot_name='initial_start' AND slot_ordinal=30
 ) INTO initial_start_exists;
 SELECT head.candidate_deployment_id=deployment.id AND head.candidate_step_attempt=NEW.step_attempt AND
  head.candidate_worker_id=NEW.worker_id INTO candidate_valid
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
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  NEW.command_sha256 IS DISTINCT FROM plan.command_sha256 OR
  NEW.workspace_sha256 IS DISTINCT FROM plan.workspace_sha256 OR initial_start_exists IS DISTINCT FROM TRUE OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id AND status='started') OR
  attempt_count>=plan.max_attempts OR (latest_attempt IS NOT NULL AND
   (NEW.step_attempt<=latest_attempt OR latest_residual IS DISTINCT FROM TRUE)) OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment rollback attempt authority is invalid or exhausted';
 END IF;
 RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_generated_deployment_rollback_observation_insert() RETURNS TRIGGER AS $function$
DECLARE
 deployment generated_workload_deployments;
 plan generated_workload_deployment_rollback_plans;
 observation JSONB:=NEW.observation_json::JSONB;
 candidate_valid BOOLEAN;
 authority_valid BOOLEAN;
 valid_evidence BOOLEAN;
 expected_observation TEXT;
 invalid_resources BOOLEAN;
 basis_valid BOOLEAN;
 pre_attempt_count INTEGER;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO plan FROM generated_workload_deployment_rollback_plans WHERE operation_id=NEW.operation_id;
 SELECT count(*) INTO pre_attempt_count FROM generated_workload_deployment_rollback_observations
 WHERE operation_id=NEW.operation_id AND basis='pre_attempt';
 SELECT head.candidate_deployment_id=deployment.id AND
  head.candidate_step_attempt=NEW.observer_step_attempt AND
  head.candidate_worker_id=NEW.observer_worker_id INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head WHERE head.project_id=deployment.project_id;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.observer_generation AND
  steps.status='running' AND steps.generation=NEW.observer_generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.observer_step_attempt AND
  steps.worker_id=NEW.observer_worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.observer_worker_id AND attempts.expires_at>clock_timestamp() INTO authority_valid
 FROM jobs JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.observer_step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.observer_job_id AND
  attempts.generation=NEW.observer_generation AND attempts.step_id=NEW.observer_step_id AND
  attempts.attempt=NEW.observer_step_attempt WHERE jobs.id=NEW.observer_job_id;
 SELECT CASE NEW.basis
  WHEN 'command_attempt' THEN NEW.rollback_step_attempt>0 AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts AS attempt
   WHERE attempt.operation_id=NEW.operation_id AND attempt.step_attempt=NEW.rollback_step_attempt AND
    attempt.command_sha256=plan.command_sha256 AND attempt.workspace_sha256=plan.workspace_sha256
  )
  WHEN 'pre_attempt' THEN NEW.rollback_step_attempt=-NEW.observer_step_attempt AND NOT EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=NEW.operation_id
  ) AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions WHERE operation_id=NEW.operation_id
  ) AND (pre_attempt_count<plan.max_attempts OR (NEW.outcome='clean' AND NOT EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id AND status='started'
  )))
  ELSE FALSE END INTO basis_valid;
 SELECT EXISTS(
  SELECT 1 FROM (
   SELECT item,ordinality,LAG(item #>> '{}') OVER (PARTITION BY resource ORDER BY ordinality) AS previous,resource
   FROM (VALUES ('container',observation->'container_ids'),('network',observation->'network_ids'),
    ('volume',observation->'volume_names')) AS resources(resource,items)
   CROSS JOIN LATERAL jsonb_array_elements(resources.items) WITH ORDINALITY AS values(item,ordinality)
  ) AS entries WHERE jsonb_typeof(item) IS DISTINCT FROM 'string' OR
   (resource IN ('container','network') AND ((item #>> '{}') ~ '^[0-9a-f]{64}$') IS DISTINCT FROM TRUE) OR
   (resource='volume' AND ((item #>> '{}') ~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$') IS DISTINCT FROM TRUE) OR
   (previous IS NOT NULL AND (item #>> '{}')<=previous)
 ) INTO invalid_resources;
 expected_observation:='{"compose_project":"'||plan.compose_project||'","container_ids":'||
  replace((observation->'container_ids')::TEXT,', ',',')||',"network_ids":'||
  replace((observation->'network_ids')::TEXT,', ',',')||',"postcondition_sha256":"'||
  plan.postcondition_sha256||'","schema":"omnidex.generated-deployment-rollback-observation.v1","volume_names":'||
  replace((observation->'volume_names')::TEXT,', ',',')||'}';
 IF deployment.status NOT IN ('applying','indeterminate') OR
  ROW(NEW.observer_job_id,NEW.observer_generation,NEW.observer_step_id,
      NEW.observer_step_attempt,NEW.observer_worker_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  basis_valid IS DISTINCT FROM TRUE OR
  (NEW.basis='command_attempt' AND EXISTS(
   SELECT 1 FROM generated_workload_deployment_rollback_attempts AS later
   WHERE later.operation_id=NEW.operation_id AND later.step_attempt>NEW.rollback_step_attempt
  )) OR generated_deployment_exact_keys(
   observation,ARRAY['compose_project','container_ids','network_ids','postcondition_sha256','schema','volume_names']
  ) IS DISTINCT FROM TRUE OR
  observation->>'schema' IS DISTINCT FROM 'omnidex.generated-deployment-rollback-observation.v1' OR
  observation->>'compose_project' IS DISTINCT FROM plan.compose_project OR
  observation->>'postcondition_sha256' IS DISTINCT FROM plan.postcondition_sha256 OR
  jsonb_typeof(observation->'container_ids') IS DISTINCT FROM 'array' OR
  jsonb_typeof(observation->'network_ids') IS DISTINCT FROM 'array' OR
  jsonb_typeof(observation->'volume_names') IS DISTINCT FROM 'array' OR
  jsonb_array_length(observation->'container_ids')>1024 OR
  jsonb_array_length(observation->'network_ids')>1024 OR
  jsonb_array_length(observation->'volume_names')>1024 OR invalid_resources IS DISTINCT FROM FALSE OR
  NEW.observation_json IS DISTINCT FROM expected_observation OR
  (NEW.outcome='clean') IS DISTINCT FROM
   (plan.require_container_absence AND plan.require_network_absence AND plan.require_volume_absence AND
    jsonb_array_length(observation->'container_ids')=0 AND
    jsonb_array_length(observation->'network_ids')=0 AND
    jsonb_array_length(observation->'volume_names')=0) THEN
  RAISE EXCEPTION 'deployment rollback observation authority is invalid';
 END IF;
 SELECT evidence.job_id=deployment.job_id AND evidence.step_id=deployment.step_id AND
  evidence.kind='deployment_observation' AND
  evidence.source_type='generated_workload_deployment_rollback_observation' AND
  evidence.source_ref=NEW.operation_id AND evidence.payload_json->>'hash'=NEW.observation_sha256 AND
  evidence.payload_json->>'excerpt'=NEW.observation_json AND
  evidence.payload_json->'metadata'->>'outcome'=NEW.outcome AND
  evidence.payload_json->'metadata'->>'basis'=NEW.basis AND
  (evidence.payload_json->'metadata'->>'rollback_step_attempt')::BIGINT=NEW.rollback_step_attempt AND
  evidence.payload_json->'metadata'->>'postcondition_sha256'=plan.postcondition_sha256 INTO valid_evidence
 FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF valid_evidence IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment rollback observation evidence is invalid';
 END IF;
 RETURN NEW;
END;
$function$ LANGUAGE plpgsql;

COMMIT;
