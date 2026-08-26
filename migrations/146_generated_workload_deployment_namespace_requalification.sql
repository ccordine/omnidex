BEGIN;

LOCK TABLE generated_workload_deployment_executions IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM generated_workload_deployment_executions
  WHERE slot_name IN ('build','initial_start') OR slot_ordinal IN (20,30)
 ) THEN
  RAISE EXCEPTION 'deployment namespace requalification requires explicit audit of every legacy protected execution';
 END IF;
END;
$$;

CREATE TABLE generated_workload_deployment_namespace_requalifications (
 operation_id TEXT NOT NULL REFERENCES generated_workload_deployments(id) ON DELETE RESTRICT,
 job_id BIGINT NOT NULL,
 generation BIGINT NOT NULL CHECK(generation>0),
 step_id BIGINT NOT NULL,
 slot_name TEXT NOT NULL CHECK(slot_name IN ('build','initial_start')),
 slot_ordinal INTEGER NOT NULL CHECK(slot_ordinal IN (20,30)),
 command_sha256 TEXT NOT NULL CHECK(command_sha256 ~ '^[0-9a-f]{64}$'),
 workspace_sha256 TEXT NOT NULL CHECK(workspace_sha256 ~ '^[0-9a-f]{64}$'),
 compose_project TEXT NOT NULL CHECK(compose_project ~ '^[a-z0-9][a-z0-9_-]{0,62}$'),
 step_attempt BIGINT NOT NULL CHECK(step_attempt>0),
 worker_id TEXT NOT NULL CHECK(
  worker_id IS DISTINCT FROM '' AND worker_id=BTRIM(worker_id) AND octet_length(worker_id)<=256
 ),
 proof_json TEXT NOT NULL CHECK(octet_length(proof_json) BETWEEN 2 AND 32768),
 proof_sha256 TEXT NOT NULL CHECK(
  proof_sha256 ~ '^[0-9a-f]{64}$' AND
  proof_sha256=encode(digest(convert_to(proof_json,'UTF8'),'sha256'),'hex')
 ),
 evidence_id BIGINT NOT NULL UNIQUE REFERENCES evidence(id) ON DELETE RESTRICT,
 observed_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(operation_id,slot_ordinal,step_attempt),
 FOREIGN KEY(job_id,generation,step_id,step_attempt)
  REFERENCES job_step_attempts(job_id,generation,step_id,attempt) ON DELETE RESTRICT,
 CHECK((slot_name='build')=(slot_ordinal=20)),
 CHECK((slot_name='initial_start')=(slot_ordinal=30))
);

CREATE FUNCTION validate_generated_deployment_namespace_requalification_insert()
RETURNS TRIGGER AS $$
DECLARE
 deployment generated_workload_deployments;
 binding generated_workload_deployment_verifications;
 manifest_entry JSONB;
 target_index INTEGER;
 authority_valid BOOLEAN;
 candidate_valid BOOLEAN;
 valid_evidence BOOLEAN;
 evidence_created_at TIMESTAMPTZ;
