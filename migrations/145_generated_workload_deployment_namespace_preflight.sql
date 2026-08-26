BEGIN;
CREATE FUNCTION generated_deployment_vacant_namespace_preflight_valid(
 project TEXT,config_source TEXT,config_metadata JSONB
) RETURNS BOOLEAN AS $$
DECLARE proof JSONB; expected TEXT;
BEGIN
 IF project IS NULL OR project !~ '^[a-z0-9][a-z0-9_-]{0,62}$' OR
  config_source IS DISTINCT FROM 'docker_compose_resolved_config' OR
  jsonb_typeof(config_metadata) IS DISTINCT FROM 'object' THEN RETURN FALSE; END IF;
 proof:=config_metadata->'namespace_preflight';
 IF jsonb_typeof(proof) IS DISTINCT FROM 'object' OR
  NOT generated_deployment_exact_keys(proof,ARRAY[
   'compose_project','container_ids','network_ids','schema','sha256','volume_names'
  ]) OR proof->>'schema' IS DISTINCT FROM 'omnidex.generated-deployment-namespace-preflight.v1' OR
  proof->>'compose_project' IS DISTINCT FROM project OR proof->'container_ids' IS DISTINCT FROM '[]'::JSONB OR
  proof->'network_ids' IS DISTINCT FROM '[]'::JSONB OR
  proof->'volume_names' IS DISTINCT FROM '[]'::JSONB OR proof->>'sha256' IS NULL OR
  proof->>'sha256' !~ '^[0-9a-f]{64}$'
 THEN RETURN FALSE; END IF;
 expected:='{"compose_project":'||to_jsonb(project)::TEXT||
  ',"container_ids":[],"network_ids":[],"schema":"omnidex.generated-deployment-namespace-preflight.v1","volume_names":[]}';
 RETURN proof->>'sha256'=encode(digest(convert_to(expected,'UTF8'),'sha256'),'hex');
END $$ LANGUAGE plpgsql IMMUTABLE;
DO $$ BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_verifications AS binding
  JOIN generated_workload_deployments AS deployment ON deployment.id=binding.operation_id
  WHERE NOT EXISTS(
   SELECT 1 FROM generated_workload_verifications AS verification
   JOIN evidence AS config ON config.id=verification.command_evidence_ids[
    cardinality(verification.command_evidence_ids)
   ] AND config.job_id=deployment.job_id AND config.step_id=deployment.step_id
   WHERE verification.id=binding.verification_id AND
    generated_deployment_vacant_namespace_preflight_valid(
     deployment.compose_project,config.source_type,config.payload_json->'metadata'
    )
  )
 ) THEN RAISE EXCEPTION
  'deployment namespace preflight rail requires a vacant proof for every existing deployment binding';
 END IF;
END $$;
CREATE FUNCTION validate_generated_deployment_namespace_preflight_insert() RETURNS TRIGGER AS $$
DECLARE deployment generated_workload_deployments; valid_proof BOOLEAN;
BEGIN
 SELECT * INTO deployment FROM generated_workload_deployments
  WHERE id=NEW.operation_id FOR KEY SHARE;
 SELECT generated_deployment_vacant_namespace_preflight_valid(
   deployment.compose_project,config.source_type,config.payload_json->'metadata'
  ) INTO valid_proof
 FROM generated_workload_verifications AS verification
 JOIN evidence AS config ON config.id=verification.command_evidence_ids[
  cardinality(verification.command_evidence_ids)
 ] AND config.job_id=deployment.job_id AND config.step_id=deployment.step_id
 WHERE verification.id=NEW.verification_id;
 IF deployment.id IS NULL OR valid_proof IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'deployment binding requires an exact vacant Compose namespace preflight proof';
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER generated_deployment_binding_namespace_preflight_validate
 BEFORE INSERT ON generated_workload_deployment_verifications FOR EACH ROW
 EXECUTE FUNCTION validate_generated_deployment_namespace_preflight_insert();
COMMIT;
