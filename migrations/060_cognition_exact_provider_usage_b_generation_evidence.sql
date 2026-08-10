CREATE TABLE cognition_policy_provider_generation_evidence (
    evidence_id TEXT PRIMARY KEY CHECK (
        evidence_id~'^provider_generation_[0-9a-f]{64}$'
    ),
    call_id TEXT NOT NULL UNIQUE,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    generation_sha256 TEXT NOT NULL CHECK (generation_sha256~'^[0-9a-f]{64}$'),
    generation_bytes BIGINT NOT NULL CHECK (
		generation_bytes>0 AND generation_bytes<=24466776
    ),
    ref_json TEXT NOT NULL CHECK (
        jsonb_typeof(ref_json::jsonb)='object' AND octet_length(ref_json)<=1024
    ),
    ref_sha256 TEXT NOT NULL CHECK (
        ref_sha256~'^[0-9a-f]{64}$' AND
        ref_sha256=encode(digest(ref_json,'sha256'),'hex')
    ),
    generation_json TEXT NOT NULL CHECK (
        octet_length(generation_json)=generation_bytes AND
        encode(digest(generation_json,'sha256'),'hex')=generation_sha256
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES cognition_policy_calls(
            call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
        ) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION require_cognition_policy_provider_generation_evidence()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
BEGIN
    IF NOT cognition_json_has_unique_keys(NEW.ref_json::json) OR
       NEW.ref_json<>cognition_canonical_jsonb(NEW.ref_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(NEW.ref_json::json,ARRAY[
           'schema','id','sha256','bytes'
       ]) OR
       NEW.ref_json::jsonb->>'schema'<>'omnidex.provider-generation-evidence.v1' OR
       NEW.ref_json::jsonb->>'id'<>NEW.evidence_id OR
       NEW.ref_json::jsonb->>'sha256'<>NEW.generation_sha256 OR
       (NEW.ref_json::jsonb->>'bytes')::BIGINT<>NEW.generation_bytes OR
       NEW.evidence_id<>'provider_generation_'||encode(digest(
           cognition_canonical_jsonb(jsonb_build_object(
               'call_id',NEW.call_id,
               'ref',jsonb_set(NEW.ref_json::jsonb,'{id}','""'::jsonb,false)
           )),'sha256'
       ),'hex') OR
       NOT cognition_json_has_unique_keys(NEW.generation_json::json) OR
       NEW.generation_json<>cognition_canonical_jsonb(NEW.generation_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(NEW.generation_json::json,ARRAY[
           'schema_bytes','provider_request_dispatched','content_bytes',
           'provider_request_sha256_bytes','provider_http_status',
           'provider_response_disposition_bytes','provider_response_complete',
           'provider_response_bytes_known','provider_response_sha256_bytes',
           'provider_response_bytes','provider_response_capture_sha256_bytes',
           'provider_response_captured_bytes','provider_done_reason_bytes',
           'usage_present','usage','provider_observation'
       ]) OR NOT cognition_json_object_has_exact_keys(
           (NEW.generation_json::json->'usage')::json,ARRAY[
               'prompt_eval_count','eval_count','total_duration_nanos','load_duration_nanos',
               'prompt_eval_duration_nanos','eval_duration_nanos'
           ]
       ) OR NOT cognition_json_object_has_exact_keys(
           (NEW.generation_json::json->'provider_observation')::json,ARRAY[
               'schema_bytes','observed_year','observed_month','observed_day','observed_hour',
		       'observed_minute','observed_second','observed_nanosecond','observed_at_bytes',
               'observed_location_bytes','observed_offset_seconds','attestation_sha256_bytes',
               'version_body_sha256_bytes','installed_body_sha256_bytes',
               'preload_body_sha256_bytes','runner_body_sha256_bytes','preload_method_bytes',
               'preload_endpoint_bytes','preload_request_sha256_bytes','challenge_sha256_bytes',
               'observation_sha256_bytes'
       ]) OR
       (NEW.generation_json::jsonb->>'provider_request_dispatched')::BOOLEAN IS NOT TRUE THEN
        RAISE EXCEPTION 'untrusted cognition provider generation identity is invalid';
    END IF;
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    IF NOT FOUND OR call_row.status<>'failed' OR
       call_row.result_json::jsonb->>'failure_code' NOT IN (
           'provider_evidence_invalid','provider_request_mismatch'
       ) OR
       call_row.result_json::jsonb->'provider_generation_evidence'<>NEW.ref_json::jsonb OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id)
       IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) THEN
        RAISE EXCEPTION 'untrusted provider generation lacks its exact terminal call';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_provider_generation_exact_call
AFTER INSERT ON cognition_policy_provider_generation_evidence DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_provider_generation_evidence();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_provider_generation_evidence()
RETURNS TRIGGER AS $$
DECLARE evidence_ref JSONB;
BEGIN
    IF NEW.status NOT IN ('accepted','rejected','failed') THEN RETURN NULL; END IF;
    evidence_ref := NEW.result_json::jsonb->'provider_generation_evidence';
    IF evidence_ref<>'{"schema":"","id":"","sha256":"","bytes":0}'::jsonb AND NOT EXISTS (
        SELECT 1 FROM cognition_policy_provider_generation_evidence evidence
        WHERE evidence.call_id=NEW.call_id AND evidence.ref_json::jsonb=evidence_ref
    ) THEN
        RAISE EXCEPTION 'terminal policy call lacks its untrusted provider evidence';
    END IF;
    IF evidence_ref='{"schema":"","id":"","sha256":"","bytes":0}'::jsonb AND EXISTS (
        SELECT 1 FROM cognition_policy_provider_generation_evidence evidence
        WHERE evidence.call_id=NEW.call_id
    ) THEN
        RAISE EXCEPTION 'trusted terminal policy call has untrusted provider evidence';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_provider_generation_evidence
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_provider_generation_evidence();

CREATE TRIGGER cognition_policy_provider_generation_evidence_immutable
BEFORE UPDATE OR DELETE ON cognition_policy_provider_generation_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_policy_provider_generation_evidence_no_truncate
BEFORE TRUNCATE ON cognition_policy_provider_generation_evidence
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