BEGIN
 PERFORM 1 FROM jobs WHERE id=NEW.job_id FOR UPDATE;
 PERFORM 1 FROM job_steps WHERE job_id=NEW.job_id AND id=NEW.step_id FOR UPDATE;
 PERFORM 1 FROM job_step_attempts
  WHERE job_id=NEW.job_id AND generation=NEW.generation AND step_id=NEW.step_id AND attempt=NEW.step_attempt
  FOR UPDATE;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT * INTO binding FROM generated_workload_deployment_verifications WHERE operation_id=NEW.operation_id;
 SELECT head.candidate_deployment_id=NEW.operation_id AND head.candidate_job_id=NEW.job_id AND
  head.candidate_generation=NEW.generation AND head.candidate_step_id=NEW.step_id AND
  head.candidate_step_attempt=NEW.step_attempt AND head.candidate_worker_id=NEW.worker_id AND
  head.compose_project=NEW.compose_project INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head
 WHERE head.project_id=deployment.project_id FOR UPDATE;
 SELECT jobs.status='running' AND jobs.current_generation=NEW.generation AND
  steps.status='running' AND steps.generation=NEW.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp() AND
  NEW.observed_at>=attempts.claimed_at INTO authority_valid
 FROM jobs
 JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=NEW.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=NEW.job_id AND
  attempts.generation=NEW.generation AND attempts.step_id=NEW.step_id AND
  attempts.attempt=NEW.step_attempt
 WHERE jobs.id=NEW.job_id;
 SELECT entry,ordinality::INTEGER INTO manifest_entry,target_index
 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
  WITH ORDINALITY AS item(entry,ordinality)
 WHERE (entry->'slot'->>'ordinal')::INTEGER=NEW.slot_ordinal;
 SELECT evidence.created_at,
  evidence.job_id=NEW.job_id AND evidence.step_id=NEW.step_id AND
  evidence.kind='deployment_observation' AND
  evidence.source_type='generated_workload_deployment_namespace_requalification' AND
  evidence.source_ref=NEW.operation_id||':'||NEW.slot_ordinal::TEXT||':'||NEW.step_attempt::TEXT AND
  evidence.payload_json->>'hash'=NEW.proof_sha256 AND
  evidence.payload_json->>'excerpt'=NEW.proof_json AND
  evidence.payload_json->'metadata'->>'schema'='omnidex.generated-deployment-namespace-requalification.v1' AND
  evidence.payload_json->'metadata'->>'deployment_operation_id'=NEW.operation_id AND
  evidence.payload_json->'metadata'->>'slot'=NEW.slot_name AND
  (evidence.payload_json->'metadata'->>'ordinal')::INTEGER=NEW.slot_ordinal AND
  evidence.payload_json->'metadata'->>'command_sha256'=NEW.command_sha256 AND
  evidence.payload_json->'metadata'->>'workspace_sha256'=NEW.workspace_sha256 AND
  evidence.payload_json->'metadata'->>'compose_project'=NEW.compose_project AND
  (evidence.payload_json->'metadata'->>'step_attempt')::BIGINT=NEW.step_attempt AND
  evidence.payload_json->'metadata'->>'worker_id'=NEW.worker_id AND
  evidence.payload_json->'metadata'->>'namespace_vacant'='true' AND
  evidence.payload_json->'metadata'->>'proof_sha256'=NEW.proof_sha256
 INTO evidence_created_at,valid_evidence FROM evidence WHERE evidence.id=NEW.evidence_id;
 IF deployment.id IS NULL OR binding.operation_id IS NULL OR deployment.status IS DISTINCT FROM 'applying' OR
  ROW(NEW.job_id,NEW.generation,NEW.step_id,NEW.step_attempt,NEW.worker_id) IS DISTINCT FROM
  ROW(deployment.job_id,deployment.generation,deployment.step_id,
      deployment.current_step_attempt,deployment.current_worker_id) OR
  NEW.compose_project IS DISTINCT FROM deployment.compose_project OR
  NEW.workspace_sha256 IS DISTINCT FROM binding.workspace_sha256 OR candidate_valid IS DISTINCT FROM TRUE OR
  authority_valid IS DISTINCT FROM TRUE OR valid_evidence IS DISTINCT FROM TRUE OR
  evidence_created_at IS DISTINCT FROM NEW.observed_at OR NEW.observed_at>clock_timestamp() OR
  manifest_entry IS NULL OR manifest_entry->'slot'->>'name' IS DISTINCT FROM NEW.slot_name OR
  manifest_entry->>'command_sha256' IS DISTINCT FROM NEW.command_sha256 OR
  (SELECT count(*) FROM generated_workload_deployment_executions
   WHERE operation_id=NEW.operation_id) IS DISTINCT FROM target_index-1 OR
  EXISTS(
   SELECT 1 FROM generated_workload_deployment_executions AS execution
   WHERE execution.operation_id=NEW.operation_id AND
    (execution.step_attempt IS DISTINCT FROM NEW.step_attempt OR
     execution.worker_id IS DISTINCT FROM NEW.worker_id OR
     execution.status IS DISTINCT FROM 'completed' OR execution.succeeded IS DISTINCT FROM TRUE OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(binding.lifecycle_manifest_json::JSONB->'commands')
       WITH ORDINALITY AS prior(entry,ordinality)
      WHERE (prior.entry->'slot'->>'ordinal')::INTEGER=execution.slot_ordinal AND
       prior.ordinality<target_index
     ))
  ) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_attempts
   WHERE operation_id=NEW.operation_id) OR
  EXISTS(SELECT 1 FROM generated_workload_deployment_rollback_observations
   WHERE operation_id=NEW.operation_id) OR
  NOT generated_deployment_vacant_namespace_preflight_valid(
   NEW.compose_project,'docker_compose_resolved_config',
   jsonb_build_object(
    'namespace_preflight',NEW.proof_json::JSONB||jsonb_build_object('sha256',NEW.proof_sha256)
   )
  ) THEN
  RAISE EXCEPTION 'deployment namespace requalification authority is invalid';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER generated_deployment_namespace_requalification_insert_validate
