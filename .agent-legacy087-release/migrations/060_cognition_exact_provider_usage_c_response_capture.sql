CREATE TABLE cognition_policy_provider_response_captures (
    evidence_id TEXT PRIMARY KEY CHECK (
        evidence_id~'^provider_response_capture_[0-9a-f]{64}$'
    ),
    call_id TEXT NOT NULL UNIQUE,
    episode_id TEXT NOT NULL,
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation>0),
    step_id BIGINT NOT NULL,
    step_attempt BIGINT NOT NULL CHECK (step_attempt>0),
    worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(worker_id)),
    capture_sha256 TEXT NOT NULL CHECK (capture_sha256~'^[0-9a-f]{64}$'),
    capture_bytes BIGINT NOT NULL CHECK (
        capture_bytes>=0 AND capture_bytes<=16777217
    ),
    ref_json TEXT NOT NULL CHECK (
        jsonb_typeof(ref_json::jsonb)='object' AND octet_length(ref_json)<=1024
    ),
    ref_sha256 TEXT NOT NULL CHECK (
        ref_sha256~'^[0-9a-f]{64}$' AND
        ref_sha256=encode(digest(ref_json,'sha256'),'hex')
    ),
    content BYTEA NOT NULL CHECK (
        octet_length(content)=capture_bytes AND
        encode(digest(content,'sha256'),'hex')=capture_sha256
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id)
        REFERENCES cognition_policy_calls(
            call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
        ) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION require_cognition_policy_provider_response_capture()
RETURNS TRIGGER AS $$
DECLARE call_row cognition_policy_calls%ROWTYPE;
BEGIN
    IF NOT cognition_json_has_unique_keys(NEW.ref_json::json) OR
       NEW.ref_json<>cognition_canonical_jsonb(NEW.ref_json::jsonb) OR
       NOT cognition_json_object_has_exact_keys(NEW.ref_json::json,ARRAY[
           'schema','id','sha256','bytes'
       ]) OR
       NEW.ref_json::jsonb->>'schema'<>'omnidex.provider-response-capture-evidence.v1' OR
       NEW.ref_json::jsonb->>'id'<>NEW.evidence_id OR
       NEW.ref_json::jsonb->>'sha256'<>NEW.capture_sha256 OR
       (NEW.ref_json::jsonb->>'bytes')::BIGINT<>NEW.capture_bytes OR
       NEW.evidence_id<>'provider_response_capture_'||encode(digest(
           cognition_canonical_jsonb(jsonb_build_object(
               'call_id',NEW.call_id,
               'ref',jsonb_set(NEW.ref_json::jsonb,'{id}','""'::jsonb,false)
           )),'sha256'
       ),'hex') THEN
        RAISE EXCEPTION 'provider response capture identity is invalid';
    END IF;
    SELECT * INTO call_row FROM cognition_policy_calls calls
    WHERE calls.call_id=NEW.call_id FOR SHARE;
    IF NOT FOUND OR call_row.status NOT IN ('accepted','rejected','failed') OR
       call_row.result_json::jsonb->'provider_response_capture_evidence'<>NEW.ref_json::jsonb OR
       call_row.result_json::jsonb->>'provider_response_capture_sha256'<>NEW.capture_sha256 OR
       (call_row.result_json::jsonb->>'provider_response_captured_bytes')::BIGINT<>NEW.capture_bytes OR
       ROW(call_row.episode_id,call_row.job_id,call_row.generation,call_row.step_id,
           call_row.step_attempt,call_row.worker_id)
       IS DISTINCT FROM
       ROW(NEW.episode_id,NEW.job_id,NEW.generation,NEW.step_id,
           NEW.step_attempt,NEW.worker_id) THEN
        RAISE EXCEPTION 'provider response capture lacks its exact terminal call';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_provider_response_capture_exact_call
AFTER INSERT ON cognition_policy_provider_response_captures DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_provider_response_capture();

CREATE OR REPLACE FUNCTION require_cognition_policy_call_provider_response_capture()
RETURNS TRIGGER AS $$
DECLARE evidence_ref JSONB;
BEGIN
    IF NEW.status NOT IN ('accepted','rejected','failed') THEN RETURN NULL; END IF;
    evidence_ref := NEW.result_json::jsonb->'provider_response_capture_evidence';
    IF evidence_ref<>'{"schema":"","id":"","sha256":"","bytes":0}'::jsonb AND NOT EXISTS (
        SELECT 1 FROM cognition_policy_provider_response_captures evidence
        WHERE evidence.call_id=NEW.call_id AND evidence.ref_json::jsonb=evidence_ref
    ) THEN
        RAISE EXCEPTION 'terminal policy call lacks exact provider response capture';
    END IF;
    IF evidence_ref='{"schema":"","id":"","sha256":"","bytes":0}'::jsonb AND EXISTS (
        SELECT 1 FROM cognition_policy_provider_response_captures evidence
        WHERE evidence.call_id=NEW.call_id
    ) THEN
        RAISE EXCEPTION 'terminal policy call has extraneous provider response capture';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_policy_calls_require_provider_response_capture
AFTER INSERT OR UPDATE ON cognition_policy_calls DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_policy_call_provider_response_capture();

CREATE TRIGGER cognition_policy_provider_response_captures_immutable
BEFORE UPDATE OR DELETE ON cognition_policy_provider_response_captures
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE TRIGGER cognition_policy_provider_response_captures_no_truncate
BEFORE TRUNCATE ON cognition_policy_provider_response_captures
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