BEFORE INSERT ON generated_workload_deployment_namespace_requalifications
FOR EACH ROW EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_insert();

CREATE FUNCTION require_generated_deployment_namespace_requalification_for_execution()
RETURNS TRIGGER AS $$
DECLARE
 deployment_authority RECORD;
 deployment generated_workload_deployments;
 authority_valid BOOLEAN;
 candidate_valid BOOLEAN;
 qualification_valid BOOLEAN;
BEGIN
 IF NOT (NEW.slot_name IN ('build','initial_start') OR NEW.slot_ordinal IN (20,30)) THEN
  RETURN NEW;
 END IF;
 IF (NEW.slot_name='build') IS DISTINCT FROM (NEW.slot_ordinal=20) OR
  (NEW.slot_name='initial_start') IS DISTINCT FROM (NEW.slot_ordinal=30) THEN
  RAISE EXCEPTION 'protected deployment execution slot identity is invalid';
 END IF;
 SELECT job_id,generation,step_id,project_id INTO deployment_authority
 FROM generated_workload_deployments WHERE id=NEW.operation_id;
 PERFORM 1 FROM jobs WHERE id=deployment_authority.job_id FOR UPDATE;
 PERFORM 1 FROM job_steps
  WHERE job_id=deployment_authority.job_id AND id=deployment_authority.step_id FOR UPDATE;
 PERFORM 1 FROM job_step_attempts
  WHERE job_id=deployment_authority.job_id AND generation=deployment_authority.generation AND
   step_id=deployment_authority.step_id AND attempt=NEW.step_attempt FOR UPDATE;
 SELECT * INTO deployment FROM generated_workload_deployments WHERE id=NEW.operation_id FOR UPDATE;
 SELECT head.candidate_deployment_id=NEW.operation_id AND
  head.candidate_job_id=deployment.job_id AND head.candidate_generation=deployment.generation AND
  head.candidate_step_id=deployment.step_id AND head.candidate_step_attempt=NEW.step_attempt AND
  head.candidate_worker_id=NEW.worker_id INTO candidate_valid
 FROM generated_workload_project_deployment_heads AS head
 WHERE head.project_id=deployment.project_id FOR UPDATE;
 SELECT jobs.status='running' AND jobs.current_generation=deployment.generation AND
  steps.status='running' AND steps.generation=deployment.generation AND
  steps.superseded_at_generation IS NULL AND steps.current_attempt=NEW.step_attempt AND
  steps.worker_id=NEW.worker_id AND attempts.status='active' AND
  attempts.worker_id=NEW.worker_id AND attempts.expires_at>clock_timestamp()
 INTO authority_valid
 FROM jobs
 JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=deployment.step_id
 JOIN job_step_attempts AS attempts ON attempts.job_id=deployment.job_id AND
  attempts.generation=deployment.generation AND attempts.step_id=deployment.step_id AND
  attempts.attempt=NEW.step_attempt
 WHERE jobs.id=deployment.job_id;
 SELECT qualification.operation_id=NEW.operation_id AND
  qualification.job_id=deployment.job_id AND qualification.generation=deployment.generation AND
  qualification.step_id=deployment.step_id AND qualification.slot_name=NEW.slot_name AND
  qualification.slot_ordinal=NEW.slot_ordinal AND
  qualification.command_sha256=NEW.command_sha256 AND
  qualification.workspace_sha256=NEW.workspace_sha256 AND
  qualification.compose_project=deployment.compose_project AND
  qualification.step_attempt=NEW.step_attempt AND qualification.worker_id=NEW.worker_id AND
  qualification.observed_at<=clock_timestamp()
 INTO qualification_valid
 FROM generated_workload_deployment_namespace_requalifications AS qualification
 WHERE qualification.operation_id=NEW.operation_id AND
  qualification.slot_ordinal=NEW.slot_ordinal AND qualification.step_attempt=NEW.step_attempt;
 IF deployment.id IS NULL OR deployment.status IS DISTINCT FROM 'applying' OR
  deployment.current_step_attempt IS DISTINCT FROM NEW.step_attempt OR
  deployment.current_worker_id IS DISTINCT FROM NEW.worker_id OR
  candidate_valid IS DISTINCT FROM TRUE OR authority_valid IS DISTINCT FROM TRUE OR
  qualification_valid IS DISTINCT FROM TRUE THEN
  RAISE EXCEPTION 'protected deployment execution lacks exact current-attempt namespace requalification';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER generated_deployment_execution_00_namespace_requalification_require
BEFORE INSERT ON generated_workload_deployment_executions
FOR EACH ROW EXECUTE FUNCTION require_generated_deployment_namespace_requalification_for_execution();

CREATE FUNCTION prevent_generated_deployment_namespace_requalification_evidence_change()
RETURNS TRIGGER AS $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_namespace_requalifications WHERE evidence_id=OLD.id
 ) THEN
  RAISE EXCEPTION 'generated deployment namespace requalification evidence is immutable';
 END IF;
 IF TG_OP='UPDATE' THEN RETURN NEW; END IF;
 RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER generated_deployment_namespace_requalification_evidence_immutable
BEFORE UPDATE OR DELETE ON evidence FOR EACH ROW
EXECUTE FUNCTION prevent_generated_deployment_namespace_requalification_evidence_change();

CREATE TRIGGER generated_deployment_namespace_requalification_change_immutable
BEFORE UPDATE OR DELETE ON generated_workload_deployment_namespace_requalifications
FOR EACH ROW EXECUTE FUNCTION prevent_generated_deployment_rail_change();

CREATE TRIGGER generated_deployment_namespace_requalification_truncate_immutable
BEFORE TRUNCATE ON generated_workload_deployment_namespace_requalifications
FOR EACH STATEMENT EXECUTE FUNCTION prevent_generated_deployment_rail_change();

CREATE FUNCTION validate_generated_deployment_namespace_requalification_convergence()
RETURNS TRIGGER AS $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions AS execution
  JOIN generated_workload_deployments AS deployment ON deployment.id=execution.operation_id
  LEFT JOIN generated_workload_deployment_namespace_requalifications AS qualification ON
   qualification.operation_id=execution.operation_id AND
   qualification.slot_ordinal=execution.slot_ordinal AND
   qualification.step_attempt=execution.step_attempt AND
   qualification.job_id=deployment.job_id AND
   qualification.generation=deployment.generation AND
   qualification.step_id=deployment.step_id AND
   qualification.slot_name=execution.slot_name AND
   qualification.command_sha256=execution.command_sha256 AND
   qualification.workspace_sha256=execution.workspace_sha256 AND
   qualification.compose_project=deployment.compose_project AND
   qualification.worker_id=execution.worker_id
  WHERE (execution.slot_name IN ('build','initial_start') OR execution.slot_ordinal IN (20,30)) AND
   qualification.operation_id IS NULL
 ) THEN
  RAISE EXCEPTION 'committed protected deployment execution lacks exact namespace requalification';
 END IF;
 RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER generated_deployment_namespace_requalification_converge_from_proof
AFTER INSERT ON generated_workload_deployment_namespace_requalifications
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_convergence();

CREATE CONSTRAINT TRIGGER generated_deployment_namespace_requalification_converge_from_execution
AFTER INSERT OR UPDATE ON generated_workload_deployment_executions
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION validate_generated_deployment_namespace_requalification_convergence();

CREATE CONSTRAINT TRIGGER generated_deployment_head_consistency_from_namespace_requalification
AFTER INSERT ON generated_workload_deployment_namespace_requalifications
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION validate_generated_deployment_head_consistency();

DO $$
BEGIN
 IF EXISTS(
  SELECT 1 FROM generated_workload_deployment_executions
  WHERE slot_name IN ('build','initial_start') OR slot_ordinal IN (20,30)
 ) THEN
  RAISE EXCEPTION 'deployment namespace requalification postcondition found unqualified protected execution';
 END IF;
END;
$$;

COMMIT;
